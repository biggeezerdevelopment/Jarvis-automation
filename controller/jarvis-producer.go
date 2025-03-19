package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"jarvis/pkg/crypto"
	"jarvis/pkg/messaging"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

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

// Our struct that we will send
type Event struct {
	Name string
	Msg  string
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
	eventName := flag.String("name", "Default Name", "Name of the event to send")
	eventMsg := flag.String("msg", "Hello from default name", "Message to send")
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

	// Declare queue
	if err := client.DeclareStreamQueue(config.Queues[0].Name); err != nil {
		panic(fmt.Sprintf("Failed to declare queue: %v", err))
	}

	// Create and encrypt event
	event := Event{
		Name: *eventName,
		Msg:  *eventMsg,
	}
	data, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}

	encryptedData, err := cryptor.Encrypt(data)
	if err != nil {
		panic(err)
	}

	// Publish message
	ctx := context.Background()
	if err := client.PublishMessage(ctx, config.Queues[0].Name, uuid.NewString(), []byte(encryptedData)); err != nil {
		panic(fmt.Sprintf("Failed to publish message: %v", err))
	}

	fmt.Printf("Successfully published message to queue: %s\n", config.Queues[0].Name)
}
