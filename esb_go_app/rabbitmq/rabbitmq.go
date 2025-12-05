package rabbitmq

import (
	"esb-go-app/config"
	"esb-go-app/metrics"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

// RabbitMQ - обертка для соединения с RabbitMQ.
type RabbitMQ struct {
	conn    *amqp091.Connection
	logger  *slog.Logger
	workers map[string]bool // Карта для отслеживания запущенных воркеров
}

// New создает и возвращает новый экземпляр RabbitMQ.
func New(cfg *config.RabbitMQConfig, logger *slog.Logger) (*RabbitMQ, error) {
	conn, err := amqp091.Dial(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}
	logger.Info("connected to RabbitMQ successfully")

	return &RabbitMQ{
		conn:    conn,
		logger:  logger,
		workers: make(map[string]bool),
	}, nil
}

// Close закрывает соединение с RabbitMQ.
func (r *RabbitMQ) Close() error {
	return r.conn.Close()
}

// SetupDurableTopology создает постоянный обменник и очередь.
// Эта топология используется как надежное хранилище в обоих направлениях.
// Inbound: Продюсеры отправляют сюда, "мост" забирает.
// Outbound: "Мост" отправляет сюда, потребители забирают.
func (r *RabbitMQ) SetupDurableTopology(baseName string) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open a channel: %w", err)
	}
	defer ch.Close()

	durableExchangeName := "durable_exchange_for_" + baseName
	durableQueueName := "durable_queue_for_" + baseName

	// 1. Создаем постоянный обменник
	r.logger.Info("declaring durable exchange", "exchange", durableExchangeName)
	err = ch.ExchangeDeclare(durableExchangeName, "fanout", true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to declare durable exchange: %w", err)
	}

	// 2. Создаем постоянную очередь
	r.logger.Info("declaring durable queue", "queue", durableQueueName)
	_, err = ch.QueueDeclare(durableQueueName, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to declare durable queue: %w", err)
	}

	// 3. Связываем их
	r.logger.Info("binding durable queue to durable exchange", "queue", durableQueueName, "exchange", durableExchangeName)
	err = ch.QueueBind(durableQueueName, "", durableExchangeName, false, nil)
	if err != nil {
		return fmt.Errorf("failed to bind durable queue: %w", err)
	}

	r.logger.Info("durable topology setup complete", "baseName", baseName)
	return nil
}

// StartInboundForwarder запускает воркер для INBOUND направления.
// Он пересылает сообщения из постоянной очереди во временную (для 1С).
func (r *RabbitMQ) StartInboundForwarder(baseName string) {
	workerKey := "inbound-" + baseName
	if r.workers[workerKey] {
		r.logger.Warn("inbound forwarder already started, skipping", "baseName", baseName)
		return
	}

	sourceQueue := "durable_queue_for_" + baseName
	destQueue := baseName

	r.logger.Info("starting INBOUND forwarder", "from", sourceQueue, "to", destQueue)
	r.workers[workerKey] = true
	metrics.ActiveWorkers.WithLabelValues("inbound").Inc()

	go func() {
		defer metrics.ActiveWorkers.WithLabelValues("inbound").Dec()
		for {
			time.Sleep(1 * time.Second) // Пауза между попытками
			err := r.forwardOneMessage(sourceQueue, destQueue)
			if err != nil {
				// Логируем только "настоящие" ошибки, а не "очередь пуста" или "очередь еще не создана"
				if err.Error() != "no message in queue" && !strings.Contains(err.Error(), "does not exist yet") {
					r.logger.Error("inbound forwarder error", "baseName", baseName, "error", err)
					metrics.ErrorsTotal.WithLabelValues("inbound").Inc()
					time.Sleep(5 * time.Second)
				}
			}
		}
	}()
}

// forwardOneMessage - атомарная операция для Inbound воркера.
func (r *RabbitMQ) forwardOneMessage(sourceQueue, destQueue string) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("could not open channel: %w", err)
	}
	defer ch.Close()

	_, err = ch.QueueDeclarePassive(destQueue, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("destination queue '%s' does not exist yet", destQueue)
	}

	msg, ok, err := ch.Get(sourceQueue, false) // autoAck = false
	if err != nil {
		return fmt.Errorf("failed to get message from '%s': %w", sourceQueue, err)
	}
	if !ok {
		return fmt.Errorf("no message in queue")
	}

	err = ch.Publish("", destQueue, false, false, amqp091.Publishing{
		Headers:         msg.Headers,
		ContentType:     msg.ContentType,
		ContentEncoding: msg.ContentEncoding,
		DeliveryMode:    amqp091.Transient,
		Priority:        msg.Priority,
		CorrelationId:   msg.CorrelationId,
		ReplyTo:         msg.ReplyTo,
		Expiration:      msg.Expiration,
		MessageId:       msg.MessageId,
		Timestamp:       msg.Timestamp,
		Type:            msg.Type,
		UserId:          msg.UserId,
		AppId:           msg.AppId,
		Body:            msg.Body,
	})

	if err != nil {
		r.logger.Error("failed to forward message, requeueing", "error", err)
		_ = msg.Nack(false, true) // Requeue
		return fmt.Errorf("failed to publish to '%s': %w", destQueue, err)
	}

	_ = msg.Ack(false)
	r.logger.Info("message forwarded successfully (INBOUND)", "from", sourceQueue, "to", destQueue, "msgId", msg.MessageId)
	metrics.MessagesProcessed.WithLabelValues("inbound", sourceQueue, destQueue).Inc()
	return nil
}

// StartOutboundCollector запускает воркер для OUTBOUND направления.
// Забирает сообщения из временной очереди 1С и сохраняет их в постоянную.
func (r *RabbitMQ) StartOutboundCollector(baseName string) {
	workerKey := "outbound-" + baseName
	if r.workers[workerKey] {
		r.logger.Warn("outbound collector already started, skipping", "baseName", baseName)
		return
	}

	sourceQueue := baseName // Временная очередь, куда пишет 1С
	destExchange := "durable_exchange_for_" + baseName

	r.logger.Info("starting OUTBOUND collector", "from", sourceQueue, "to", destExchange)
	r.workers[workerKey] = true
	metrics.ActiveWorkers.WithLabelValues("outbound").Inc()

	go func() {
		defer metrics.ActiveWorkers.WithLabelValues("outbound").Dec()
		for {
			err := r.collectMessages(sourceQueue, destExchange)
			r.logger.Error("outbound collector failed, restarting...", "baseName", baseName, "error", err)
			metrics.ErrorsTotal.WithLabelValues("outbound").Inc()
			time.Sleep(5 * time.Second)
		}
	}()
}

// collectMessages - основная логика для Outbound воркера.
func (r *RabbitMQ) collectMessages(sourceQueue, destExchange string) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("could not open channel: %w", err)
	}
	defer ch.Close()

	// Убеждаемся, что временная очередь существует. Если нет, это не ошибка,
	// консьюмер просто будет ждать ее появления.
	// Этот declare нужен, чтобы избежать ошибок, если 1С еще не создала очередь.
	_, err = ch.QueueDeclare(sourceQueue, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("cannot declare source queue '%s': %w", sourceQueue, err)
	}

	msgs, err := ch.Consume(sourceQueue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to register a consumer for '%s': %w", sourceQueue, err)
	}

	for d := range msgs {
		r.logger.Debug("collected message from transient queue, processing...", "source", sourceQueue, "msgId", d.MessageId)
		err := r.republishAsDurable(&d, destExchange)
		if err != nil {
			r.logger.Error("failed to republish message as durable, requeueing", "error", err)
			_ = d.Nack(false, true)
		} else {
			r.logger.Info("message collected successfully (OUTBOUND)", "from", sourceQueue, "to", destExchange, "msgId", d.MessageId)
			metrics.MessagesProcessed.WithLabelValues("outbound", sourceQueue, destExchange).Inc()
			_ = d.Ack(false)
		}
	}

	return fmt.Errorf("consumer channel for '%s' closed", sourceQueue)
}

// republishAsDurable пере-публикует сообщение как постоянное.
func (r *RabbitMQ) republishAsDurable(msg *amqp091.Delivery, exchangeName string) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return err
	}

	defer ch.Close()
	return ch.Publish(
		exchangeName,
		"", // fanout не использует routing key
		false,
		false,
		amqp091.Publishing{
			Headers: msg.Headers,
			ContentType: msg.ContentType,
			ContentEncoding: msg.ContentEncoding,
			DeliveryMode: amqp091.Persistent,
			Priority: msg.Priority,
			CorrelationId: msg.CorrelationId,
			ReplyTo: msg.ReplyTo,
			Expiration: msg.Expiration,
			MessageId: msg.MessageId,
			Timestamp: msg.Timestamp,
			Type: msg.Type,
			UserId: msg.UserId,
			AppId: msg.AppId,
			Body: msg.Body,
		},
	)
}

// StartRouter запускает воркер для маршрутизации сообщений между двумя постоянными очередями.
func (r *RabbitMQ) StartRouter(sourceBaseName, destBaseName string) {
	workerKey := "router-" + sourceBaseName + "-to-" + destBaseName
	if r.workers[workerKey] {
		r.logger.Warn("router worker already started, skipping", "from", sourceBaseName, "to", destBaseName)
		return
	}

	sourceQueue := "durable_queue_for_" + sourceBaseName
	destExchange := "durable_exchange_for_" + destBaseName
	r.logger.Info("starting ROUTER worker", "from_queue", sourceQueue, "to_exchange", destExchange)
	r.workers[workerKey] = true
	metrics.ActiveWorkers.WithLabelValues("router").Inc()

	go func() {
		defer metrics.ActiveWorkers.WithLabelValues("router").Dec()
		for {
			err := r.routeMessages(sourceQueue, destExchange)
			r.logger.Error("router worker failed, restarting...", "from", sourceQueue, "to", destExchange, "error", err)
			metrics.ErrorsTotal.WithLabelValues("router").Inc()
			time.Sleep(5 * time.Second)
		}
	}()
}

// routeMessages - основная логика маршрутизации сообщений.
func (r *RabbitMQ) routeMessages(sourceQueue, destExchange string) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("could not open channel: %w", err)
	}

	defer ch.Close()
	msgs, err := ch.Consume(sourceQueue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to register a consumer for '%s': %w", sourceQueue, err)
	}

	for d := range msgs {
		r.logger.Debug("routing message from durable queue", "source", sourceQueue, "msgId", d.MessageId)
		err := r.republishAsDurable(&d, destExchange)
		if err != nil {
			r.logger.Error("failed to republish routed message, requeueing", "error", err)
			_ = d.Nack(false, true)
		} else {
			r.logger.Info("message routed successfully", "from", sourceQueue, "to", destExchange, "msgId", d.MessageId)
			metrics.MessagesProcessed.WithLabelValues("router", sourceQueue, destExchange).Inc()
			_ = d.Ack(false)
		}
	}

	return fmt.Errorf("consumer channel for '%s' closed", sourceQueue)
}

// Publish отправляет тестовое сообщение в указанный обменник.
func (r *RabbitMQ) Publish(exchangeName, routingKey, body string) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("could not open channel: %w", err)
	}

	defer ch.Close()
	r.logger.Info("publishing test message", "exchange", exchangeName, "routingKey", routingKey)
	err = ch.Publish(
		exchangeName,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp091.Publishing{
			ContentType: "application/json",
			DeliveryMode: amqp091.Persistent,
			Body: []byte(body),
			Timestamp: time.Now(),
		},
	)

	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}

// GetOneMessage получает одно сообщение из очереди для теста.
func (r *RabbitMQ) GetOneMessage(queueName string) (body string, ok bool, err error) {
	ch, err := r.conn.Channel()
	if err != nil {
		return "", false, fmt.Errorf("could not open channel: %w", err)
	}

	defer ch.Close()
	msg, ok, err := ch.Get(queueName, false) // autoAck = false
	if err != nil {
		return "", false, fmt.Errorf("failed to get message from '%s': %w", queueName, err)
	}

	if !ok {
		return "", false, nil // No message in queue
	}

	// Сообщение получено, подтверждаем его, чтобы оно удалилось из очереди
	_ = msg.Ack(false)
	r.logger.Info("got one test message", "queue", queueName, "message_id", msg.MessageId)

	return string(msg.Body), true, nil
}
