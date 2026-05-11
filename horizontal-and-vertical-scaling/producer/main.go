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

	// Инициализируем транзакции
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = p.InitTransactions(ctx)
		cancel()

		if err == nil {
			break
		}

		log.Printf("Waiting for Kafka transaction init... (%v)", err)
		time.Sleep(3 * time.Second)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	topic := "events"
	for {
		// Начинаем транзакцию
		if err := p.BeginTransaction(); err != nil {
			log.Printf("Error beginning transaction: %v", err)
			continue
		}

		msg := fmt.Sprintf("Message at %s", time.Now().Format(time.RFC3339))
		err = p.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
			Value:          []byte(msg),
		}, nil)

		if err != nil {
			p.AbortTransaction(ctx)
			log.Printf("Produce failed: %v", err)
		} else {
			// Коммитим (Exactly-Once гарантируется здесь)
			p.CommitTransaction(ctx)
			log.Printf("Sent: %s", msg)
		}

		time.Sleep(5 * time.Second)
	}
}
