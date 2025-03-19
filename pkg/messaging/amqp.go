package messaging

import (
	"context"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// AMQPConfig holds the configuration for AMQP connection
type AMQPConfig struct {
	Username string
	Password string
	Host     string
	VHost    string
}

// QueueConfig holds the configuration for AMQP queue
type QueueConfig struct {
	Name     string
	Consumer string
}

// MessageHandler is a function type for processing messages
type MessageHandler func(delivery amqp.Delivery)

// ConsumerConfig holds configuration for a consumer
type ConsumerConfig struct {
	Queue         QueueConfig
	PrefetchCount int
	Handler       MessageHandler
}

// Client represents an AMQP client
type Client struct {
	conn      *amqp.Connection
	channel   *amqp.Channel
	config    *AMQPConfig
	consumers map[string]<-chan amqp.Delivery
	wg        sync.WaitGroup
	mu        sync.Mutex
}

// NewClient creates a new AMQP client
func NewClient(config *AMQPConfig) (*Client, error) {
	conn, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s/%s",
		config.Username,
		config.Password,
		config.Host,
		config.VHost))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %v", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %v", err)
	}

	return &Client{
		conn:      conn,
		channel:   ch,
		config:    config,
		consumers: make(map[string]<-chan amqp.Delivery),
	}, nil
}

// DeclareStreamQueue declares a stream queue with the given configuration
func (c *Client) DeclareStreamQueue(queueName string) error {
	_, err := c.channel.QueueDeclare(
		queueName,
		true,  // durable
		false, // autoDelete
		false, // exclusive
		true,  // noWait
		amqp.Table{
			"x-queue-type":                    "stream",
			"x-stream-max-segment-size-bytes": 30000,  // 0.03 MB
			"x-max-length-bytes":              150000, // 0.15 MB
		},
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue: %v", err)
	}
	return nil
}

// AddConsumer adds a new consumer with the given configuration
func (c *Client) AddConsumer(config ConsumerConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.channel.Qos(config.PrefetchCount, 0, false); err != nil {
		return fmt.Errorf("failed to set QoS: %v", err)
	}

	stream, err := c.channel.Consume(
		config.Queue.Name,
		config.Queue.Consumer,
		false, // autoAck
		false, // exclusive
		false, // noLocal
		false, // noWait
		amqp.Table{},
	)
	if err != nil {
		return fmt.Errorf("failed to create consumer: %v", err)
	}

	c.consumers[config.Queue.Name] = stream
	c.wg.Add(1)

	// Start consumer goroutine
	go func() {
		defer c.wg.Done()
		for delivery := range stream {
			config.Handler(delivery)
		}
	}()

	return nil
}

// WaitForConsumers waits for all consumers to finish
func (c *Client) WaitForConsumers() {
	c.wg.Wait()
}

// PublishMessage publishes a message to the specified queue
func (c *Client) PublishMessage(ctx context.Context, queueName string, correlationID string, body []byte) error {
	err := c.channel.PublishWithContext(
		ctx,
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			CorrelationId: correlationID,
			Body:          body,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish message: %v", err)
	}
	return nil
}

// Close closes the AMQP connection and channel
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}
