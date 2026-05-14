package slidingwindowlog

import (
	"sync"
	"time"
)

// SlidingWindowLogLimiter реализует алгоритм журнала скользящих интервалов.
// SlidingWindowLogLimiter implements the sliding window log rate limiting algorithm.
type SlidingWindowLogLimiter struct {
	mu         sync.Mutex
	windowSize time.Duration // Размер скользящего окна (например, 1 * time.Minute) / The sliding window size (e.g., 1 * time.Minute)
	maxLimit   int           // Максимальное число запросов внутри плавающего окна / Maximum number of requests inside the sliding window
	log        []time.Time   // Журнал (слайс) с метками времени успешных запросов / Log (slice) tracking timestamps of successful requests
}

// NewSlidingWindowLogLimiter создает новый лимитер скользящего окна.
// NewSlidingWindowLogLimiter creates a new sliding window log limiter instance.
func NewSlidingWindowLogLimiter(windowSize time.Duration, maxLimit int) *SlidingWindowLogLimiter {
	return &SlidingWindowLogLimiter{
		windowSize: windowSize,
		maxLimit:   maxLimit,
		log:        make([]time.Time, 0, maxLimit), // Заранее аллоцируем память под капасити лимита / Pre-allocate slice capacity to match the max limit
	}
}

// Allow проверяет лимит трафика по динамическому скользящему окну.
// Allow evaluates the traffic throughput threshold relative to a dynamic sliding window.
func (b *SlidingWindowLogLimiter) Allow() bool {
	// Блокируем мьютекс для предотвращения состояния гонки (Race Condition)
	// Lock the mutex to prevent race conditions during log manipulation
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	// Вычисляем левую (прошлую) границу нашего скользящего окна
	// Calculate the left (past) boundary of our sliding window
	windowBoundary := now.Add(-b.windowSize)

	// Очищаем старые записи: находим индекс первого элемента, который входит в текущее окно
	// Clean up stale records: locate the index of the first element that falls inside the active window
	validIndex := 0
	for i, timestamp := range b.log {
		if timestamp.After(windowBoundary) {
			validIndex = i
			break
		}
		// Если все элементы устарели, validIndex сдвинется до конца в блоке ниже
		// If all elements are outdated, advance validIndex to the very end in the block below
		if i == len(b.log)-1 {
			validIndex = len(b.log)
		}
	}

	// Отрезаем устаревшие метки времени, освобождая место в слайсе
	// Slice off the outdated timestamps, freeing up underlying array slots
	if validIndex > 0 {
		b.log = b.log[validIndex:]
	}

	// Проверяем, укладываемся ли мы в лимит внутри текущего плавающего окна
	// Verify if the active log size is strictly below the allowed max limit threshold
	if len(b.log) < b.maxLimit {
		b.log = append(b.log, now) // Записываем текущий запрос в журнал / Append the current request timestamp to the log
		return true                // Запрос разрешен / Request is allowed
	}

	// Журнал заполнен, отправляем жесткий отказ
	// Log is saturated, return an immediate restriction response
	return false
}
