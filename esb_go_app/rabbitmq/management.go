package rabbitmq

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// QueueInfo represents information about a queue from the RabbitMQ Management API.
type QueueInfo struct {
	Name    string `json:"name"`
	Vhost   string `json:"vhost"`
	Durable bool   `json:"durable"`
}

// ListQueues retrieves a list of all queues from the RabbitMQ Management API.
func (r *RabbitMQ) ListQueues() ([]QueueInfo, error) {
	url := fmt.Sprintf("%s/api/queues", r.cfg.ManagementDSN)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create request: %w", err)
	}
	req.SetBasicAuth(r.cfg.ManagementUser, r.cfg.ManagementPass)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not perform request to management API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rabbitmq management API returned non-200 status: %s", resp.Status)
	}

	var queues []QueueInfo
	if err := json.NewDecoder(resp.Body).Decode(&queues); err != nil {
		return nil, fmt.Errorf("could not decode queue list from management API: %w", err)
	}

	return queues, nil
}
