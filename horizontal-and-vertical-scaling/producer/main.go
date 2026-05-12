package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

func main() {
	broker := os.Getenv("KAFKA_BROKER")
	txID := os.Getenv("TRANSACTIONAL_ID")

	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers":  broker,
		"enable.idempotence": true,  // Идемпотентность (защита от дублей)
		"transactional.id":   txID,  // ID для Exactly-Once
		"acks":               "all", // Подтверждение от всех реплик
	})

	if err != nil {
		log.Fatalf("Failed to create producer: %v", err)
	}
	defer p.Close()

	log.Println("Initializing Kafka transactions...")
	for {
		// Даем фоновым Си-потокам 15 секунд на один чистый запрос
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err = p.InitTransactions(ctx)
		cancel()

		if err == nil {
			log.Println("Kafka transactions initialized successfully!")
			break // ТОЛЬКО ТЕПЕРЬ выходим из цикла к отправке сообщений
		}

		log.Printf("Kafka not ready yet (err: %v). Retrying in 5 seconds...", err)
		time.Sleep(5 * time.Second)
	}

	topic := "events"
	for {
		// Начинаем транзакцию
		if err := p.BeginTransaction(); err != nil {
			log.Printf("Error beginning transaction: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		msg := fmt.Sprintf("Message at %s", time.Now().Format(time.RFC3339))
		err = p.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
			Value:          []byte(msg),
		}, nil)
		if err != nil {
			log.Printf("Produce failed: %v", err)
		}

		err = p.CommitTransaction(context.Background())
		if err != nil {
			log.Printf("Commit failed: %v", err)
			p.AbortTransaction(context.Background()) // Обязательно откатываем, если не смогли закоммитить
		} else {
			log.Printf("Sent and Committed: %s", msg)
		}

		time.Sleep(5 * time.Second)
	}
}
