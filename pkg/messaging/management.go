package messaging

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// QueueInfo represents information about a RabbitMQ queue
type QueueInfo struct {
	Name      string `json:"name"`      // Queue name
	Messages  int    `json:"messages"`  // Number of messages in the queue
	Consumers int    `json:"consumers"` // Number of consumers
	State     string `json:"state"`     // Queue state (running, idle, etc.)
	Node      string `json:"node"`      // Node name (for clustered setups)
}

// ManagementConfig holds the configuration for RabbitMQ management API
type ManagementConfig struct {
	Username string
	Password string
	Host     string
	Port     int
}

// NewManagementConfig creates a new management configuration
func NewManagementConfig(username, password, host string, port int) *ManagementConfig {
	return &ManagementConfig{
		Username: username,
		Password: password,
		Host:     host,
		Port:     port,
	}
}

// ListQueues retrieves information about all queues from RabbitMQ using the management API
func (c *ManagementConfig) ListQueues() ([]QueueInfo, error) {
	// Construct the management API URL
	url := fmt.Sprintf("http://%s:%d/api/queues", c.Host, c.Port)

	// Create HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set basic auth
	req.SetBasicAuth(c.Username, c.Password)

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("management API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var queues []QueueInfo
	if err := json.Unmarshal(body, &queues); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	return queues, nil
}

// GetQueueInfo retrieves information about a specific queue
func (c *ManagementConfig) GetQueueInfo(queueName string) (*QueueInfo, error) {
	// Construct the management API URL
	url := fmt.Sprintf("http://%s:%d/api/queues/%s", c.Host, c.Port, queueName)

	// Create HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set basic auth
	req.SetBasicAuth(c.Username, c.Password)

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("management API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var queue QueueInfo
	if err := json.Unmarshal(body, &queue); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	return &queue, nil
}
