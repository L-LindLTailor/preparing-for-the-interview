package notifier

import (
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// МОДЕЛИ ДАННЫХ И МЕТРИКИ / DATA MODELS & METRICS
// ============================================================================

// User инкапсулирует профиль клиента из таблицы СУБД.
// User encapsulates the customer profile from the database schema.
type User struct {
	ID          uint64
	Email       string
	Phone       string
	CountryCode string // RU: Гео-код для выбора SMS-провайдеров / EN: Country code for SMS carrier selection
}

// Device хранит push-токены мобильных устройств (связь 1-ко-многим к User).
// Device tracks mobile push tokens (1-to-Many relation to the User).
type Device struct {
	ID          uint64
	DeviceToken string
}

// NotificationMetrics собирает метрики мониторинга на базе неблокирующих атомиков.
// NotificationMetrics aggregates observability metrics via lock-free atomics.
type NotificationMetrics struct {
	SentSuccess atomic.Uint64 // Успешно отправлено провайдерами / Successfully sent by providers
	SentFailed  atomic.Uint64 // Окончательные сбои отправки / Terminal delivery failures
	Retries     atomic.Uint64 // Запущенные повторные попытки в MQ / Message Queue retry operations
}

// ============================================================================
// ОГРАНИЧИТЕЛЬ ТРАФИКА (TOKEN BUCKET) / RATE LIMITER
// ============================================================================

// RateLimiter защищает внешние API провайдеров от превышения лимитов (RPS).
// RateLimiter shields external provider APIs from exceeding rate limits (RPS).
type RateLimiter struct {
	mu         sync.Mutex
	capacity   float64
	tokens     float64
	refillRate float64
	lastRefill time.Time
}

// NewRateLimiter инициализирует лимитер по алгоритму маркерной корзины.
// NewRateLimiter initializes a rate limiter using the Token Bucket algorithm.
func NewRateLimiter(capacity int, refillRate float64) *RateLimiter {
	return &RateLimiter{
		capacity:   float64(capacity),
		tokens:     float64(capacity),
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow проверяет лимит входящего трафика за O(1) времени.
// Allow evaluates ingress throughput boundaries in O(1) time complexity.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.lastRefill = now

	// Ленивое пополнение токенов на основе прошедшего времени
	// Lazy token replenishment based on elapsed timestamps
	rl.tokens += elapsed * rl.refillRate
	if rl.tokens > rl.capacity {
		rl.tokens = rl.capacity
	}

	if rl.tokens >= 1.0 {
		rl.tokens--
		return true
	}
	return false
}

// ============================================================================
// СИСТЕМА УВЕДОМЛЕНИЙ / NOTIFICATION SYSTEM ENGINE
// ============================================================================

// NotificationTask представляет контекст сообщения, проходящего сквозь конвейер.
// NotificationTask defines the payload context moving through the pipe.
type NotificationTask struct {
	User       User
	Device     *Device
	Type       string // "email", "sms", "push"
	Message    string
	RetryCount int // Текущее количество повторов задачи / Current task retry counter
}

// NotificationSystem координирует работу лимитеров, биллинга и очередей задач.
// NotificationSystem coordinates rate shaping, billing locks, and message queues.
type NotificationSystem struct {
	metrics     *NotificationMetrics
	limiter     *RateLimiter
	retryQueue  chan NotificationTask // Наша асинхронная очередь повторов (MQ) / Our async Retry Message Queue
	maxRetries  int                   // Порог жесткого сброса задачи / Terminal retry ceiling threshold
	stopChannel chan struct{}         // Канал для Graceful Shutdown / Graceful Shutdown orchestrator stream
	wg          sync.WaitGroup
}

// NewNotificationSystem конструирует отказоустойчивый конвейер уведомлений.
// NewNotificationSystem constructs a fault-tolerant notification production pipe.
func NewNotificationSystem(maxRetries, queueSize int, rps float64) *NotificationSystem {
	return &NotificationSystem{
		metrics:     &NotificationMetrics{},
		limiter:     NewRateLimiter(int(rps), rps),
		retryQueue:  make(chan NotificationTask, queueSize),
		maxRetries:  maxRetries,
		stopChannel: make(chan struct{}),
	}
}

// Start активирует фоновые горутины-консьюмеры для разбора очереди повторов.
// Start activates background consumer goroutines to process the retry message queue.
func (ns *NotificationSystem) Start() {
	ns.wg.Add(1)
	go ns.retryWorkerLoop()
}

// Stop безопасно останавливает консьюмеры (Паттерн Graceful Shutdown).
// Stop safely shuts down pipeline consumers (Graceful Shutdown Pattern).
func (ns *NotificationSystem) Stop() {
	close(ns.stopChannel)
	ns.wg.Wait()
}

// SendMessage — центральное API, выполняющее маршрутизацию, биллинг и отправку.
// SendMessage — core API facilitating target routing, billing checks, and delivery.
func (ns *NotificationSystem) SendMessage(task NotificationTask) {
	// 1. ОГРАНИЧЕНИЕ ТРАФИКА: Срезаем пиковые всплески и уводим излишки в очередь
	// 1. RATE LIMITING: Smooth traffic spikes and redirect excess loads to the queue
	if !ns.limiter.Allow() {
		log.Printf("[RATE LIMIT] Отправка для User %d отклонена. Перенаправляем в очередь.", task.User.ID)
		ns.EnQueueRetry(task)
		return
	}

	// 2. СИСТЕМА АВТОМАТИЗИРОВАННОГО БИЛЛИНГА: Проверка баланса до вызова внешних провайдеров
	// 2. AUTOMATED BILLING GATE: Balance verification prior to invoking external providers
	if !ns.checkBilling(task.User.ID) {
		log.Printf("[BILLING] Отказ: у пользователя %d недостаточно средств.", task.User.ID)
		ns.metrics.SentFailed.Add(1)
		return
	}

	// 3. МОДУЛИ РАССЫЛКИ: Попытка передачи сообщения в сеть стороннего вендора
	// 3. DELIVERY VECTORS: Attempting payload transfer to a third-party vendor network
	var err error
	switch task.Type {
	case "email":
		err = ns.sendEmail(task.User.Email, task.Message)
	case "sms":
		err = ns.sendSMS(task.User.Phone, task.Message)
	case "push":
		if task.Device != nil {
			err = ns.sendPush(task.Device.DeviceToken, task.Message)
		} else {
			err = errors.New("no device token registered")
		}
	}

	if err != nil {
		log.Printf("[PROVIDER ERROR] Ошибка отправки %s для User %d: %v", task.Type, task.User.ID, err)
		ns.EnQueueRetry(task) // Логика ухода на повтор / Fallback to retry logic
	} else {
		ns.metrics.SentSuccess.Add(1) // Фиксируем успех / Track success milestone
	}
}

// EnQueueRetry безопасно помещает сбойную задачу в брокер очередей с контролем лимита.
// EnQueueRetry safely appends a failing task into the message broker validating thresholds.
func (ns *NotificationSystem) EnQueueRetry(task NotificationTask) {
	// Если лимит экспоненциальных попыток исчерпан — полностью удаляем сообщение
	// If exponential retry thresholds are breached — drop the message tracking failure
	if task.RetryCount >= ns.maxRetries {
		log.Printf("[GIVING UP] Превышен макс. лимит повторов (%d) для User %d.", ns.maxRetries, task.User.ID)
		ns.metrics.SentFailed.Add(1)
		return
	}

	task.RetryCount++
	ns.metrics.Retries.Add(1)

	select {
	case ns.retryQueue <- task:
		log.Printf("[QUEUED] Задача добавлена в асинхронную очередь (Попытка %d)", task.RetryCount)
	default:
		// Защита от Out Of Memory: если канал забит, отбрасываем задачу ради выживания ноды
		// Out Of Memory Shield: if the queue saturates, drop the task to preserve node stability
		log.Printf("[QUEUE FULL] Очередь переполнена! Сообщение для User %d утеряно.", task.User.ID)
		ns.metrics.SentFailed.Add(1)
	}
}

// retryWorkerLoop представляет собой фоновый цикл разбора брокера сообщений.
// retryWorkerLoop structures the infinite asynchronous Message Queue consumer loop.
func (ns *NotificationSystem) retryWorkerLoop() {
	defer ns.wg.Done()

	for {
		select {
		case task := <-ns.retryQueue:
			// Экспоненциальное сглаживание: увеличиваем паузу перед каждым повтором шлюза
			// Exponential backoff: amplify the cooldown duration before repeating the API invocation
			delay := time.Duration(task.RetryCount) * 100 * time.Millisecond
			time.Sleep(delay)

			log.Printf("[RETRY WORKER] Повторный запуск конвейера %s для User %d...", task.Type, task.User.ID)
			ns.SendMessage(task)

		case <-ns.stopChannel:
			log.Println("[SHUTDOWN] Фоновый воркер очереди уведомлений остановлен.")
			return
		}
	}
}

// ============================================================================
// ИМИТАЦИЯ ТРЕТЬИХ СТОРОН И БИЛЛИНГА / EMULATED THIRD-PARTY API PROVIDERS
// ============================================================================

func (ns *NotificationSystem) checkBilling(userID uint64) bool {
	return userID != 999
}

func (ns *NotificationSystem) sendEmail(email, msg string) error {
	// Симулируем 33% случайных сетевых таймаутов у провайдеров (SendGrid/Mailchimp)
	// Emulate 33% random network timeouts on third-party vendor APIs (SendGrid/Mailchimp)
	if time.Now().UnixNano()%3 == 0 {
		return errors.New("sendgrid upstream connection timeout")
	}
	log.Printf("[SUCCESS EMAIL] Письмо отправлено на %s: %s", email, msg)
	return nil
}

func (ns *NotificationSystem) sendSMS(phone, msg string) error {
	log.Printf("[SUCCESS SMS] СМС отправлено на номер %s: %s", phone, msg)
	return nil
}

func (ns *NotificationSystem) sendPush(token, msg string) error {
	log.Printf("[SUCCESS PUSH] Пуш отправлен на девайс %s: %s", token, msg)
	return nil
}

// GetMetrics атомарно извлекает снапшот данных для скрейперов мониторинга.
// GetMetrics atomically pulls an aggregation metrics snapshot for observability scrapers.
func (ns *NotificationSystem) GetMetrics() (uint64, uint64, uint64) {
	return ns.metrics.SentSuccess.Load(), ns.metrics.SentFailed.Load(), ns.metrics.Retries.Load()
}
