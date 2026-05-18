package main

import (
	"log"
	"net/http"

	fixedwindowcounter "designing-a-traffic-limiter/fixed-window-counter"
	leakingbucket "designing-a-traffic-limiter/leaking-bucket"
	slidingwindowcounter "designing-a-traffic-limiter/sliding-window-counter"
	slidingwindowlog "designing-a-traffic-limiter/sliding-window-log"
	tokenbucket "designing-a-traffic-limiter/token-bucket"
)

var (
	/*** SlidingWindowCounter ***/
	slidingWindowCounterLimiter = slidingwindowcounter.NewSlidingWindowCounterLimiter(5.0, 3)

	/*** SlidingWindowLog ***/
	slidingWindowLogLimiter = slidingwindowlog.NewSlidingWindowLogLimiter(5.0, 3)

	/*** FixedWindowCounter ***/
	/*	Размер окна (windowSize) = 5 секунд: вся временная шкала делится на жесткие статические
		отрезки времени длительностью ровно по 5 секунд каждый
		(например, 00:00–00:05, 00:05–00:10 и так далее).
		Округление и привязка к сетке происходят автоматически.
		Максимальный лимит (maxLimit) = 3 запроса: суммарно внутри одного 5-секундного окна система
		пропустит не более 3 запросов.
		Все последующие запросы в рамках этого же окна будут мгновенно
		отклоняться с кодом 429 Too Many Requests.Как это работает на практике:
		если пользователь присылает 3 запроса на 1-й секунде окна, они успешно обрабатываются.
		На 2-й, 3-й и 4-й секундах этого же окна любые новые запросы от
		этого пользователя будут блокироваться.
		На 5-й секунде окно закроется, счетчик сбросится в 0, и пользователь снова сможет
		сделать до 3 запросов.
		Уязвимость: из-за фиксированной границы окон пользователь может совершить «всплеск» и
		прислать 3 запроса в самом конце первого окна (например, на отметке 00:04.9)
		и еще 3 запроса в самом начале второго окна (на отметке 00:05.1).
		В итоге за промежуток всего в 200 миллисекунд сервер обработает 6 запросов,
		что в два раза превышает установленный лимит безопасности.

		Window Size (windowSize) = 5 seconds: the continuous timeline is segmented into rigid,
		static time frames lasting exactly 5 seconds each (e.g., 00:00–00:05, 00:05–00:10, etc.).
		Time truncation and grid alignment occur automatically under the hood.Maximum Limit
		(maxLimit) = 3 requests:The system allows a combined maximum of 3 requests to pass
		within any single 5-second interval.
		Any additional requests arriving during the same active window will be abruptly dropped
		with an HTTP 429 Too Many Requests error.
		Practical Behavior: if a client sends 3 requests during the 1st second of the window,
		they are processed successfully.
		During the 2nd, 3rd, and 4th seconds of that same window, any subsequent requests from
		this client will be blocked. Once the 5th second passes, the window expires, the counter
		resets back to 0, and the client is allowed another batch of up to 3 requests.
		Vulnerability: due to the rigid window boundaries, a client can execute a "boundary burst"
		attack by sending 3 requests at the tail end of the first window (e.g., at 00:04.9)
		followed immediately by 3 more requests at the very beginning of the next window
		(e.g., at 00:05.1). As a result, the server ends up processing 6 requests within
		a tiny 200-millisecond span, which effectively doubles the expected safe throughput threshold. */
	fixedWindowCounterLimiter = fixedwindowcounter.NewFixedWindowLimiter(5.0, 3)

	/*** LeakingBucket ***/
	// Ограничение: строго 5 запросов в секунду (интервал 200мс).
	// Максимальный буфер очереди — 3 запроса.
	// Limit: strictly 5 requests per second (200ms interval).
	// Maximum queue buffer — 3 requests.
	leakingBucketLimiter = leakingbucket.NewLeakyBucket(5.0, 3)

	/*** TokenBucket ***/
	// 1. Создаем один глобальный лимитер на всё приложение.
	// Сервер физически не примет больше 5000 запросов в секунду суммарно от всех.
	// 1. Create a single global limiter for the entire application.
	// The server physically will not accept more than 5000 requests per second in total from everyone.
	tokenBucketGlobalLimiter = tokenbucket.NewTokenBucket(5000, 5000.0)

	// 2. Создаем менеджер для персональных лимитеров пользователей.
	// Каждый отдельный пользователь не может делать больше 5 запросов в секунду.
	// 2. Create a manager for personal user limiters.
	// Each individual user cannot make more than 5 requests per second.
	tokenBucketUserManager = tokenbucket.NewRateLimiterManager(5, 5.0)
)

func main() {
	http.HandleFunc("/token-bucket", TokenBucket)
	http.HandleFunc("/leaking-bucket", LeakingBucket)
	http.HandleFunc("/fixed-window-counter", FixedWindowCounter)
	http.HandleFunc("/sliding-windowLog", SlidingWindowLog)
	http.HandleFunc("/sliding-window-counter", SlidingWindowCounter)

	// Сервер запущен на порту :8080...
	// Server started on port :8080...
	log.Println("Сервер запущен на порту :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

/*** SlidingWindowLog ***/
func SlidingWindowCounter(w http.ResponseWriter, _ *http.Request) {
	if slidingWindowCounterLimiter.Allow() {
		w.WriteHeader(http.StatusOK)
		// Пропущено равномерно
		// Processed smoothly
		w.Write([]byte("Пропущено равномерно"))
	} else {
		w.WriteHeader(http.StatusTooManyRequests)
		// Ведро переполнено. Слишком частые запросы.
		// Bucket overflow. Too frequent requests.
		w.Write([]byte("Ведро переполнено. Слишком частые запросы."))
	}
}

/*** SlidingWindowLog ***/
func SlidingWindowLog(w http.ResponseWriter, _ *http.Request) {
	if slidingWindowLogLimiter.Allow() {
		w.WriteHeader(http.StatusOK)
		// Пропущено равномерно
		// Processed smoothly
		w.Write([]byte("Пропущено равномерно"))
	} else {
		w.WriteHeader(http.StatusTooManyRequests)
		// Ведро переполнено. Слишком частые запросы.
		// Bucket overflow. Too frequent requests.
		w.Write([]byte("Ведро переполнено. Слишком частые запросы."))
	}
}

/*** FixedWindowCounter ***/
func FixedWindowCounter(w http.ResponseWriter, _ *http.Request) {
	if leakingBucketLimiter.Allow() {
		w.WriteHeader(http.StatusOK)
		// Пропущено равномерно
		// Processed smoothly
		w.Write([]byte("Пропущено равномерно"))
	} else {
		w.WriteHeader(http.StatusTooManyRequests)
		// Ведро переполнено. Слишком частые запросы.
		// Bucket overflow. Too frequent requests.
		w.Write([]byte("Ведро переполнено. Слишком частые запросы."))
	}
}

/*** LeakingBucket ***/
func LeakingBucket(w http.ResponseWriter, _ *http.Request) {
	if leakingBucketLimiter.Allow() {
		w.WriteHeader(http.StatusOK)
		// Пропущено равномерно
		// Processed smoothly
		w.Write([]byte("Пропущено равномерно"))
	} else {
		w.WriteHeader(http.StatusTooManyRequests)
		// Ведро переполнено. Слишком частые запросы.
		// Bucket overflow. Too frequent requests.
		w.Write([]byte("Ведро переполнено. Слишком частые запросы."))
	}
}

/*** TokenBucket ***/
func TokenBucket(w http.ResponseWriter, r *http.Request) {
	// СЛОЙ 1: Глобальная защита сервера
	// Если набежал миллион пользователей, этот барьер пропустит только первые 5000,
	// а остальные 995 000 будут мгновенно сброшены здесь, не нагружая систему.
	// LAYER 1: Global server protection
	// If a million users rush in, this barrier will only allow the first 5000,
	// and the remaining 995,000 will be dropped here instantly without stressing the system.
	if !tokenBucketGlobalLimiter.Allow() {
		w.WriteHeader(http.StatusTooManyRequests)
		// Сервер перегружен. Попробуйте позже.
		// Server overloaded. Please try again later.
		w.Write([]byte("Сервер перегружен. Попробуйте позже."))
		return
	}

	// СЛОЙ 2: Защита от спама конкретного пользователя
	// LAYER 2: Protection against spam from a specific user
	clientID := r.Header.Get("X-User-ID")
	if clientID == "" {
		clientID = r.RemoteAddr
	}

	userLimiter := tokenBucketUserManager.GetLimiter(clientID)
	if !userLimiter.Allow() {
		w.WriteHeader(http.StatusTooManyRequests)
		// Вы превысили свой персональный лимит запросов.
		// You have exceeded your personal request limit.
		w.Write([]byte("Вы превысили свой персональный лимит запросов."))
		return
	}

	// Бизнес-логика...
	// Business logic...
	w.WriteHeader(http.StatusOK)
}
