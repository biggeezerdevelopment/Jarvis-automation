// Package main implements the Jarvis monitoring service, which provides a dedicated
// monitoring interface for system metrics collection and reporting. This service
// complements the monitoring capabilities of the main Jarvis agent.
package main

import (
	//ncoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jarvis/pkg/crypto"
	"jarvis/pkg/logging"
	"jarvis/pkg/messaging"

	amqp "github.com/rabbitmq/amqp091-go"
	"gopkg.in/yaml.v3"
)

// encryption key should be 32 bytes for AES-256
var encryptionKey = []byte("12345678901234567890123456789012")

// QueueDefinition represents a RabbitMQ queue configuration.
// It includes the queue name and consumer identifier for message handling.
type QueueDefinition struct {
	Name     string `yaml:"name"`     // Name of the queue to monitor
	Consumer string `yaml:"consumer"` // Identifier for the queue consumer
}

// Config represents the complete configuration for the Jarvis monitor.
// It includes AMQP connection details, queue definitions, monitoring settings,
// and logging configuration.
type Config struct {
	AMQP struct {
		Username string `yaml:"username"` // RabbitMQ username
		Password string `yaml:"password"` // RabbitMQ password
		Host     string `yaml:"host"`     // RabbitMQ host address
		VHost    string `yaml:"vhost"`    // RabbitMQ virtual host
	} `yaml:"amqp"`
	Queues     []QueueDefinition `yaml:"queues"` // List of queues to monitor
	Monitoring struct {
		Enabled  bool          `yaml:"enabled"`  // Whether monitoring is enabled
		Interval time.Duration `yaml:"interval"` // Interval between monitoring checks
	} `yaml:"monitoring"`
	Logging struct {
		Agent logging.LogConfig `yaml:"agent"` // Logging configuration for the monitor
	} `yaml:"logging"`
}

// Event represents the structure of messages received by the monitor.
// Each event has a name that determines its type and a message payload.
type Event struct {
	Name string `json:"name"` // Type of event to process
	Msg  string `json:"msg"`  // Event payload or parameters
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

// handleMessage creates a message handler function that processes incoming AMQP messages.
// This handler focuses on monitoring-related messages and metrics collection.
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

		// Note: Event parsing is currently commented out
		// This section can be expanded to handle specific monitoring events
		// such as configuration updates, monitoring interval changes, etc.

		delivery.Ack(true)
	}
}

// main is the entry point of the Jarvis monitor.
// It initializes the monitoring service, sets up message handlers, and starts
// processing monitoring-related events. The monitor will run until interrupted
// by a system signal (CTRL+C).
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

	// Setup message handlers for each monitored queue
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
		logger.Info("Started monitoring queue: %s", queueDef.Name)
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Monitor is running. Press CTRL+C to exit.")
	<-sigChan
	logger.Info("Shutting down monitoring service...")
}
