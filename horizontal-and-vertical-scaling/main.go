package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	_ "github.com/lib/pq" // драйвер PostgreSQL
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

var (
	dbMaster *sql.DB
	dbSlave  *sql.DB
	rdb      *redis.Client

	ctx = context.Background()

	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"source"}) // Теги: "cache" или "db"
)

func initRedis() {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}

	rdb = redis.NewClient(&redis.Options{
		Addr: redisURL,
	})

	for i := 1; i <= 10; i++ {
		_, err := rdb.Ping(ctx).Result()
		if err == nil {
			log.Println("Successfully connected to Redis")
			return
		}
		log.Printf("Попытка %d: Redis не отвечает, ждем 3 сек... (%v)", i, err)
		time.Sleep(3 * time.Second)
	}

	log.Fatalf("Could not connect to Redis after 10 attempts")
}

func initDB() {
	// Инициализируем Master (Запись)
	dbMaster = connectWithRetry(os.Getenv("DATABASE_URL"), "MASTER")
	setupPool(dbMaster)

	// Инициализируем Slave (Чтение)
	dbSlave = connectWithRetry(os.Getenv("READ_DATABASE_URL"), "SLAVE")
	setupPool(dbSlave)
}

func connectWithRetry(url, name string) *sql.DB {
	db, _ := sql.Open("postgres", url)
	for i := 1; i <= 10; i++ {
		if err := db.Ping(); err == nil {
			log.Printf("[%s] Connected", name)
			return db
		}
		time.Sleep(3 * time.Second)
	}
	log.Fatalf("[%s] Connection failed", name)
	return nil
}

func setupPool(db *sql.DB) {
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
}

func main() {
	var exit = make(chan struct{})

	initDB()
	initRedis()

	defer dbMaster.Close()
	defer dbSlave.Close()
	defer rdb.Close()

	// serverID := os.Getenv("SERVER_ID")
	serverID, _ := os.Hostname()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 1. Пытаемся взять из Redis
		val, err := rdb.Get(ctx, "db_time").Result()
		var currentTime string
		source := "CACHE"

		if err == redis.Nil {
			// 2. Если нет в кэше — читаем из SLAVE
			httpRequestsTotal.WithLabelValues("db").Inc()
			source = "SLAVE DB"
			err := dbSlave.QueryRow("SELECT NOW()::text").Scan(&currentTime)
			if err != nil {
				log.Printf("Query error: %v", err)
				currentTime = "N/A"
			} else {
				// Кладем в кэш на 10 сек
				rdb.Set(ctx, "db_time", currentTime, 10*time.Second)
			}
		} else {
			httpRequestsTotal.WithLabelValues("cache").Inc()
			currentTime = val
		}

		// 3. Для записи лога (эмуляция) использовали бы dbMaster.Exec(...)

		fmt.Fprintf(w, "<h1>Instance: %s</h1><p>Source: %s</p><p>Time: %s</p>", serverID, source, currentTime)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Проверяем всех
		if dbMaster.Ping() != nil || dbSlave.Ping() != nil || rdb.Ping(ctx).Err() != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	http.Handle("/metrics", promhttp.Handler())

	go func() {
		broker := os.Getenv("KAFKA_BROKER")

	EXIT:
		for {
			c, err := kafka.NewConsumer(&kafka.ConfigMap{
				"bootstrap.servers": broker,
				"group.id":          "go-server-group",
				"auto.offset.reset": "earliest",
				// Критически важно для Exactly-Once:
				// Читать только сообщения из успешно завершенных (committed) транзакций
				"isolation.level": "read_committed",
			})

			if err != nil {
				log.Println(err)
				continue
			}

			log.Println(c)

			select {
			case <-exit:
				break EXIT
			default:
				time.Sleep(time.Second * 3)
			}
		}
	}()

	log.Printf("Server %s starting on :%s\n", serverID, port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
	exit <- struct{}{}
}
