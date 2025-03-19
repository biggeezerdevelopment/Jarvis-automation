// Package main implements the Jarvis agent, a multi-purpose system agent that handles
// various system operations including system monitoring, remote commands, and event processing.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jarvis/pkg/crypto"
	"jarvis/pkg/logging"
	"jarvis/pkg/messaging"
	"jarvis/pkg/monitoring"

	amqp "github.com/rabbitmq/amqp091-go"
	"gopkg.in/yaml.v3"
)

// encryption key should be 32 bytes for AES-256
var encryptionKey = []byte("12345678901234567890123456789012")

// QueueDefinition represents a RabbitMQ queue configuration.
// It includes the queue name and consumer identifier.
type QueueDefinition struct {
	Name     string `yaml:"name"`     // Name of the queue
	Consumer string `yaml:"consumer"` // Identifier for the consumer
	IsRemote bool   `yaml:"remote"`   // Whether this is a remote queue for responses
}

// Config represents the complete configuration for the Jarvis agent.
// It includes AMQP connection details, queue definitions, monitoring settings,
// and logging configuration.
type Config struct {
	AMQP struct {
		Username string `yaml:"username"` // RabbitMQ username
		Password string `yaml:"password"` // RabbitMQ password
		Host     string `yaml:"host"`     // RabbitMQ host address
		VHost    string `yaml:"vhost"`    // RabbitMQ virtual host
	} `yaml:"amqp"`
	Queues     []QueueDefinition `yaml:"queues"` // List of queues to consume from
	Monitoring struct {
		Enabled  bool          `yaml:"enabled"`  // Whether monitoring is enabled
		Interval time.Duration `yaml:"interval"` // Interval between metrics collections
	} `yaml:"monitoring"`
	Logging struct {
		Agent logging.LogConfig `yaml:"agent"` // Logging configuration for the agent
	} `yaml:"logging"`
}

// Event represents the structure of messages received by the agent.
// Each event has a name that determines its type and a message payload.
type Event struct {
	Name   string `json:"name"`   // Type of event (e.g., "get_metrics", "reboot_server")
	Msg    string `json:"msg"`    // Event payload or parameters
	Remote string `json:"remote"` // Whether the event is remote
}

// loadConfig reads and parses the configuration file from the specified path.
// It returns a pointer to the Config structure and any error encountered.
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// handleRebootRequest processes a system reboot request.
// This is a placeholder function that currently only logs the request.
// In a production environment, this would implement actual system reboot logic.
func handleRebootRequest(data string, logger *logging.Logger) {
	logger.Info("Rebooting the system: %s", data)
}

// sendToRemoteQueue sends an encrypted message to a remote queue.
// Parameters:
//   - queueName: Name of the remote queue to send to
//   - correlationID: ID to correlate the message with a request
//   - data: Data to encrypt and send
//   - cryptor: For encrypting the data
//   - logger: For logging operations
//   - config: System configuration
//
// Returns an error if the message cannot be sent.
func sendToRemoteQueue(queueName, correlationID string, data interface{}, cryptor *crypto.Cryptor, logger *logging.Logger, config *Config) error {
	// Setup AMQP client for response
	amqpConfig := &messaging.AMQPConfig{
		Username: config.AMQP.Username,
		Password: config.AMQP.Password,
		Host:     config.AMQP.Host,
		VHost:    config.AMQP.VHost,
	}

	client, err := messaging.NewClient(amqpConfig)
	if err != nil {
		return fmt.Errorf("failed to create AMQP client: %v", err)
	}
	defer client.Close()

	// Convert data to JSON if it's not already a string or []byte
	var dataBytes []byte
	switch v := data.(type) {
	case string:
		dataBytes = []byte(v)
	case []byte:
		dataBytes = v
	default:
		jsonData, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to marshal data: %v", err)
		}
		dataBytes = jsonData
	}

	// Encrypt the data
	encryptedData, err := cryptor.Encrypt(dataBytes)
	if err != nil {
		return fmt.Errorf("failed to encrypt data: %v", err)
	}

	// Send the message
	err = client.PublishMessage(messaging.PublishConfig{
		Queue:         queueName,
		CorrelationID: correlationID,
		Body:          []byte(encryptedData),
	})
	if err != nil {
		return fmt.Errorf("failed to publish message: %v", err)
	}

	logger.Info("Successfully sent message to remote queue %s (ID: %s)", queueName, correlationID)
	return nil
}

// handleMonitoringRequest processes a monitoring request and sends system metrics.
// Parameters:
//   - responseQueue: Queue to send the metrics response to
//   - correlationID: ID to correlate the response with the request
//   - cryptor: For encrypting the metrics data
//   - logger: For logging operations
//   - config: System configuration
func handleMonitoringRequest(responseQueue, correlationID string, cryptor *crypto.Cryptor, logger *logging.Logger, config *Config) {
	// Create monitor with 1-second interval for measurements
	monitor := monitoring.NewMonitor(1 * time.Second)

	// Collect system metrics
	metrics, err := monitor.GetMetrics()
	if err != nil {
		logger.Error("Failed to get system metrics: %v", err)
		return
	}

	// Prepare metrics data
	metricsJSON, err := metrics.ToJSON()
	if err != nil {
		logger.Error("Failed to convert metrics to JSON: %v", err)
		return
	}

	// Send metrics to remote queue
	err = sendToRemoteQueue(responseQueue, correlationID, metricsJSON, cryptor, logger, config)
	if err != nil {
		logger.Error("Failed to send metrics to remote queue: %v", err)
		return
	}
}

// handleMessage creates a message handler function that processes incoming AMQP messages.
// It handles different types of events including monitoring requests and system commands.
// Parameters:
//   - cryptor: For encrypting/decrypting messages
//   - logger: For logging operations
//   - config: System configuration
//
// Returns a MessageHandler function that processes AMQP deliveries.
func handleMessage(cryptor *crypto.Cryptor, logger *logging.Logger, config *Config) messaging.MessageHandler {
	return func(delivery amqp.Delivery) {
		logger.Info("Queue [%s] Event: %s", delivery.ConsumerTag, delivery.CorrelationId)
		logger.Info("Headers: %v", delivery.Headers)

		// Decrypt and validate the message
		decryptedData, err := cryptor.Decrypt(string(delivery.Body))
		if err != nil {
			logger.Error("Failed to decrypt message: %v", err)
			delivery.Ack(false)
			return
		}

		logger.Info("Decrypted data: %s", decryptedData)

		// Parse the event
		var event Event
		if err := json.Unmarshal(decryptedData, &event); err != nil {
			logger.Error("Failed to parse event: %v", err)
			delivery.Ack(false)
			return
		}
		responseQueue := event.Remote
		// Route the event to appropriate handler
		switch event.Name {
		case "get_metrics":
			// Use response queue from headers or event message

			if responseQueue != "" {
				handleMonitoringRequest(responseQueue, delivery.CorrelationId, cryptor, logger, config)
			} else {
				logger.Error("No response queue specified for metrics request")
			}
			delivery.Ack(true)
		case "reboot_server":
			handleRebootRequest(event.Msg, logger)
			// If response queue is specified, send acknowledgment
			if responseQueue != "" {
				response := map[string]string{"status": "reboot_initiated"}
				responseJSON, _ := json.Marshal(response)
				err = sendToRemoteQueue(responseQueue, delivery.CorrelationId, responseJSON, cryptor, logger, config)
				if err != nil {
					logger.Error("Failed to send reboot response: %v", err)
				}
			}
			delivery.Ack(true)
		default:
			logger.Info("Received event: Name=%s, Message=%s", event.Name, event.Msg)
			delivery.Ack(true)
		}
	}
}

// main is the entry point of the Jarvis agent.
// It initializes the agent, sets up message handlers, and starts processing events.
// The agent will run until interrupted by a system signal (CTRL+C).
func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	// Load configuration
	config, err := loadConfig(*configPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to load config: %v", err))
	}

	// Initialize logger
	logger, err := logging.NewLoggerWithConfig(&config.Logging.Agent)
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
	defer logger.Close()

	// Initialize components
	cryptor := crypto.NewCryptor(encryptionKey)
	amqpConfig := &messaging.AMQPConfig{
		Username: config.AMQP.Username,
		Password: config.AMQP.Password,
		Host:     config.AMQP.Host,
		VHost:    config.AMQP.VHost,
	}

	// Setup AMQP client
	client, err := messaging.NewClient(amqpConfig)
	if err != nil {
		logger.Error("Failed to create AMQP client: %v", err)
		panic(err)
	}
	defer client.Close()

	// Setup message handlers for each queue
	messageHandler := handleMessage(cryptor, logger, config)
	for _, queueDef := range config.Queues {
		consumerConfig := messaging.ConsumerConfig{
			Queue: messaging.QueueConfig{
				Name:     queueDef.Name,
				Consumer: queueDef.Consumer,
			},
			PrefetchCount: 50,
			Handler:       messageHandler,
		}

		if err := client.AddConsumer(consumerConfig); err != nil {
			logger.Error("Failed to setup consumer for queue %s: %v", queueDef.Name, err)
			panic(err)
		}
		logger.Info("Started consuming from queue: %s", queueDef.Name)
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Agent is running. Press CTRL+C to exit.")
	<-sigChan
	logger.Info("Shutting down...")
}
