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
	Queues  []QueueDefinition `yaml:"queues"`
	Logging struct {
		Producer logging.LogConfig `yaml:"producer"`
	} `yaml:"logging"`
}

// Our struct that we will send
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

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	eventName := flag.String("name", "", "Event name")
	eventMsg := flag.String("msg", "", "Event message")
	flag.Parse()

	if *eventName == "" || *eventMsg == "" {
		fmt.Println("Event name and message are required")
		flag.Usage()
		os.Exit(1)
	}

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

	// Create event
	event := Event{
		Name: *eventName,
		Msg:  *eventMsg,
	}

	// Marshal event to JSON
	eventJSON, err := json.Marshal(event)
	if err != nil {
		logger.Error("Failed to marshal event: %v", err)
		panic(err)
	}

	// Encrypt the event data
	encryptedData, err := cryptor.Encrypt(eventJSON)
	if err != nil {
		logger.Error("Failed to encrypt event data: %v", err)
		panic(err)
	}

	// Generate a unique message ID
	messageID := uuid.New().String()

	// Publish to all configured queues
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
