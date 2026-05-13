package tokenbucket

import (
	"sync"
	"time"
)

// TokenBucketMutex реализует устойчивую к DDoS математическую модель маркерной корзины.
type TokenBucketMutex struct {
	mu           sync.Mutex
	capacity     float64   // Максимальный лимит токенов (вместимость корзины)
	tokens       float64   // Текущее доступное количество токенов (дробное для точности)
	refillRate   float64   // Сколько токенов генерируется строго за 1 секунду
	lastRefillTo time.Time // Точная метка времени последнего обращения к корзине
}

// NewTokenBucket создает новую заполненную корзину для конкретного пользователя.
func NewTokenBucket(capacity int, refillRatePerSec float64) *TokenBucketMutex {
	return &TokenBucketMutex{
		capacity:     float64(capacity),
		tokens:       float64(capacity), // Изначально корзина полностью заряжена
		refillRate:   refillRatePerSec,
		lastRefillTo: time.Now(), // Отсчет времени начинается с момента создания
	}
}

// Allow проверяет возможность прохождения запроса БЕЗ блокировки потока на ожидание.
// Отрабатывает за O(1) времени и не расходует память на горутины.
func (b *TokenBucketMutex) Allow() bool {
	// Блокируем мьютекс для защиты внутренних полей от Race Condition.
	// Блокировка сверхкороткая, так как внутри только базовая математика.
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	// Вычисляем, сколько секунд прошло с момента последнего обращения к этой корзине
	elapsed := now.Sub(b.lastRefillTo).Seconds()
	b.lastRefillTo = now

	// ЛЕЗИ-ПОПОЛНЕНИЕ: Математически досыпаем токены за время, пока запросов не было.
	// Формула: текущие_токены + (прошедшее_время_в_сек * скорость_в_сек)
	b.tokens = b.tokens + (elapsed * b.refillRate)

	// ОГРАНИЧЕНИЕ ВСПЛЕСКА: Токены не могут копиться бесконечно. Срезаем излишки по капасити.
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}

	// ПРОВЕРКА ЛИМИТА: Для выполнения запроса нужен как минимум 1 целый токен.
	if b.tokens >= 1.0 {
		b.tokens--  // Списываем токен за текущий запрос
		return true // Запрос разрешен, уходим на выполнение бизнес-логики
	}

	// Если токенов < 1.0, баланс не уменьшаем.
	// Мгновенно возвращаем false. Поток освобождается, DDoS успешно отражен.
	return false
}

// RateLimiterManager координирует распределение персональных корзин между клиентами.
type RateLimiterManager struct {
	mu         sync.RWMutex                 // RWMutex эффективен, так как чтение корзин происходит чаще создания
	buckets    map[string]*TokenBucketMutex // Хранилище пар "Идентификатор -> Корзина"
	capacity   int                          // Дефолтная емкость для новых корзин
	refillRate float64                      // Дефолтная скорость для новых корзин
}

// NewRateLimiterManager инициализирует потокобезопасный менеджер корзин.
func NewRateLimiterManager(capacity int, refillRate float64) *RateLimiterManager {
	return &RateLimiterManager{
		buckets:    make(map[string]*TokenBucketMutex),
		capacity:   capacity,
		refillRate: refillRate,
	}
}

// GetLimiter возвращает существующий лимитер пользователя или безопасно создает новый.
func (m *RateLimiterManager) GetLimiter(key string) *TokenBucketMutex {
	// Сначала проверяем существование под R-блокировкой (быстрый путь)
	m.mu.RLock()
	bucket, exists := m.buckets[key]
	m.mu.RUnlock()

	if exists {
		return bucket
	}

	// Если корзины нет, запрашиваем W-блокировку для модификации map (медленный путь)
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check паттерн: проверяем снова, не создал ли её другой поток, пока мы ждали мьютекс
	bucket, exists = m.buckets[key]
	if !exists {
		bucket = NewTokenBucket(m.capacity, m.refillRate)
		m.buckets[key] = bucket
	}

	return bucket
}
