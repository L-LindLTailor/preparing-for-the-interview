package main

import (
	"log"
	"net/http"

	tokenbucket "workout-0/preparing-for-the-interview/designing-a-traffic-limiter/token-bucket"
)

func main() {
	http.HandleFunc("/", handleRequest)

	log.Println("Сервер запущен на порту :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// 1. Создаем один глобальный лимитер на всё приложение.
// Сервер физически не примет больше 5000 запросов в секунду суммарно от всех.
var globalLimiter = tokenbucket.NewTokenBucket(5000, 5000.0)

// 2. Создаем менеджер для персональных лимитеров пользователей.
// Каждый отдельный пользователь не может делать больше 5 запросов в секунду.
var userManager = tokenbucket.NewRateLimiterManager(5, 5.0)

func handleRequest(w http.ResponseWriter, r *http.Request) {
	// СЛОЙ 1: Глобальная защита сервера
	// Если набежал миллион пользователей, этот барьер пропустит только первые 5000,
	// а остальные 995 000 будут мгновенно сброшены здесь, не нагружая систему.
	if !globalLimiter.Allow() {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Сервер перегружен. Попробуйте позже."))
		return
	}

	// СЛОЙ 2: Защита от спама конкретного пользователя
	clientID := r.Header.Get("X-User-ID")
	if clientID == "" {
		clientID = r.RemoteAddr
	}

	userLimiter := userManager.GetLimiter(clientID)
	if !userLimiter.Allow() {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Вы превысили свой персональный лимит запросов."))
		return
	}

	// Бизнес-логика...
	w.WriteHeader(http.StatusOK)
}
