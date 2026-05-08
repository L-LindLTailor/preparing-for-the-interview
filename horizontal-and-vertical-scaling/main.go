package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq" // драйвер PostgreSQL
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
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Настройка пула соединений
	db.SetMaxOpenConns(25)                 // макс. открытых соединений
	db.SetMaxIdleConns(25)                 // макс. idle соединений
	db.SetConnMaxLifetime(5 * time.Minute) // время жизни соединения
	db.SetConnMaxIdleTime(1 * time.Minute) // время простоя соединения

	// Проверка подключения
	err = db.Ping()
	if err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("Database connection established")
}

func main() {
	initDB()
	defer db.Close()

	serverID := os.Getenv("SERVER_ID")
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
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("healthy"))
	})

	log.Printf("Server %s starting on :%s\n", serverID, port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
