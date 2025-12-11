package rabbitmq

import (
	"fmt"

	"github.com/rabbitmq/amqp091-go"
)
// SetupDurableTopology creates the durable part of the topology for a given channel.
// This topology is used for reliable storage of messages within the ESB.
func (r *RabbitMQ) SetupDurableTopology(baseName string) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open a channel: %w", err)
	}
	defer ch.Close()

	var _ amqp091.Delivery


	durableExchangeName := "durable_exchange_for_" + baseName
	durableQueueName := "durable_queue_for_" + baseName

	// 1. Declare a durable exchange
	r.logger.Info("declaring durable exchange", "exchange", durableExchangeName)
	err = ch.ExchangeDeclare(durableExchangeName, "fanout", true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to declare durable exchange: %w", err)
	}

	// 2. Declare a durable queue
	r.logger.Info("declaring durable queue", "queue", durableQueueName)
	_, err = ch.QueueDeclare(durableQueueName, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to declare durable queue: %w", err)
	}

	// 3. Bind them together
	r.logger.Info("binding durable queue to durable exchange", "queue", durableQueueName, "exchange", durableExchangeName)
	err = ch.QueueBind(durableQueueName, "", durableExchangeName, false, nil)
	if err != nil {
		return fmt.Errorf("failed to bind durable queue: %w", err)
	}

	r.logger.Info("durable topology setup complete", "baseName", baseName)
	return nil
}
