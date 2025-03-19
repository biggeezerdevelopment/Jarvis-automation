package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"gopkg.in/yaml.v3"
)

var encryptionKey = []byte("12345678901234567890123456789012")

type Config struct {
	AMQP struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
		Host     string `yaml:"host"`
		VHost    string `yaml:"vhost"`
	} `yaml:"amqp"`
	Queue struct {
		Name     string `yaml:"name"`
		Consumer string `yaml:"consumer"`
	} `yaml:"queue"`
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

func encrypt(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	// Create a new GCM cipher
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Create a nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Encrypt and seal the data
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	// Convert to base64 for easy transmission
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	eventName := flag.String("name", "Default Name", "Name of the event to send")
	eventMsg := flag.String("msg", "Hello from default name", "Message to send")
	flag.Parse()

	config, err := loadConfig(*configPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to load config: %v", err))
	}

	conn, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s/%s",
		config.AMQP.Username,
		config.AMQP.Password,
		config.AMQP.Host,
		config.AMQP.VHost))
	if err != nil {
		panic(err)
	}

	ch, err := conn.Channel()
	if err != nil {
		panic(err)
	}

	// Start the queue
	q, err := ch.QueueDeclare(config.Queue.Name, true, false, false, true, amqp.Table{
		"x-queue-type":                    "stream",
		"x-stream-max-segment-size-bytes": 30000,  // 0.03 MB
		"x-max-length-bytes":              150000, // 0.15 MB
		// If you want to use Timebased, set the x-max-age
		// "x-max-age":                       "10s",
	})
	if err != nil {
		panic(err)
	}
	// Publish 1000 Messages
	ctx := context.Background()
	event := Event{
		Name: *eventName,
		Msg:  *eventMsg,
	}
	data, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}

	encryptedData, err := encrypt(data)
	if err != nil {
		panic(err)
	}

	err = ch.PublishWithContext(ctx, "", "events", false, false, amqp.Publishing{
		CorrelationId: uuid.NewString(),
		Body:          []byte(encryptedData),
	})
	if err != nil {
		panic(err)
	}

	ch.Close()
	fmt.Println(q.Name)
}
