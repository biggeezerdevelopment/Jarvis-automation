// Package main implements the Jarvis producer, which is responsible for sending
// encrypted events to the Jarvis system through RabbitMQ queues. The producer
// supports multiple queues and provides secure message delivery with encryption.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"jarvis/pkg/crypto"
	"jarvis/pkg/logging"
	"jarvis/pkg/messaging"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// encryption key should be 32 bytes for AES-256
var encryptionKey = []byte("12345678901234567890123456789012")

// QueueDefinition represents a RabbitMQ queue configuration.
// It includes the queue name and consumer identifier for message routing.
type QueueDefinition struct {
	Name     string `yaml:"name"`     // Name of the queue to publish to
	Consumer string `yaml:"consumer"` // Identifier for the queue consumer
}

// Config represents the complete configuration for the Jarvis producer.
// It includes AMQP connection details, queue definitions, and logging configuration.
type Config struct {
	AMQP struct {
		Username string `yaml:"username"` // RabbitMQ username
		Password string `yaml:"password"` // RabbitMQ password
		Host     string `yaml:"host"`     // RabbitMQ host address
		VHost    string `yaml:"vhost"`    // RabbitMQ virtual host
	} `yaml:"amqp"`
	Queues  []QueueDefinition `yaml:"queues"` // List of queues to publish to
	Logging struct {
		Producer logging.LogConfig `yaml:"producer"` // Logging configuration for the producer
	} `yaml:"logging"`
}

// Event represents the structure of messages sent by the producer.
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

// listQueues connects to RabbitMQ and retrieves a list of all queues.
// Parameters:
//   - config: AMQP configuration
//   - logger: For logging operations
func listQueues(config *Config, logger *logging.Logger) error {
	// Create management API client
	mgmtConfig := messaging.NewManagementConfig(
		config.AMQP.Username,
		config.AMQP.Password,
		config.AMQP.Host,
		15672, // Default RabbitMQ management port
	)

	// Get all queues
	queues, err := mgmtConfig.ListQueues()
	if err != nil {
		return fmt.Errorf("failed to list queues: %v", err)
	}

	// Print queue information
	fmt.Println("\nRabbitMQ Queues:")
	fmt.Println("----------------")
	for _, queue := range queues {
		fmt.Printf("Name: %s\n", queue.Name)
		fmt.Printf("  Messages: %d\n", queue.Messages)
		fmt.Printf("  Consumers: %d\n", queue.Consumers)
		fmt.Printf("  State: %s\n", queue.State)
		if queue.Node != "" {
			fmt.Printf("  Node: %s\n", queue.Node)
		}
		fmt.Println("----------------")
	}

	return nil
}

// createQueue creates a new queue in RabbitMQ
func createQueue(config *Config, logger *logging.Logger, queueName string) error {
	// Setup AMQP client
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

	// Declare the queue
	_, err = client.DeclareQueue(queueName)
	if err != nil {
		return fmt.Errorf("failed to create queue: %v", err)
	}

	logger.Info("Successfully created queue: %s", queueName)
	return nil
}

// deleteQueue deletes a queue from RabbitMQ
func deleteQueue(config *Config, logger *logging.Logger, queueName string) error {
	// Setup AMQP client
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

	// Delete the queue
	err = client.DeleteQueue(queueName)
	if err != nil {
		return fmt.Errorf("failed to delete queue: %v", err)
	}

	logger.Info("Successfully deleted queue: %s", queueName)
	return nil
}

// main is the entry point of the Jarvis producer.
// It processes command-line arguments, initializes components, and publishes
// the specified event to all configured queues with encryption.
func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to config file")
	eventName := flag.String("name", "", "Event name")
	eventMsg := flag.String("msg", "", "Event message")
	eventRemote := flag.String("remote", "Tester", "Event remote")
	listQueuesFlag := flag.Bool("list-queues", false, "List all queues from RabbitMQ")
	createQueueFlag := flag.String("create-queue", "", "Create a new queue with the specified name")
	deleteQueueFlag := flag.String("delete-queue", "", "Delete a queue with the specified name")
	flag.Parse()

	// Load configuration
	config, err := loadConfig(*configPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to load config: %v", err))
	}

	// Initialize logger with config
	logger, err := logging.NewLoggerWithConfig(&config.Logging.Producer)
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
	defer logger.Close()

	// Start log rotation checker in background
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			if err := logger.RotateLogFile(); err != nil {
				logger.Error("Failed to rotate log file: %v", err)
			}
		}
	}()

	// Handle delete queue command
	if *deleteQueueFlag != "" {
		if err := deleteQueue(config, logger, *deleteQueueFlag); err != nil {
			logger.Error("Failed to delete queue: %v", err)
			os.Exit(1)
		}
		return
	}

	// Handle create queue command
	if *createQueueFlag != "" {
		if err := createQueue(config, logger, *createQueueFlag); err != nil {
			logger.Error("Failed to create queue: %v", err)
			os.Exit(1)
		}
		return
	}

	// Handle list queues command
	if *listQueuesFlag {
		if err := listQueues(config, logger); err != nil {
			logger.Error("Failed to list queues: %v", err)
			os.Exit(1)
		}
		return
	}

	// Validate required arguments for event publishing
	if *eventName == "" {
		fmt.Println("Event name is required")
		flag.Usage()
		os.Exit(1)
	}

	// Initialize components
	cryptor := crypto.NewCryptor(encryptionKey)

	// Setup AMQP client
	amqpConfig := &messaging.AMQPConfig{
		Username: config.AMQP.Username,
		Password: config.AMQP.Password,
		Host:     config.AMQP.Host,
		VHost:    config.AMQP.VHost,
	}

	client, err := messaging.NewClient(amqpConfig)
	if err != nil {
		logger.Error("Failed to create AMQP client: %v", err)
		panic(err)
	}
	defer client.Close()

	// Create and marshal event
	event := Event{
		Name:   *eventName,
		Msg:    *eventMsg,
		Remote: *eventRemote,
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		logger.Error("Failed to marshal event: %v", err)
		panic(err)
	}

	// Encrypt the event data for secure transmission
	encryptedData, err := cryptor.Encrypt(eventJSON)
	if err != nil {
		logger.Error("Failed to encrypt event data: %v", err)
		panic(err)
	}

	// Generate a unique message ID for tracking
	messageID := uuid.New().String()

	// Publish to all configured queues with the same message ID
	for _, queue := range config.Queues {
		err = client.PublishMessage(messaging.PublishConfig{
			Queue:         queue.Name,
			CorrelationID: messageID,
			Body:          []byte(encryptedData),
		})
		if err != nil {
			logger.Error("Failed to publish message to queue %s: %v", queue.Name, err)
			continue
		}
		logger.Info("Successfully published message (ID: %s) to queue: %s", messageID, queue.Name)
	}
}
