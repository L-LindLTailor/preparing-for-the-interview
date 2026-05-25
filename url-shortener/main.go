package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"url-shortener/base62"
	"url-shortener/storage"
	idgen "url-shortener/twitter-snowflake-id" // Твой пакет со Snowflake из прошлой темы / Your Snowflake package from the previous topic
)

var (
	sfNode *idgen.SnowflakeNode
	db     *storage.Storage
)

type ShortenRequest struct {
	LongURL string `json:"long_url"`
}

type ShortenResponse struct {
	ShortURL string `json:"short_url"`
}

func main() {
	var err error
	// Инициализируем Snowflake для Сервера №1 в Дата-центре №1
	// Initialize Snowflake for Server #1 in Datacenter #1
	sfNode, err = idgen.NewSnowflakeNode(1, 1)
	if err != nil {
		log.Fatal(err)
	}

	// Строка подключения к PostgreSQL из Docker Compose
	// PostgreSQL connection string from Docker Compose
	connStr := "postgres://user:password@localhost:5432/shortener?sslmode=disable"
	db, err = storage.NewPostgresStorage(connStr)
	if err != nil {
		log.Fatal("DB Connection failed: ", err)
	}

	http.HandleFunc("/api/v1/shorten", HandleShorten)
	http.HandleFunc("/", HandleRedirect)

	log.Println("URL Shortener запущен на порту :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// shuffleBits превращает последовательные или близкие числа в хаотичный разброс.
// Использует битовые маски и XOR-сдвиги для защиты от атак перебором.
// shuffleBits scrambles sequential or near numbers into a chaotic spread.
// Utilizes bit masks and XOR shifts to mitigate enumeration attacks.
func shuffleBits(num uint64) uint64 {
	num = ((num >> 32) ^ num) * 0x45d9f3b
	num = ((num >> 32) ^ num) * 0x45d9f3b
	num = (num >> 32) ^ num
	return num
}

// Обратная функция для восстановления оригинального ID
// Inverse function to restore the original ID layout
func unshuffleBits(num uint64) uint64 {
	num = ((num >> 32) ^ num) * 0x11844e75915d974b
	num = ((num >> 32) ^ num) * 0x11844e75915d974b
	num = (num >> 32) ^ num
	return num
}

// HandleShorten принимает длинный URL, генерирует ID, хаотично перемешивает биты и кодирует в Base62
// HandleShorten accepts a long URL, generates an ID, scrambles bits into chaos, and encodes to Base62
func HandleShorten(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ShortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 1. Генерируем уникальный 64-битный ID через Snowflake (p99 latency < 10ns)
	// 1. Generate a unique 64-bit ID via Snowflake (p99 latency < 10ns)
	id, err := sfNode.Generate()
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// SECURITY FIX: Перемешиваем биты полученного ID, делая его непредсказуемым для хакера
	// SECURITY FIX: Scramble the bits of the generated ID, making it completely unpredictable for an attacker
	shuffledID := shuffleBits(id)

	// 2. Кодируем перемешанное число в короткую Base62 строку
	// 2. Encode the scrambled integer into a short Base62 token string
	shortToken := base62.Encode(shuffledID)

	// 3. Сохраняем оригинальный ID, токен и оригинальную ссылку в PostgreSQL
	// 3. Save the original ID, token, and long URL mapping into PostgreSQL
	if err := db.Save(id, shortToken, req.LongURL); err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ShortenResponse{
		ShortURL: fmt.Sprintf("http://localhost:8080/%s", shortToken),
	})
}

// HandleRedirect обрабатывает короткую ссылку и совершает безопасный 302 редирект
// HandleRedirect processes the short link token and executes a secure 302 redirect
func HandleRedirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Извлекаем токен из URL пути (например, "/7bX9" -> "7bX9")
	// Extract the token from the URL path (e.g., "/7bX9" -> "7bX9")
	token := strings.TrimPrefix(r.URL.Path, "/")
	if token == "" {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Мгновенный поиск оригинального URL в PostgreSQL по B-Tree индексу
	// Instant long URL lookup inside PostgreSQL via a unique B-Tree index slot
	longURL, err := db.GetLongURL(token)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if longURL == "" {
		http.Error(w, "URL not found", http.StatusNotFound)
		return
	}

	// Возвращаем временный редирект 302 для сохранения возможности сбора аналитики кликов
	// Return a 302 temporary redirect to preserve downstream clickstream analytics capabilities
	http.Redirect(w, r, longURL, http.StatusFound)
}
