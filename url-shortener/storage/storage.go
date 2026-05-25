package storage

import (
	"database/sql"

	_ "github.com/lib/pq"
)

// Storage инкапсулирует пул соединений с базой данных PostgreSQL.
// Storage encapsulates the underlying PostgreSQL database connection pool.
type Storage struct {
	db *sql.DB
}

// NewPostgresStorage инициализирует новое подключение к PostgreSQL и проверяет схему данных.
// NewPostgresStorage initializes a new PostgreSQL connection and validates the data schema.
func NewPostgresStorage(connStr string) (*Storage, error) {
	// Открываем пул соединений без немедленного физического подключения
	// Open a database connection pool without establishing a physical network link immediately
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	// Выполняем Ping, чтобы гарантировать физическую доступность СУБД по сети
	// Perform a Ping operation to ensure the database engine is physically reachable over the network
	if err := db.Ping(); err != nil {
		return nil, err
	}

	s := &Storage{db: db}

	// Ленивая инициализация схемы данных при запуске ноды
	// Reactive database schema initialization upon node startup
	if err := s.initSchema(); err != nil {
		return nil, err
	}

	return s, nil
}

// initSchema создает целевую структуру таблиц и индексов, если они отсутствуют.
// initSchema builds the target table structures and required indices if absent.
func (s *Storage) initSchema() error {
	// Маркер UNIQUE автоматически разворачивает B-Tree индекс для поиска за O(log N)
	// The UNIQUE token automatically provisions a B-Tree index for O(log N) lookups
	query := `
	CREATE TABLE IF NOT EXISTS urls (
		id BIGINT PRIMARY KEY,
		short_url VARCHAR(15) UNIQUE NOT NULL,
		long_url TEXT NOT NULL
	);`
	_, err := s.db.Exec(query)
	return err
}

// Save атомарно записывает связку идентификаторов и оригинальный URL в персистентное хранилище.
// Save atomically persists the identifier bindings alongside the original long URL.
func (s *Storage) Save(id uint64, shortURL, longURL string) error {
	query := `INSERT INTO urls (id, short_url, long_url) VALUES ($1, $2, $3);`
	// Запрос выполняется через безопасные плейсхолдеры для защиты от SQL-инъекций
	// The query uses parameterized placeholders to shield the layer from SQL Injection vectors
	_, err := s.db.Exec(query, id, shortURL, longURL)
	return err
}

// GetLongURL извлекает оригинальный длинный URL по короткому Base62 токену.
// GetLongURL extracts the original long URL target matching the short Base62 token.
func (s *Storage) GetLongURL(shortURL string) (string, error) {
	query := `SELECT long_url FROM urls WHERE short_url = $1;`
	var longURL string

	// Сканируем строчку. Ошибку пустого результата (ErrNoRows) обрабатываем как безопасное отсутствие ключа
	// Scan the row. Handle empty results (ErrNoRows) gracefully as a safe key-miss state
	err := s.db.QueryRow(query, shortURL).Scan(&longURL)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return longURL, err
}
