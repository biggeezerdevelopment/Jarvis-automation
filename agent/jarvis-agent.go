package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"flag"
	"fmt"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
	"gopkg.in/yaml.v3"
)

// encryption key should be 32 bytes for AES-256
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

func decrypt(encryptedStr string) ([]byte, error) {
	// Decode from base64
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedStr)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
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

	// Set prefetchCount to 50 to allow 50 messages before Acks are returned
	if err := ch.Qos(50, 0, false); err != nil {
		panic(err)
	}

	// Auto ACK has to be False, AutoAck = True will Crash dueue to it is no implemented in Streams
	stream, err := ch.Consume(config.Queue.Name, config.Queue.Consumer, false, false, false, false, amqp.Table{})
	if err != nil {
		panic(err)
	}
	// Loop forever and read messages
	fmt.Println("Starting to consume")
	for event := range stream {
		fmt.Printf("Event: %s \n", event.CorrelationId)
		fmt.Printf("headers: %v \n", event.Headers)

		// Decrypt the received message
		decryptedData, err := decrypt(string(event.Body))
		if err != nil {
			fmt.Printf("Failed to decrypt message: %v\n", err)
			continue
		}

		fmt.Printf("Decrypted data: %s \n", string(decryptedData))
		event.Ack(true)
	}
	ch.Close()
}
