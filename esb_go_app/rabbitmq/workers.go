package rabbitmq

import (
	"fmt"
	"strings"
	"time"

	"esb-go-app/metrics"
	"github.com/rabbitmq/amqp091-go"
)

// StartInboundForwarder starts a worker for an INBOUND channel.
// It forwards messages from the durable queue to the transient queue for 1C.
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
			time.Sleep(1 * time.Second) // Simple backoff
			err := r.forwardOneMessage(sourceQueue, destQueue)
			if err != nil {
				if err.Error() != "no message in queue" && !strings.Contains(err.Error(), "does not exist yet") {
					r.logger.Error("inbound forwarder error", "baseName", baseName, "error", err)
					metrics.ErrorsTotal.WithLabelValues("inbound").Inc()
					time.Sleep(5 * time.Second)
				}
			}
		}
	}()
}

// forwardOneMessage performs the one-shot forwarding for the Inbound worker.
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

// StartOutboundCollector starts a worker for an OUTBOUND channel.
// It collects messages from the transient 1C queue and persists them to the durable exchange.
func (r *RabbitMQ) StartOutboundCollector(baseName string) {
	workerKey := "outbound-" + baseName
	if r.workers[workerKey] {
		r.logger.Warn("outbound collector already started, skipping", "baseName", baseName)
		return
	}

	sourceQueue := baseName
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

// collectMessages is the core logic for the Outbound worker.
func (r *RabbitMQ) collectMessages(sourceQueue, destExchange string) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("could not open channel: %w", err)
	}
	defer ch.Close()

	_, err = ch.QueueDeclarePassive(sourceQueue, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("source queue '%s' does not exist yet or cannot be declared: %w", sourceQueue, err)
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
