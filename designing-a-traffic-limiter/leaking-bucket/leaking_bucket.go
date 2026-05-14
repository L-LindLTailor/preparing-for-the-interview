package leakybucket

import (
	"sync"
	"time"
)

// LeakyBucketLimiter реализует алгоритм Дырявого Ведра без фоновых горутин.
// LeakyBucketLimiter implements the Leaky Bucket algorithm without background goroutines.
type LeakyBucketLimiter struct {
	mu           sync.Mutex
	interval     time.Duration // Жесткий интервал между запросами (например, 200 мс для скорости 5 запр/сек) / Strict interval between requests (e.g., 200 ms for 5 req/sec rate)
	maxQueueTime time.Duration // Максимальная глубина виртуальной очереди (емкость ведра во времени) / Maximum depth of the virtual queue (bucket capacity in terms of time)
	nextFreeTime time.Time     // Время, когда ведро полностью опустеет / The exact timestamp when the bucket will become completely empty
}

// NewLeakyBucket создает новый лимитер.
// ratePerSec — сколько запросов в секунду мы строго хотим выпускать.
// maxQueueSize — сколько запросов может одновременно "постоять в очереди", излишки сбросятся.
// NewLeakyBucket creates a new limiter.
// ratePerSec — the exact number of requests per second we strictly intend to process.
// maxQueueSize — the number of requests that can simultaneously "wait in line"; excess requests are dropped.
func NewLeakyBucket(ratePerSec float64, maxQueueSize int) *LeakyBucketLimiter {
	interval := time.Duration(float64(time.Second) / ratePerSec)
	return &LeakyBucketLimiter{
		interval: interval,
		// Переводим емкость очереди в эквивалент времени
		// Convert queue capacity into its time-based equivalent
		maxQueueTime: interval * time.Duration(maxQueueSize),
		nextFreeTime: time.Now(),
	}
}

// Allow определяет, укладывается ли запрос в рамки "капающего" ведра
// Allow determines whether the incoming request fits within the boundaries of the "leaking" bucket
func (b *LeakyBucketLimiter) Allow() bool {
	// Блокируем мьютекс для предотвращения состояния гонки (Race Condition)
	// Lock the mutex to prevent race conditions during time calculation
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	// Если ведро уже успело полностью "вытечь" до прихода текущего запроса
	// If the bucket has already leaked completely dry before the current request arrived
	if now.After(b.nextFreeTime) {
		// Сервер свободен, планируем следующий интервал от текущего момента
		// The server is free, schedule the next interval starting from the current moment
		b.nextFreeTime = now.Add(b.interval)
		return true
	}

	// Если ведро еще "капает", вычисляем, как далеко в будущее ушла виртуальная очередь
	// Если мы добавим текущий запрос, не переполнится ли ведро?
	// If the bucket is still "leaking", calculate how far into the future the virtual queue extends.
	// Will the bucket overflow if we accept the current request?
	if b.nextFreeTime.Add(b.interval).Sub(now) > b.maxQueueTime {
		// Очередь переполнена! Мгновенный сброс атаки
		// The queue is full! Immediate drop of the attack volume
		return false
	}

	// Место в очереди есть. Сдвигаем время освобождения ведра вперед
	// Space is available in the queue. Shift the bucket's release time further forward
	b.nextFreeTime = b.nextFreeTime.Add(b.interval)
	return true
}
