package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	// Официальный Go-клиент Confluent для взаимодействия с Kafka.
	// Official Confluent Go client for Kafka interaction.
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	// Чистый Go-драйвер для СУБД PostgreSQL. Префикс "_" необходим для регистрации драйвера в пакете database/sql.
	// Pure Go driver for PostgreSQL. The "_" prefix is required to register the driver in the database/sql package.
	_ "github.com/lib/pq"

	// Пакеты официального SDK Prometheus для генерации, авторегистрации и экспорта метрик.
	// Packages from the official Prometheus SDK for generating, auto-registering, and exporting metrics.
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	// Клиент для работы с NoSQL СУБД и кэшем Redis.
	// Client to interact with the Redis NoSQL database and cache.
	"github.com/redis/go-redis/v9"

	// Библиотека сама при старте (или изменении лимитов контейнера на лету)
	// выставит GOMAXPROCS = 1 (округлит 0.5 до ближайшего целого)
	_ "go.uber.org/automaxprocs"
)

var (
	// Глобальные пулы соединений для Master и Slave инстансов СУБД PostgreSQL.
	// Global connection pools for PostgreSQL Master and Slave instances.
	dbMaster *sql.DB
	dbSlave  *sql.DB

	// Глобальный клиент для взаимодействия с сервером Redis.
	// Global client instance for interacting with the Redis server.
	rdb *redis.Client

	// Корневой фоновый контекст для долгоживущих сетевых операций приложения.
	// Root background context for long-running network operations of the application.
	ctx = context.Background()

	// Счетчик Prometheus для регистрации общего количества обработанных HTTP-запросов.
	// Вектор меток "source" разделяет метрики на Cache Hits и Cache Misses (запросы в БД).
	// Prometheus counter to register the total number of processed HTTP requests.
	// The "source" label vector splits metrics into Cache Hits and Cache Misses (DB queries).
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"source"})
)

// Функция для инициализации подключения к Redis с логикой повторных попыток (Retry).
// Function to initialize Redis connection with retry logic.
func initRedis() {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}

	// Инициализируем структуру конфигурации клиента Redis.
	// Initialize the Redis client configuration structure.
	rdb = redis.NewClient(&redis.Options{
		Addr: redisURL,
	})

	// Цикл ожидания доступности Redis (до 10 попыток с паузой в 3 секунды).
	// Wait loop for Redis availability (up to 10 attempts with a 3-second pause).
	for i := 1; i <= 10; i++ {
		_, err := rdb.Ping(ctx).Result()
		if err == nil {
			log.Println("Successfully connected to Redis")
			return // Успех: выходим из функции. / Success: exit the function.
		}
		log.Printf("Попытка %d: Redis не отвечает, ждем 3 сек... (%v)", i, err)
		time.Sleep(3 * time.Second)
	}

	// Если Redis не ответил за 10 попыток, аварийно завершаем процесс приложения.
	// If Redis fails to respond within 10 attempts, terminate the application process immediately.
	log.Fatalf("Could not connect to Redis after 10 attempts")
}

// Оркестратор инициализации пулов соединений баз данных.
// DB connection pools initialization orchestrator.
func initDB() {
	// Инициализируем Master пул (только для операций записи: INSERT/UPDATE/DELETE).
	// Initialize Master pool (only for write operations: INSERT/UPDATE/DELETE).
	dbMaster = connectWithRetry(os.Getenv("DATABASE_URL"), "MASTER")
	setupPool(dbMaster)

	// Инициализируем Slave пул (только для распределенных операций чтения: SELECT).
	// Initialize Slave pool (only for distributed read operations: SELECT).
	dbSlave = connectWithRetry(os.Getenv("READ_DATABASE_URL"), "SLAVE")
	setupPool(dbSlave)
}

// Создает пул соединений к СУБД с циклом отказоустойчивости при старте контейнеров Swarm.
// Creates a database connection pool with a fault-tolerant loop at Swarm containers boot-up.
func connectWithRetry(url, name string) *sql.DB {
	// Инициализируем абстракцию пула драйвера. Метод не открывает физических сетевых сокетов.
	// Initializes the driver pool abstraction. This method does not open physical network sockets.
	db, _ := sql.Open("postgres", url)

	for i := 1; i <= 10; i++ {
		// Метод Ping принудительно заставляет драйвер открыть тестовое сетевое соединение с СУБД.
		// The Ping method forces the driver to open a test network connection to the database.
		if err := db.Ping(); err == nil {
			log.Printf("[%s] Connected", name)
			return db // Узел базы данных доступен. / The database node is accessible.
		}
		time.Sleep(3 * time.Second)
	}
	log.Fatalf("[%s] Connection failed", name)
	return nil
}

// Настройка лимитов производительности и жизненного цикла соединений внутри пула.
// Tweaking performance limits and connection lifecycles within the pool.
func setupPool(db *sql.DB) {
	db.SetMaxOpenConns(25)                 // Ограничение на максимальное число открытых сокетов к СУБД. / Upper limit on concurrent open sockets to DB.
	db.SetMaxIdleConns(25)                 // Сколько простаивающих соединений удерживать открытыми в пуле. / Idle connections count to keep open in the pool.
	db.SetConnMaxLifetime(5 * time.Minute) // Максимальный возраст соединения перед утилизацией. / Maximum age of a connection before recycling.
}

func main() {
	// Канал для координации корректного завершения (Graceful Shutdown) фонового консьюмера.
	// Channel to coordinate the graceful shutdown of the background consumer.
	var exit = make(chan struct{})

	initDB()
	initRedis()

	// Гарантируем закрытие всех системных дескрипторов и сокетов при выходе из main.
	// Guarantees all system descriptors and sockets close upon exiting main.
	defer dbMaster.Close()
	defer dbSlave.Close()
	defer rdb.Close()

	// Считываем уникальное имя хоста контейнера (в Swarm это уникальный ID реплики).
	// Reads the container's unique hostname (in Swarm, this represents a unique replica ID).
	serverID, _ := os.Hostname()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Обработчик корневого эндпоинта приложения с паттерном кэширования "Cache-Aside".
	// Root endpoint application handler implementing the "Cache-Aside" caching pattern.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 1. Ищем значение в оперативной памяти Redis.
		// 1. Try fetching the value from Redis in-memory storage.
		val, err := rdb.Get(ctx, "db_time").Result()
		var currentTime string
		source := "CACHE"

		// Проверяем, вернул ли Redis специфичную ошибку отсутствия ключа (Cache Miss).
		// Verifies if Redis returned a specific missing key error (Cache Miss).
		if err == redis.Nil {
			// Инкрементируем метрику промахов кэша (запросы уходят в физическую СУБД).
			// Increments cache miss metrics (queries routed to physical RDBMS).
			httpRequestsTotal.WithLabelValues("db").Inc()
			source = "SLAVE DB"

			// Выполняем легкий SELECT-запрос к СУБД, используя пул соединений Slave-реплики.
			// Executes a lightweight SELECT query to the DB using the Slave replica connection pool.
			err := dbSlave.QueryRow("SELECT NOW()::text").Scan(&currentTime)
			if err != nil {
				log.Printf("Query error: %v", err)
				currentTime = "N/A"
			} else {
				// Кладем данные в кэш Redis на 10 секунд для обслуживания последующих запросов.
				// Pushes data into Redis cache for 10 seconds to serve subsequent requests.
				rdb.Set(ctx, "db_time", currentTime, 10*time.Second)
			}
		} else {
			// Успешное попадание в кэш (Cache Hit): инкрементируем метрику и берем данные из памяти.
			// Successful Cache Hit: increment metric and retrieve data directly from memory.
			httpRequestsTotal.WithLabelValues("cache").Inc()
			currentTime = val
		}

		// 3. Любые транзакции на изменение данных (INSERT/UPDATE) направлялись бы в dbMaster.
		// 3. Any mutation transactions (INSERT/UPDATE) would be explicitly routed to dbMaster.

		// Формируем HTML-ответ с указанием конкретной реплики-исполнителя и источника данных.
		// Forms the HTML response specifying the processing replica and data origin source.
		fmt.Fprintf(w, "<h1>Instance: %s</h1><p>Source: %s</p><p>Time: %s</p>", serverID, source, currentTime)
	})

	// Эндпоинт проверки здоровья контейнера для оркестратора Docker Swarm.
	// Container infrastructure healthcheck endpoint for Docker Swarm orchestrator.
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Контейнер считается живым, только если все внешние зависимости ответили на Ping.
		// Container is considered healthy only if all external dependencies acknowledge Ping.
		if dbMaster.Ping() != nil || dbSlave.Ping() != nil || rdb.Ping(ctx).Err() != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Экспортируем стандартный HTTP-обработчик метрик Prometheus для сбора данных агентом (Scraping).
	// Exposes standard Prometheus metrics HTTP handler for agent-based metric scraping.
	http.Handle("/metrics", promhttp.Handler())

	// Запускаем фоновый отказоустойчивый транзакционный консьюмер Kafka в отдельной горутине.
	// Launches a background fault-tolerant transactional Kafka consumer inside a separate goroutine.
	go func() {
		broker := os.Getenv("KAFKA_BROKER")

		// 1. Инициализируем консьюмер ОДИН РАЗ перед циклом чтения.
		// 1. Initialize consumer EXACTLY ONCE before entering the retrieval loop.
		c, err := kafka.NewConsumer(&kafka.ConfigMap{
			"bootstrap.servers": broker,
			// Динамический суффикс group.id гарантирует, что каждая реплика развернет свою независимую Consumer Group.
			// Dynamic group.id suffix ensures each replica spawns its own independent Consumer Group.
			"group.id":          "go-server-group-" + serverID,
			"auto.offset.reset": "earliest",
			// Изоляция гарантирует чтение ТОЛЬКО успешно закомиченных (committed) Exactly-Once транзакций.
			// Isolation guarantees reading ONLY successfully committed Exactly-Once transactions.
			"isolation.level":    "read_committed",
			"enable.auto.commit": true,
		})

		if err != nil {
			log.Fatalf("Failed to create consumer: %v", err)
		}
		defer c.Close() // Закрываем консьюмер и освобождаем Си-потоки librdkafka при выходе из горутины.
		// Closes consumer and releases librdkafka C-threads upon goroutine exit.

		// 2. Оформляем подписку на целевой топик 'events'.
		// 2. Subscribe to the target 'events' topic.
		err = c.SubscribeTopics([]string{"events"}, nil)
		if err != nil {
			log.Fatalf("Failed to subscribe: %v", err)
		}

		log.Println("Kafka Consumer started and subscribed to 'events'")

		// 3. Бесконечный цикл последовательного бесконфликтного чтения сообщений из брокера.
		// 3. Infinite loop for sequential, conflict-free message ingestion from the broker.
		for {
			select {
			case <-exit:
				// Перехватываем сигнал остановки из основного потока для мягкого завершения.
				// Intercepts stop signal from the main thread for graceful termination.
				log.Println("Stopping consumer...")
				return
			default:
				// Блокирующий метод чтения. Ждет сообщение из шины данных не более 1 секунды.
				// Blocking read method. Awaits data bus message for a maximum of 1 second.
				msg, err := c.ReadMessage(time.Second)
				if err == nil {
					fmt.Printf("Received message: %s\n", string(msg.Value))
				} else if !err.(kafka.Error).IsTimeout() {
					// Логируем только критические ошибки связи, игнорируя штатные таймауты ожидания шины.
					// Logs critical communication errors only, ignoring routine data bus await timeouts.
					log.Printf("Consumer error: %v\n", err)
				}
			}
		}
	}()

	log.Printf("Server %s starting on :%s\n", serverID, port)

	// Блокирующий запуск основного веб-сервера.
	// Blocking invocation of the primary web server.
	log.Fatal(http.ListenAndServe(":"+port, nil))

	// Точка недостижимого мёртвого кода из-за Fatal выше, но зарезервированная под архитектурный паттерн shutdown-сигналов.
	// Unreachable dead-code point due to Fatal above, reserved for structural shutdown signaling patterns.
	exit <- struct{}{}
}
