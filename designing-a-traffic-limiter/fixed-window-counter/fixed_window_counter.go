package fixedwindowcounter

import (
	"sync"
	"time"
)

// FixedWindowLimiter реализует алгоритм счетчика фиксированных интервалов.
// FixedWindowLimiter implements the fixed window counter rate limiting algorithm.
type FixedWindowLimiter struct {
	mu           sync.Mutex
	windowSize   time.Duration // Размер окна (например, 1 * time.Minute или 1 * time.Second) / The duration of the window (e.g., 1 * time.Minute or 1 * time.Second)
	maxLimit     int64         // Максимальное число запросов, разрешенное внутри одного окна / Maximum number of requests allowed within a single window
	currentCount int64         // Текущее количество запросов в текущем окне / Current request count inside the active window
	windowStart  time.Time     // Время начала текущего фиксированного окна / The exact start timestamp of the current fixed window
}

// NewFixedWindowLimiter создает новый лимитер фиксированных окон.
// NewFixedWindowLimiter creates a new fixed window limiter instance.
func NewFixedWindowLimiter(windowSize time.Duration, maxLimit int64) *FixedWindowLimiter {
	return &FixedWindowLimiter{
		windowSize:  windowSize,
		maxLimit:    maxLimit,
		windowStart: time.Now().Truncate(windowSize), // Округляем время до начала текущего окна / Truncate current time to the start boundary of the window
	}
}

// Allow атомарно проверяет и увеличивает счетчик внутри фиксированного временного окна.
// Allow atomically checks and increments the counter within the fixed time window.
func (b *FixedWindowLimiter) Allow() bool {
	// Блокируем мьютекс для предотвращения состояния гонки (Race Condition)
	// Lock the mutex to prevent race conditions during window evaluations
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	// Если текущее время вышло за пределы сохраненного окна, сдвигаем окно вперед
	// If the current timestamp extends past the saved window boundary, advance the window forward
	if now.Sub(b.windowStart) >= b.windowSize {
		// Вычисляем новое начало окна, округляя текущее время
		// Calculate the new window start by truncating the current timestamp
		b.windowStart = now.Truncate(b.windowSize)
		b.currentCount = 0 // Сбрасываем глобальный счетчик для нового интервала / Reset the global counter for the new interval
	}

	// Проверяем, не превышен ли лимит трафика за единицу времени
	// Verify if the traffic throughput has exceeded the limit for this time frame
	if b.currentCount < b.maxLimit {
		b.currentCount++ // Увеличиваем счетчик / Increment the request counter
		return true      // Запрос разрешен / Request is allowed
	}

	// Лимит исчерпан, жесткий сброс лишнего трафика
	// Limit reached, abruptly drop the excess traffic volume
	return false
}
