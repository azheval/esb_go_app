package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"esb-go-app/metrics"
	"esb-go-app/storage"
)

// StartRouter starts a worker for a specific route.
// sourceID is either a channel ID or a collector ID prefixed with "collector-output:"
func (r *RabbitMQ) StartRouter(routeID, routeName, sourceID string) {
	workerKey := "router-" + routeID
	if r.workers[workerKey] {
		r.logger.Warn("router worker already started, skipping", "route_id", routeID)
		return
	}

	var sourceQueue string
	isFanout := false

	// Determine source type and fanout mode
	if strings.HasPrefix(sourceID, "collector-output:") {
		// Collectors are always fanout
		isFanout = true
		sourceExchange := sourceID
		r.logger.Info("starting ROUTER from collector (fanout mode)", "route_id", routeID, "from_exchange", sourceExchange)

		sourceQueue = fmt.Sprintf("route_fanout_queue_for_%s_%s", routeName, routeID)
		if err := r.setupFanoutSubscription(sourceExchange, sourceQueue); err != nil {
			r.logger.Error("failed to setup fanout route topology for collector", "route_id", routeID, "exchange", sourceExchange, "error", err)
			return
		}
	} else {
		// This is a route from a standard channel, we need to check its mode
		sourceChannel, err := r.dataStore.GetChannelByID(sourceID)
		if err != nil || sourceChannel == nil {
			r.logger.Error("failed to get source channel for router start", "error", err, "channel_id", sourceID)
			return
		}

		isFanout = sourceChannel.FanoutMode
		if isFanout {
			sourceExchange := "durable_exchange_for_" + sourceChannel.Destination
			sourceQueue = fmt.Sprintf("route_fanout_queue_for_%s_%s", routeName, routeID)
			r.logger.Info("starting ROUTER from channel (fanout mode)", "route_id", routeID, "from_exchange", sourceExchange)
			if err := r.setupFanoutSubscription(sourceExchange, sourceQueue); err != nil {
				r.logger.Error("failed to setup fanout route topology for channel", "route_id", routeID, "exchange", sourceExchange, "error", err)
				return
			}
		} else {
			sourceQueue = "durable_queue_for_" + sourceChannel.Destination
			r.logger.Info("starting ROUTER from channel (direct mode)", "route_id", routeID, "from_queue", sourceQueue)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.stoppersMu.Lock()
	r.stoppers[workerKey] = cancel
	r.stoppersMu.Unlock()

	r.workers[workerKey] = true
	metrics.ActiveWorkers.WithLabelValues("router").Inc()

	go func() {
		defer metrics.ActiveWorkers.WithLabelValues("router").Dec()
		for {
			select {
			case <-ctx.Done():
				r.logger.Info("router worker stopping before loop", "route_id", routeID)
				return
			default:
			}

			err := r.routeMessageLoop(ctx, routeID, sourceQueue)
			if err != nil {
				if ctx.Err() == context.Canceled {
					r.logger.Info("router worker gracefully stopped.", "route_id", routeID)
					return
				}
				r.logger.Error("router worker failed, restarting...", "route_id", routeID, "error", err)
				metrics.ErrorsTotal.WithLabelValues("router").Inc()
			}

			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				r.logger.Info("router worker stopping during backoff.", "route_id", routeID)
				return
			}
		}
	}()
}

// setupFanoutSubscription ensures a unique queue exists and is bound to a fanout exchange.
// This is used for collectors and channels in FanoutMode.
func (r *RabbitMQ) setupFanoutSubscription(exchangeName, queueName string) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("could not open channel: %w", err)
	}
	defer ch.Close()

	// 1. Declare the fanout exchange (idempotent)
	// This ensures it exists, whether it's from a collector or a durable channel topology.
	err = ch.ExchangeDeclare(
		exchangeName,
		"fanout", // Fanout for broadcast to all listening routes
		true,     // durable
		false,    // auto-delete
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare fanout exchange '%s': %w", exchangeName, err)
	}

	// 2. Declare a durable, non-exclusive, non-autodelete queue for this specific route's subscription
	_, err = ch.QueueDeclare(
		queueName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare subscription queue '%s': %w", queueName, err)
	}

	// 3. Bind the queue to the exchange
	err = ch.QueueBind(
		queueName,
		"", // routing key (not used for fanout)
		exchangeName,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind queue '%s' to exchange '%s': %w", queueName, exchangeName, err)
	}

	r.logger.Info("successfully set up fanout subscription", "exchange", exchangeName, "queue", queueName)
	return nil
}

// routeMessageLoop is the core logic for routing a single message.
func (r *RabbitMQ) routeMessageLoop(ctx context.Context, routeID, sourceQueue string) error {
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("could not open channel: %w", err)
	}
	defer ch.Close()

	msgs, err := ch.ConsumeWithContext(ctx, sourceQueue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to register a consumer for '%s': %w", sourceQueue, err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-msgs:
			if !ok {
				return fmt.Errorf("consumer channel for '%s' closed", sourceQueue)
			}

			var route *storage.Route
			var getRouteErr error
			// Use a simple retry mechanism for fetching route details
			for i := 0; i < 3; i++ {
				route, getRouteErr = r.dataStore.GetRouteByID(routeID)
				if getRouteErr == nil && route != nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			if getRouteErr != nil || route == nil {
				r.logger.Error("failed to get route details after retries, requeueing", "route_id", routeID, "error", getRouteErr)
				_ = d.Nack(false, true)
				continue
			}

			if route.DestinationChannelID == nil || *route.DestinationChannelID == "" {
				r.logger.Error("route has no destination channel, dead-lettering", "route_id", routeID)
				_ = d.Nack(false, false)
				continue
			}
			destChannel, err := r.dataStore.GetChannelByID(*route.DestinationChannelID)
			if err != nil || destChannel == nil {
				r.logger.Error("failed to get destination channel for route, requeueing", "route_id", routeID, "error", err)
				_ = d.Nack(false, true)
				continue
			}

			finalDestExchange := "durable_exchange_for_" + destChannel.Destination
			finalBody := d.Body // Default to original body

			if route.RouteType == "transform" {
				r.logger.Debug("performing transformation for route", "route_id", routeID)

				if route.TransformationID == nil || *route.TransformationID == "" {
					r.logger.Error("transformation route has no transformation ID, dead-lettering", "route_id", routeID)
					_ = d.Nack(false, false)
					continue
				}

				transform, err := r.dataStore.GetTransformationByID(*route.TransformationID)
				if err != nil || transform == nil {
					r.logger.Error("failed to get transformation details, dead-lettering", "transformation_id", *route.TransformationID, "error", err)
					_ = d.Nack(false, false)
					continue
				}

				var bodyMap map[string]interface{}
				if err := json.Unmarshal(d.Body, &bodyMap); err != nil {
					r.logger.Error("failed to unmarshal message body for transformation, dead-lettering", "msg_id", d.MessageId, "error", err)
					_ = d.Nack(false, false)
					continue
				}

				headersMap := make(map[string]interface{})
				for k, v := range d.Headers {
					headersMap[k] = v
				}

				transformedMsg, err := r.scriptingService.ExecuteScript(transform.Engine, transform.Script, bodyMap, headersMap)
				if err != nil {
					r.logger.Error("failed to execute transformation script, dead-lettering", "transformation_id", transform.ID, "error", err)
					_ = d.Nack(false, false)
					continue
				}

				if transformedMsg == nil || transformedMsg.Body == nil {
					r.logger.Info("transformation script returned nil, message filtered", "route_id", routeID, "transformation_id", transform.ID)
					_ = d.Ack(false) // Acknowledge and drop
					continue
				}

				newBodyBytes, err := json.Marshal(transformedMsg.Body)
				if err != nil {
					r.logger.Error("failed to marshal transformed message body, dead-lettering", "msg_id", d.MessageId, "error", err)
					_ = d.Nack(false, false)
					continue
				}
				finalBody = newBodyBytes
			}

			// Republish logic
			republishDelivery := d
			republishDelivery.Body = finalBody

			err = r.republishAsDurable(&republishDelivery, finalDestExchange)
			if err != nil {
				r.logger.Error("failed to republish routed message, requeueing", "error", err)
				_ = d.Nack(false, true)
			} else {
				r.logger.Info("message routed successfully", "from", sourceQueue, "to", finalDestExchange, "msgId", d.MessageId)
				metrics.MessagesProcessed.WithLabelValues("router", sourceQueue, finalDestExchange).Inc()
				_ = d.Ack(false)
			}
		}
	}
}
