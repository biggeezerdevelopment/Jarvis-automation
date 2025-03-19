package monitoring

import (
	"encoding/json"
	"time"

	"jarvis/pkg/crypto"
	"jarvis/pkg/logging"
	"jarvis/pkg/messaging"

	amqp "github.com/rabbitmq/amqp091-go"
)

// MonitoringRequest represents a request for system metrics
type MonitoringRequest struct {
	RequestID string `json:"request_id"`
	Type      string `json:"type"`     // Type of metrics requested (e.g., "cpu")
	Response  string `json:"response"` // Queue to send response to
}

// Service handles monitoring operations
type Service struct {
	monitor      *Monitor
	client       *messaging.Client
	cryptor      *crypto.Cryptor
	logger       *logging.Logger
	requestQueue string
}

// ServiceConfig holds configuration for the monitoring service
type ServiceConfig struct {
	RequestQueue string
	Client       *messaging.Client
	Cryptor      *crypto.Cryptor
	Logger       *logging.Logger
	Interval     time.Duration
}

// NewService creates a new monitoring service
func NewService(config ServiceConfig) *Service {
	return &Service{
		monitor:      NewMonitor(config.Interval),
		client:       config.Client,
		cryptor:      config.Cryptor,
		logger:       config.Logger,
		requestQueue: config.RequestQueue,
	}
}

// Start begins listening for monitoring requests
func (s *Service) Start() error {
	s.logger.Info("Starting monitoring service, listening on queue: %s", s.requestQueue)

	return s.client.AddConsumer(messaging.ConsumerConfig{
		Queue: messaging.QueueConfig{
			Name:     s.requestQueue,
			Consumer: "monitoring_consumer",
		},
		PrefetchCount: 1,
		Handler:       s.handleRequest,
	})
}

// handleRequest processes incoming monitoring requests
func (s *Service) handleRequest(delivery amqp.Delivery) {
	s.logger.Info("Received monitoring request: %s", delivery.CorrelationId)

	// Decrypt the request
	decryptedData, err := s.cryptor.Decrypt(string(delivery.Body))
	if err != nil {
		s.logger.Error("Failed to decrypt request: %v", err)
		delivery.Ack(false)
		return
	}

	// Parse the request
	var request MonitoringRequest
	if err := json.Unmarshal(decryptedData, &request); err != nil {
		s.logger.Error("Failed to parse request: %v", err)
		delivery.Ack(false)
		return
	}

	// Get system metrics
	metrics, err := s.monitor.GetMetrics()
	if err != nil {
		s.logger.Error("Failed to get system metrics: %v", err)
		delivery.Ack(false)
		return
	}

	// Convert metrics to JSON
	metricsJSON, err := metrics.ToJSON()
	if err != nil {
		s.logger.Error("Failed to convert metrics to JSON: %v", err)
		delivery.Ack(false)
		return
	}

	// Encrypt the metrics
	encryptedData, err := s.cryptor.Encrypt(metricsJSON)
	if err != nil {
		s.logger.Error("Failed to encrypt metrics: %v", err)
		delivery.Ack(false)
		return
	}

	// Publish the response
	err = s.client.PublishMessage(messaging.PublishConfig{
		Queue:         request.Response,
		CorrelationID: request.RequestID,
		Body:          []byte(encryptedData),
	})
	if err != nil {
		s.logger.Error("Failed to publish response: %v", err)
		delivery.Ack(false)
		return
	}

	s.logger.Info("Successfully sent metrics response to queue: %s", request.Response)
	delivery.Ack(true)
}
