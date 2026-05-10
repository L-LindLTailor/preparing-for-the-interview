package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq" // драйвер PostgreSQL
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var db *sql.DB

func initDB() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://user:password@postgres:5432/myapp?sslmode=disable"
	}

	var err error
	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Critical error opening DB: %v", err)
	}

	// Настройка пула
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)

	// --- ЦИКЛ ОЖИДАНИЯ БАЗЫ (RETRY LOGIC) ---
	for i := 1; i <= 10; i++ {
		err = db.Ping()
		if err == nil {
			log.Println("Database connection established!")
			return
		}
		log.Printf("Попытка %d: База еще не готова. Ждем 5 сек... (%v)", i, err)
		time.Sleep(5 * time.Second)
	}

	// Если за 50 секунд не подключились — тогда падаем
	log.Fatalf("Could not connect to DB after 10 attempts")
}

func main() {
	initDB()
	defer db.Close()

	// serverID := os.Getenv("SERVER_ID")
	serverID, _ := os.Hostname()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Проверяем подключение к БД через пул
		err := db.Ping()
		dbStatus := "DB: connected"
		if err != nil {
			dbStatus = "DB: error"
		}

		// Пример запроса к БД (получение текущего времени)
		var currentTime string
		err = db.QueryRow("SELECT NOW()::text").Scan(&currentTime)
		if err != nil {
			log.Printf("Database query error: %v", err)
			currentTime = "N/A"
		}

		fmt.Fprintf(w, `
		<h1>Go Server Instance %s</h1>
		<p>Port: %s</p>
	<p>Request from: %s</p>
	<p>%s</p>
	<p>Database time: %s</p>
	<p>Timestamp: %d</p>
	`, serverID, port, r.RemoteAddr, dbStatus, currentTime, time.Now().Unix())
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Теперь healthcheck в Swarm будет знать, если база отвалится
		if err := db.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("unhealthy"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("healthy"))
	})

	http.Handle("/metrics", promhttp.Handler())

	log.Printf("Server %s starting on :%s\n", serverID, port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
