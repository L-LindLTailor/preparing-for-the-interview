package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	// Go-клиент Confluent для Kafka, использующий под капотом Си-библиотеку librdkafka.
	// Confluent Go client for Kafka, using the C-based librdkafka library under the hood.
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	// Библиотека сама при старте (или изменении лимитов контейнера на лету)
	// выставит GOMAXPROCS = 1 (округлит 0.5 до ближайшего целого)
	_ "go.uber.org/automaxprocs"
)

func main() {
	// Считываем сетевой адрес брокера и уникальный ID транзакции из переменных окружения контейнера.
	// Read the broker network address and unique transaction ID from the container's environment variables.
	broker := os.Getenv("KAFKA_BROKER")
	txID := os.Getenv("TRANSACTIONAL_ID")

	// Создаем новый экземпляр продюсера со строгими гарантиями доставки.
	// Create a new producer instance with strict delivery guarantees.
	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers":  broker,
		"enable.idempotence": true, // Защита от дубликатов на уровне брокера (идемпотентность).
		// Prevents duplicate messages on the broker side (idempotence).
		"transactional.id": txID, // Включает транзакционный режим работы (требуется для Exactly-Once).
		// Enables transactional execution mode (required for Exactly-Once).
		"acks": "all", // Ждем подтверждения записи от всех синхронных реплик топика.
		// Wait for write acknowledgment from all in-sync topic replicas.
	})

	if err != nil {
		log.Fatalf("Failed to create producer: %v", err)
	}
	defer p.Close() // Гарантируем корректное закрытие продюсера при завершении приложения.
	// Ensure proper producer closure upon application termination.

	log.Println("Initializing Kafka transactions...")
	// Бесконечный цикл для ожидания готовности координатора транзакций в Kafka.
	// Infinite loop to wait for the Kafka transaction coordinator to become ready.
	for {
		// Создаем временный контекст на 15 секунд для одной конкретной попытки инициализации.
		// Create a temporary context for 15 seconds for one specific initialization attempt.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

		// Запрашиваем у Kafka выделение Producer ID и регистрацию транзакционного идентификатора.
		// Request a Producer ID assignment and register the transactional identifier with Kafka.
		err = p.InitTransactions(ctx)
		cancel() // Сразу очищаем ресурсы контекста, чтобы избежать утечки памяти.
		// Immediately clear context resources to prevent memory leaks.

		if err == nil {
			log.Println("Kafka transactions initialized successfully!")
			break // Успех: выходим из цикла ожидания и переходим к отправке.
			// Success: exit the wait loop and proceed to sending messages.
		}

		log.Printf("Kafka not ready yet (err: %v). Retrying in 5 seconds...", err)
		time.Sleep(5 * time.Second) // Пауза перед следующей попыткой связи с брокером.
		// Pause before the next broker communication attempt.
	}

	topic := "events"
	// Основной рабочий цикл генерации и отправки транзакционных сообщений.
	// Main worker loop for generating and sending transactional messages.
	for {
		// Открываем новую транзакцию. Все сообщения до вызова Commit будут атомарными.
		// Open a new transaction. All messages until Commit call will be atomic.
		if err := p.BeginTransaction(); err != nil {
			log.Printf("Error beginning transaction: %v", err)
			time.Sleep(5 * time.Second) // При ошибке ждем и уходим на перезапуск цикла.
			// On error, wait and go back to restart the loop.
			continue
		}

		// Формируем текстовое сообщение с текущей временной меткой.
		// Form a text message containing the current timestamp.
		msg := fmt.Sprintf("Message at %s", time.Now().Format(time.RFC3339))

		// Асинхронно отправляем сообщение в буфер librdkafka.
		// Asynchronously send the message to the librdkafka buffer.
		err = p.Produce(&kafka.Message{
			// PartitionAny позволяет библиотеке самой выбрать раздел на основе хэша ключа (Round Robin).
			// PartitionAny allows the library to select the partition based on key hash (Round Robin).
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
			Value:          []byte(msg),
		}, nil)
		if err != nil {
			log.Printf("Produce failed: %v", err)
		}

		// Фиксируем транзакцию. Используем context.Background(), так как таймаут контролируется через time.Sleep.
		// Commit the transaction. We use context.Background() as the timeout is controlled via time.Sleep.
		err = p.CommitTransaction(context.Background())
		if err != nil {
			log.Printf("Commit failed: %v", err)
			// При сбое фиксации принудительно откатываем транзакцию, очищая состояние брокера.
			// On commit failure, force abort the transaction to clear the broker state.
			p.AbortTransaction(context.Background())
		} else {
			// Exactly-Once семантика гарантирована: сообщение доставлено и транзакция закрыта.
			// Exactly-Once semantics guaranteed: message delivered and transaction closed.
			log.Printf("Sent and Committed: %s", msg)
		}

		time.Sleep(5 * time.Second) // Интервал между отправкой пакетов сообщений.
		// Interval between sending message batches.
	}
}
