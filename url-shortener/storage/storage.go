package storage

import (
	"database/sql"

	_ "github.com/lib/pq"
)

type Storage struct {
	db *sql.DB
}

func NewPostgresStorage(connStr string) (*Storage, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	s := &Storage{db: db}
	if err := s.initSchema(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Storage) initSchema() error {
	// Создаем таблицу. Индекс по short_url делаем UNIQUE для мгновенного поиска (B-Tree)
	query := `
	CREATE TABLE IF NOT EXISTS urls (
		id BIGINT PRIMARY KEY,
		short_url VARCHAR(15) UNIQUE NOT NULL,
		long_url TEXT NOT NULL
	);`
	_, err := s.db.Exec(query)
	return err
}

func (s *Storage) Save(id uint64, shortURL, longURL string) error {
	query := `INSERT INTO urls (id, short_url, long_url) VALUES ($1, $2, $3);`
	_, err := s.db.Exec(query, id, shortURL, longURL)
	return err
}

func (s *Storage) GetLongURL(shortURL string) (string, error) {
	query := `SELECT long_url FROM urls WHERE short_url = $1;`
	var longURL string
	err := s.db.QueryRow(query, shortURL).Scan(&longURL)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return longURL, err
}
