package rabbitmq

import (
	"fmt"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

// EnsureExchange declares a durable fanout exchange if it doesn't already exist.
func (r *RabbitMQ) EnsureExchange(name string) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("could not open channel to ensure exchange: %w", err)
	}
	defer ch.Close()

	r.logger.Info("ensuring fanout exchange exists", "exchange_name", name)
	return ch.ExchangeDeclare(
		name,
		"fanout",
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,   // arguments
	)
}

// republishAsDurable re-publishes a message to a new exchange, ensuring it's persistent.
func (r *RabbitMQ) republishAsDurable(msg *amqp091.Delivery, exchangeName string) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	var _ amqp091.Delivery

	return ch.Publish(
		exchangeName,
		"", // fanout does not use a routing key
		false,
		false,
		amqp091.Publishing{
			Headers:         msg.Headers,
			ContentType:     msg.ContentType,
			ContentEncoding: msg.ContentEncoding,
			DeliveryMode:    amqp091.Persistent,
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
		},
	)
}

// Publish publishes a transient text message to a given exchange.
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
			ContentType:  "application/json",
			DeliveryMode: amqp091.Persistent,
			Body:         []byte(body),
			Timestamp:    time.Now(),
		},
	)

	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}
