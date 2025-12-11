package rabbitmq

import (
	"fmt"

	"github.com/rabbitmq/amqp091-go"
)
// GetOneMessage retrieves a single message from a queue for testing purposes.
func (r *RabbitMQ) GetOneMessage(queueName string) (body string, ok bool, err error) {
	ch, err := r.conn.Channel()
	if err != nil {
		return "", false, fmt.Errorf("could not open channel: %w", err)
	}
	defer ch.Close()

	var _ amqp091.Delivery


	msg, ok, err := ch.Get(queueName, false) // autoAck = false
	if err != nil {
		return "", false, fmt.Errorf("failed to get message from '%s': %w", queueName, err)
	}

	if !ok {
		return "", false, nil // No message in queue
	}

	// Message retrieved, acknowledge it to remove it from the queue
	_ = msg.Ack(false)
	r.logger.Info("got one test message", "queue", queueName, "message_id", msg.MessageId)

	return string(msg.Body), true, nil
}
