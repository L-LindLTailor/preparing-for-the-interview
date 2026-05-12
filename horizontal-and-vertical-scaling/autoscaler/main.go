package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

const broker = "kafka:9092"

func scaleService(w http.ResponseWriter, r *http.Request) {
	direction := r.URL.Query().Get("dir")
	targetStr := r.URL.Query().Get("target")

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Printf("Failed to create Docker client: %v", err)
		http.Error(w, "Failed to connect to Docker", http.StatusInternalServerError)
		return
	}

	serviceName := "my_stack_go-server"

	// Получаем текущее состояние сервиса
	service, _, err := cli.ServiceInspectWithRaw(context.Background(), serviceName, types.ServiceInspectOptions{})
	if err != nil {
		log.Printf("Failed to inspect service %s: %v", serviceName, err)
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	var newReplicas uint64
	if targetStr != "" {
		// Если указан target — устанавливаем точное количество
		newReplicas64, err := strconv.ParseUint(targetStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid target parameter", http.StatusBadRequest)
			return
		}
		newReplicas = newReplicas64
	} else {
		// Иначе изменяем текущее количество
		currentReplicas := *service.Spec.Mode.Replicated.Replicas
		switch direction {
		case "up":
			newReplicas = currentReplicas + 1
			updateKafkaPartitions(int(newReplicas))
		case "down":
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

	// Ограничение максимального количества реплик
	if newReplicas > 10 {
		newReplicas = 10
	}

	*service.Spec.Mode.Replicated.Replicas = newReplicas

	// Отправляем обновление в Swarm
	_, err = cli.ServiceUpdate(
		context.Background(),
		service.ID,
		service.Version,
		service.Spec,
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

func updateKafkaPartitions(newReplicas int) {

	admin, err := kafka.NewAdminClient(&kafka.ConfigMap{"bootstrap.servers": broker})
	if err != nil {
		log.Printf("Failed to create AdminClient: %v", err)
		return
	}
	defer admin.Close()

	ctx := context.Background()

	// Увеличиваем партиции до количества реплик
	// Помни: Kafka позволяет только УВЕЛИЧИВАТЬ
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
	http.HandleFunc("/scale", scaleService)
	log.Println("Autoscaler started on :8888")
	log.Fatal(http.ListenAndServe(":8888", nil))
}
