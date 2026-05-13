package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"

	// Официальный Go-клиент Confluent для взаимодействия с Kafka.
	// Official Confluent Go client for Kafka interaction.
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	// Типы данных и моделей официального SDK Docker.
	// Data types and models from the official Docker SDK.
	"github.com/docker/docker/api/types"

	// Клиент для управления Docker-демоном через API.
	// Client to manage the Docker daemon via API.
	"github.com/docker/docker/client"

	// Библиотека сама при старте (или изменении лимитов контейнера на лету)
	// выставит GOMAXPROCS = 1 (округлит 0.5 до ближайшего целого)
	_ "go.uber.org/automaxprocs"
)

// Внутренний сетевой адрес брокера Kafka внутри Overlay-сети.
// Internal network address of the Kafka broker inside the Overlay network.
const broker = "kafka:9092"

// HTTP-обработчик для управления масштабированием сервиса.
// HTTP handler to control service scaling.
func scaleService(w http.ResponseWriter, r *http.Request) {
	// Получаем параметры направления ("up"/"down") или точного целевого количества из URL.
	// Get direction ("up"/"down") or exact target count parameters from the URL query.
	direction := r.URL.Query().Get("dir")
	targetStr := r.URL.Query().Get("target")

	// Инициализируем клиент Docker, используя переменные окружения и согласование версий API.
	// Initialize the Docker client using environment variables and API version negotiation.
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Printf("Failed to create Docker client: %v", err)
		http.Error(w, "Failed to connect to Docker", http.StatusInternalServerError)
		return
	}

	// Точное имя целевого сервиса, развернутого в Docker Swarm.
	// The exact name of the target service deployed in Docker Swarm.
	serviceName := "my_stack_go-server"

	// Запрашиваем у Docker текущие метаданные и конфигурацию указанного сервиса.
	// Request the current metadata and configuration of the specified service from Docker.
	service, _, err := cli.ServiceInspectWithRaw(context.Background(), serviceName, types.ServiceInspectOptions{})
	if err != nil {
		log.Printf("Failed to inspect service %s: %v", serviceName, err)
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	var newReplicas uint64
	if targetStr != "" {
		// Если передан параметр точного количества, парсим строку в беззнаковое число.
		// If the exact target parameter is provided, parse the string into an unsigned integer.
		newReplicas64, err := strconv.ParseUint(targetStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid target parameter", http.StatusBadRequest)
			return
		}
		newReplicas = newReplicas64
	} else {
		// Извлекаем текущее количество реплик сервиса из указателя в структуре Swarm.
		// Extract the current number of service replicas from the pointer in the Swarm spec.
		currentReplicas := *service.Spec.Mode.Replicated.Replicas
		switch direction {
		case "up":
			// Вычисляем новое значение и инициируем масштабирование разделов Kafka.
			// Calculate the new value and trigger Kafka partitions scaling.
			newReplicas = currentReplicas + 1
			updateKafkaPartitions(int(newReplicas))
		case "down":
			// Ограничиваем сжатие системы: запрещено опускаться ниже 3 реплик.
			// Limit system downscaling: it is forbidden to drop below 3 replicas.
			if currentReplicas <= 3 {
				fmt.Fprintf(w, "Minimum replicas reached: %d", currentReplicas)
				return
			}
			newReplicas = currentReplicas - 1
		default:
			http.Error(w, "Invalid direction. Use 'up' or 'down'", http.StatusBadRequest)
			return
		}
	}

	// Ограничение сверху для предотвращения неконтролируемого расхода ресурсов.
	// Hard upper bound limit to prevent uncontrolled resource consumption.
	if newReplicas > 10 {
		newReplicas = 10
	}

	// Обновляем значение количества реплик в локальной структуре конфигурации.
	// Update the replicas count value in the local configuration structure.
	*service.Spec.Mode.Replicated.Replicas = newReplicas

	// Применяем обновленную спецификацию к сервису в кластере Docker Swarm.
	// Apply the updated specification to the service in the Docker Swarm cluster.
	_, err = cli.ServiceUpdate(
		context.Background(),
		service.ID,
		service.Version, // Передаем текущую версию для защиты от одновременных изменений.
		service.Spec,    // Passes the current version to protect against concurrent modifications.
		types.ServiceUpdateOptions{},
	)
	if err != nil {
		log.Printf("Failed to update service: %v", err)
		http.Error(w, "Failed to scale service", http.StatusInternalServerError)
		return
	}

	log.Printf("Service scaled to %d replicas", newReplicas)
	fmt.Fprintf(w, "OK: %d", newReplicas)
}

// Административная функция для управления количеством разделов топика Kafka.
// Administrative function to manage Kafka topic partition counts.
func updateKafkaPartitions(newReplicas int) {
	// Создаем административный клиент Kafka для изменения настроек инфраструктуры.
	// Create a Kafka Admin client to modify infrastructure settings.
	admin, err := kafka.NewAdminClient(&kafka.ConfigMap{"bootstrap.servers": broker})
	if err != nil {
		log.Printf("Failed to create AdminClient: %v", err)
		return
	}
	defer admin.Close() // Гарантируем закрытие соединений администратора.
	// Ensure administrator connections are closed.

	ctx := context.Background()

	// Отправляем запрос на увеличение разделов топика 'events'.
	// Помните: Kafka поддерживает только увеличение количества разделов.
	// Send a request to increase partitions for the 'events' topic.
	// Remember: Kafka only supports increasing the number of partitions.
	results, err := admin.CreatePartitions(ctx, []kafka.PartitionsSpecification{
		{
			Topic:      "events",
			IncreaseTo: newReplicas,
		},
	})

	if err != nil {
		log.Printf("Kafka partitions update request failed: %v", err)
	} else {
		log.Printf("Kafka partitions updated to %d: %v", newReplicas, results)
	}
}

func main() {
	// Регистрируем маршрут и запускаем веб-сервер для приема вебхуков от Alertmanager.
	// Register the route and start the web server to receive webhooks from Alertmanager.
	http.HandleFunc("/scale", scaleService)
	log.Println("Autoscaler started on :8888")
	log.Fatal(http.ListenAndServe(":8888", nil))
}
