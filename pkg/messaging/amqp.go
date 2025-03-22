package messaging

import (
	"context"
	"fmt"
	"sync"
	"time"

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

// PublishConfig holds the configuration for publishing a message
type PublishConfig struct {
	Queue         string
	CorrelationID string
	Body          []byte
}

// Default queue arguments
var defaultQueueArgs = amqp.Table{
	"x-queue-type":                    "stream",
	"x-stream-max-segment-size-bytes": 30000,  // 0.03 MB
	"x-max-length-bytes":              150000, // 0.15 MB
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
	url := fmt.Sprintf("amqp://%s:%s@%s/%s",
		config.Username,
		config.Password,
		config.Host,
		config.VHost)

	conn, err := amqp.Dial(url)
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

// DeclareQueue is a method to ensure consistent queue declaration
func (c *Client) DeclareQueue(name string) (*amqp.Queue, error) {
	queue, err := c.channel.QueueDeclare(
		name,  // name
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		defaultQueueArgs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare queue: %v", err)
	}
	return &queue, nil
}

// AddConsumer adds a new consumer with the given configuration
func (c *Client) AddConsumer(config ConsumerConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Declare the queue with consistent arguments
	queue, err := c.DeclareQueue(config.Queue.Name)
	if err != nil {
		return err
	}

	// Set QoS
	if err := c.channel.Qos(
		config.PrefetchCount, // prefetch count
		0,                    // prefetch size
		false,                // global
	); err != nil {
		return fmt.Errorf("failed to set QoS: %v", err)
	}

	// Start consuming
	msgs, err := c.channel.Consume(
		queue.Name,            // queue
		config.Queue.Consumer, // consumer
		false,                 // auto-ack
		false,                 // exclusive
		false,                 // no-local
		false,                 // no-wait
		nil,                   // args
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %v", err)
	}

	c.consumers[config.Queue.Name] = msgs
	c.wg.Add(1)

	// Start consumer goroutine
	go func() {
		defer c.wg.Done()
		for msg := range msgs {
			config.Handler(msg)
		}
	}()

	return nil
}

// WaitForConsumers waits for all consumers to finish
func (c *Client) WaitForConsumers() {
	c.wg.Wait()
}

// PublishMessage publishes a message to the specified queue
func (c *Client) PublishMessage(config PublishConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Declare the queue with consistent arguments
	_, err := c.DeclareQueue(config.Queue)
	if err != nil {
		return err
	}

	return c.channel.PublishWithContext(
		ctx,
		"",           // exchange
		config.Queue, // routing key
		false,        // mandatory
		false,        // immediate
		amqp.Publishing{
			ContentType:   "text/plain",
			Body:          config.Body,
			DeliveryMode:  amqp.Persistent,
			CorrelationId: config.CorrelationID,
		},
	)
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

// ListQueues retrieves information about all queues from RabbitMQ
func (c *Client) ListQueues() ([]QueueInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create management config
	mgmtConfig := NewManagementConfig(
		c.config.Username,
		c.config.Password,
		c.config.Host,
		15672, // Default RabbitMQ management port
	)

	// Get queues using management API
	return mgmtConfig.ListQueues()
}

// DeleteQueue deletes a queue from RabbitMQ
func (c *Client) DeleteQueue(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Delete the queue
	_, err := c.channel.QueueDelete(
		name,  // name
		false, // if unused
		false, // if empty
		false, // no wait
	)
	if err != nil {
		return fmt.Errorf("failed to delete queue: %v", err)
	}

	return nil
}
