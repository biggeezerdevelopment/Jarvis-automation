package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"jarvis/pkg/crypto"
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
	Queues []QueueDefinition `yaml:"queues"`
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

func handleMessage(cryptor *crypto.Cryptor) messaging.MessageHandler {
	return func(delivery amqp.Delivery) {
		fmt.Printf("Queue [%s] Event: %s\n", delivery.ConsumerTag, delivery.CorrelationId)
		fmt.Printf("Headers: %v\n", delivery.Headers)

		// Decrypt the received message
		decryptedData, err := cryptor.Decrypt(string(delivery.Body))
		if err != nil {
			fmt.Printf("Failed to decrypt message: %v\n", err)
			delivery.Ack(false)
			return
		}

		// Try to parse the event
		var event Event
		if err := json.Unmarshal(decryptedData, &event); err != nil {
			fmt.Printf("Failed to parse event: %v\n", err)
			delivery.Ack(false)
			return
		}

		fmt.Printf("Received event: Name=%s, Message=%s\n", event.Name, event.Msg)
		delivery.Ack(true)
	}
}

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	cryptor := crypto.NewCryptor(encryptionKey)

	config, err := loadConfig(*configPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to load config: %v", err))
	}

	// Create AMQP client
	amqpConfig := &messaging.AMQPConfig{
		Username: config.AMQP.Username,
		Password: config.AMQP.Password,
		Host:     config.AMQP.Host,
		VHost:    config.AMQP.VHost,
	}

	client, err := messaging.NewClient(amqpConfig)
	if err != nil {
		panic(fmt.Sprintf("Failed to create AMQP client: %v", err))
	}
	defer client.Close()

	// Setup handler for each queue
	messageHandler := handleMessage(cryptor)
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
			panic(fmt.Sprintf("Failed to setup consumer for queue %s: %v", queueDef.Name, err))
		}
		fmt.Printf("Started consuming from queue: %s\n", queueDef.Name)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Agent is running. Press CTRL+C to exit.")
	<-sigChan
	fmt.Println("\nShutting down...")
}
