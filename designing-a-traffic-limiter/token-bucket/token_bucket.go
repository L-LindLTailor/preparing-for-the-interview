package tokenbucket

import (
	"sync"
	"time"
)

// TokenBucketMutex реализует устойчивую к DDoS математическую модель маркерной корзины.
// TokenBucketMutex implements a DDoS-resilient mathematical model of the token bucket algorithm.
type TokenBucketMutex struct {
	mu           sync.Mutex
	capacity     float64   // Максимальный лимит токенов (вместимость корзины) / Maximum token limit (bucket capacity)
	tokens       float64   // Текущее доступное количество токенов (дробное для точности) / Current available tokens (fractional for precision)
	refillRate   float64   // Сколько токенов генерируется строго за 1 секунду / Number of tokens generated strictly per 1 second
	lastRefillTo time.Time // Точная метка времени последнего обращения к корзине / Precise timestamp of the last bucket access
}

// NewTokenBucket создает новую заполненную корзину для конкретного пользователя.
// NewTokenBucket creates a new fully charged bucket for a specific user.
func NewTokenBucket(capacity int, refillRatePerSec float64) *TokenBucketMutex {
	return &TokenBucketMutex{
		capacity:     float64(capacity),
		tokens:       float64(capacity), // Изначально корзина полностью заряжена / Bucket is initially fully charged
		refillRate:   refillRatePerSec,
		lastRefillTo: time.Now(), // Отсчет времени начинается с момента создания / Time tracking starts from the moment of creation
	}
}

// Allow проверяет возможность прохождения запроса БЕЗ блокировки потока на ожидание.
// Отрабатывает за O(1) времени и не расходует память на горутины.
// Allow checks if the request can pass WITHOUT blocking the execution thread.
// It executes in O(1) time and consumes no memory for tracking goroutines.
func (b *TokenBucketMutex) Allow() bool {
	// Блокируем мьютекс для защиты внутренних полей от Race Condition.
	// Блокировка сверхкороткая, так как внутри только базовая математика.
	// Lock the mutex to protect internal fields from Race Conditions.
	// The lock duration is extremely short since it only contains basic math.
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	// Вычисляем, сколько секунд прошло с момента последнего обращения к этой корзине
	// Calculate how many seconds have passed since the last access to this bucket
	elapsed := now.Sub(b.lastRefillTo).Seconds()
	b.lastRefillTo = now

	// ЛЕЗИ-ПОПОЛНЕНИЕ: Математически досыпаем токены за время, пока запросов не было.
	// Формула: текущие_токены + (прошедшее_время_в_сек * скорость_в_сек)
	// LAZY REFILL: Mathematically add tokens for the period during which no requests arrived.
	// Formula: current_tokens + (elapsed_time_in_sec * rate_per_sec)
	b.tokens = b.tokens + (elapsed * b.refillRate)

	// ОГРАНИЧЕНИЕ ВСПЛЕСКА: Токены не могут копиться бесконечно. Срезаем излишки по капасити.
	// BURST LIMITATION: Tokens cannot accumulate infinitely. Cap the excess values at max capacity.
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}

	// ПРОВЕРКА ЛИМИТА: Для выполнения запроса нужен как минимум 1 целый токен.
	// LIMIT CHECK: At least 1 whole token is required to execute the request.
	if b.tokens >= 1.0 {
		b.tokens--  // Списываем токен за текущий запрос / Consume a token for the current request
		return true // Запрос разрешен, уходим на выполнение бизнес-логики / Request is allowed, proceed to business logic
	}

	// Если токенов < 1.0, баланс не уменьшаем.
	// Мгновенно возвращаем false. Поток освобождается, DDoS успешно отражен.
	// If tokens < 1.0, do not decrement the balance.
	// Instantly return false. The thread is released, successfully mitigating DDoS.
	return false
}

// RateLimiterManager координирует распределение персональных корзин между клиентами.
// RateLimiterManager coordinates the distribution of personal buckets among clients.
type RateLimiterManager struct {
	mu         sync.RWMutex                 // RWMutex эффективен, так как чтение корзин происходит чаще создания / RWMutex is efficient since bucket reads occur more frequently than creations
	buckets    map[string]*TokenBucketMutex // Хранилище пар "Идентификатор -> Корзина" / Storage map for "Identifier -> Bucket" pairs
	capacity   int                          // Дефолтная емкость для новых корзин / Default capacity for new buckets
	refillRate float64                      // Дефолтная скорость для новых корзин / Default refill rate for new buckets
}

// NewRateLimiterManager инициализирует потокобезопасный менеджер корзин.
// NewRateLimiterManager initializes a thread-safe bucket manager.
func NewRateLimiterManager(capacity int, refillRate float64) *RateLimiterManager {
	return &RateLimiterManager{
		buckets:    make(map[string]*TokenBucketMutex),
		capacity:   capacity,
		refillRate: refillRate,
	}
}

// GetLimiter возвращает существующий лимитер пользователя или безопасно создает новый.
// GetLimiter returns an existing user limiter or safely creates a new one.
func (m *RateLimiterManager) GetLimiter(key string) *TokenBucketMutex {
	// Сначала проверяем существование под R-блокировкой (быстрый путь)
	// First, check existence under an R-Lock (fast path)
	m.mu.RLock()
	bucket, exists := m.buckets[key]
	m.mu.RUnlock()

	if exists {
		return bucket
	}

	// Если корзины нет, запрашиваем W-блокировку для модификации map (медленный путь)
	// If the bucket does not exist, acquire a W-Lock to modify the map (slow path)
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check паттерн: проверяем снова, не создал ли её другой поток, пока мы ждали мьютекс
	// Double-check pattern: verify again in case another thread created it while we waited for the mutex
	bucket, exists = m.buckets[key]
	if !exists {
		bucket = NewTokenBucket(m.capacity, m.refillRate)
		m.buckets[key] = bucket
	}

	return bucket
}
