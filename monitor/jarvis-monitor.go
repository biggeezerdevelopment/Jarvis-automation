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

type QueueDefinition struct {
	Name     string `yaml:"name"`
	Consumer string `yaml:"consumer"`
}

type Config struct {
	AMQP struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
		Host     string `yaml:"host"`
		VHost    string `yaml:"vhost"`
	} `yaml:"amqp"`
	Queues     []QueueDefinition `yaml:"queues"`
	Monitoring struct {
		Enabled  bool          `yaml:"enabled"`
		Interval time.Duration `yaml:"interval"`
	} `yaml:"monitoring"`
	Logging struct {
		Agent logging.LogConfig `yaml:"agent"`
	} `yaml:"logging"`
}

// Event represents the structure of messages we receive
type Event struct {
	Name string `json:"name"`
	Msg  string `json:"msg"`
}

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

// Test function to process a message

func handleMessage(cryptor *crypto.Cryptor, logger *logging.Logger, config *Config) messaging.MessageHandler {
	return func(delivery amqp.Delivery) {
		logger.Info("Queue [%s] Event: %s", delivery.ConsumerTag, delivery.CorrelationId)
		logger.Info("Headers: %v", delivery.Headers)

		// Decrypt the received message
		decryptedData, err := cryptor.Decrypt(string(delivery.Body))
		if err != nil {
			logger.Error("Failed to decrypt message: %v", err)
			delivery.Ack(false)
			return
		}

		logger.Info("Decrypted data: %s", decryptedData)

		// Try to parse the event
		/*
			if err := json.Unmarshal(decryptedData, &event); err != nil {
				logger.Error("Failed to parse event: %v", err)
				delivery.Ack(false)
				return
			}
		*/

		delivery.Ack(true)
	}
}

// handleMonitoringRequest processes a monitoring request and sends the response

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

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

	cryptor := crypto.NewCryptor(encryptionKey)

	// Create AMQP client
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

	// Setup handler for each queue
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

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Agent is running. Press CTRL+C to exit.")
	<-sigChan
	logger.Info("Shutting down...")
}
