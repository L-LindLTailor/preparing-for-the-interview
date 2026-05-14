package slidingwindowcounter

import (
	"sync"
	"time"
)

// SlidingWindowCounterLimiter реализует оптимизированный алгоритм счетчика скользящих окон.
// SlidingWindowCounterLimiter implements an optimized sliding window counter rate limiting algorithm.
type SlidingWindowCounterLimiter struct {
	mu           sync.Mutex
	windowSize   time.Duration // Размер окна (например, 1 * time.Minute) / The duration of the window (e.g., 1 * time.Minute)
	maxLimit     int64         // Максимальное количество запросов за скользящий интервал / Maximum request threshold allowed within the sliding window
	currentStart time.Time     // Время начала текущего фиксированного окна / Start timestamp of the current active fixed window
	currentCount int64         // Счетчик запросов в текущем окне / Total request count inside the current active window
	prevCount    int64         // Счетчик запросов в предыдущем окне / Total request count inside the immediate previous window
}

// NewSlidingWindowCounterLimiter создает новый лимитер на базе счетчика скользящих окон.
// NewSlidingWindowCounterLimiter creates a new sliding window counter limiter instance.
func NewSlidingWindowCounterLimiter(windowSize time.Duration, maxLimit int64) *SlidingWindowCounterLimiter {
	return &SlidingWindowCounterLimiter{
		windowSize:   windowSize,
		maxLimit:     maxLimit,
		currentStart: time.Now().Truncate(windowSize), // Округляем до базовой сетки времени / Truncate current time to fit the static time grid
	}
}

// Allow проверяет лимит трафика с математической аппроксимацией веса двух смежных окон.
// Allow evaluates the traffic threshold using a mathematical approximation based on the weights of two adjacent windows.
func (b *SlidingWindowCounterLimiter) Allow() bool {
	// Блокируем мьютекс для предотвращения состояния гонки (Race Condition)
	// Lock the mutex to prevent race conditions during window factor evaluation
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	// Сдвиг окон во времени (Lazy Window Rotation)
	// Evaluate window rotation reactively based on elapsed intervals
	elapsedWindows := now.Sub(b.currentStart) / b.windowSize

	if elapsedWindows >= 2 {
		// Прошло много времени, оба предыдущих окна полностью устарели
		// Significant time has passed, both older windows are completely stale
		b.currentStart = now.Truncate(b.windowSize)
		b.currentCount = 0
		b.prevCount = 0
	} else if elapsedWindows == 1 {
		// Наступило ровно следующее окно. Текущее становится предыдущим, а новое обнуляется.
		// Exactly one window duration elapsed. Current metrics shift to previous, and new slate is zeroed out.
		b.currentStart = b.currentStart.Add(b.windowSize)
		b.prevCount = b.currentCount
		b.currentCount = 0
	}

	// Вычисляем, какая доля текущего окна уже прожита (значение от 0.0 до 1.0)
	// Calculate what fraction of the current active window has already elapsed (value from 0.0 to 1.0)
	timePassedInCurrentWindow := now.Sub(b.currentStart).Seconds()
	currentWindowWeight := timePassedInCurrentWindow / b.windowSize.Seconds()
	prevWindowWeight := 1.0 - currentWindowWeight

	// Математически рассчитываем скользящее количество запросов за интервал
	// Mathematically extrapolate the total sliding request load within the window span
	estimatedRequests := float64(b.prevCount)*prevWindowWeight + float64(b.currentCount)

	// Проверяем, укладываемся ли мы в лимит безопасности
	// Verify if the calculated load stays strictly below the safe maximum limit threshold
	if int64(estimatedRequests) < b.maxLimit {
		b.currentCount++ // Увеличиваем счетчик текущего активного интервала / Increment the request counter for the active frame
		return true      // Запрос успешно разрешен / Request allowed successfully
	}

	// Лимит превышен, выдаем мгновенный отказ
	// Out of capacity boundary, trigger an immediate denial response
	return false
}
