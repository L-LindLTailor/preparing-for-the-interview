package main

import (
	"fmt"
	"time"

	"notification-system/notifier" // RU: Поменяй на свой путь импорта / EN: Replace with your actual module import path
)

func main() {
	// Инициализация вывода заголовков симуляции в консоль
	// Initialize simulation headers output inside the console terminal
	fmt.Println("=== ЗАПУСК СИСТЕМЫ УВЕДОМЛЕНИЙ ===")
	fmt.Println("=== LAUNCHING FAULT-TOLERANT NOTIFICATION SYSTEM ===")

	// Настройки: макс 3 повтора, размер очереди повторов 50, лимит трафика = 5 RPS
	// Config: max 3 retries, queue buffer capacity = 50, rate limit = 5 RPS
	sys := notifier.NewNotificationSystem(3, 50, 5.0)

	// Запуск асинхронного фонового консьюмера очереди (Message Queue Consumer)
	// Activating the asynchronous background message queue consumer goroutine
	sys.Start()

	// Имитируем данные, которые СУБД PostgreSQL вернула бы из таблиц User и Device
	// Emulate data layout that PostgreSQL engine would return from User and Device schemas
	user1 := notifier.User{ID: 1, Email: "alice@example.com", Phone: "+79991112233", CountryCode: "RU"}
	user2 := notifier.User{ID: 2, Email: "bob@example.com", Phone: "+79994445566", CountryCode: "RU"}
	userBillingError := notifier.User{ID: 999, Email: "poor_user@example.com", Phone: "+70000000000", CountryCode: "RU"}

	// У одного пользователя может быть несколько активных push-токенов (Связь 1-ко-многим)
	// A single user can register multiple downstream active push tokens (1-to-Many relationship)
	device1 := notifier.Device{ID: 101, DeviceToken: "apns_apple_token_xyz_123"}

	// Массово отправляем уведомления, умышленно создавая пиковую нагрузку (Traffic Spike)
	// Multi-casting notifications rapidly, intentionally provisioning a real-time traffic spike
	fmt.Println("[TRAFFIC] Инициируем лавину уведомлений...")

	sys.SendMessage(notifier.NotificationTask{User: user1, Type: "email", Message: "Добро пожаловать в Go-Highload!"})
	sys.SendMessage(notifier.NotificationTask{User: user1, Type: "sms", Message: "Ваш код подтверждения: 5042"})
	sys.SendMessage(notifier.NotificationTask{User: user1, Device: &device1, Type: "push", Message: "Новое сообщение от инженера"})

	// Этот запрос гарантированно вызовет ошибку автоматизированного биллинга на раннем этапе фильтрации
	// This payload context is guaranteed to trigger an automated billing lock failure early in the pipe
	sys.SendMessage(notifier.NotificationTask{User: userBillingError, Type: "email", Message: "Платный отчет"})

	// Генерируем еще пачку писем, чтобы гарантированно триггернуть Rate Limiter (так как лимит выставлен в 5 RPS)
	// Push an extra batch of emails to explicitly trigger the Token Bucket shaper (due to strict 5 RPS constraint)
	for i := 0; i < 5; i++ {
		sys.SendMessage(notifier.NotificationTask{User: user2, Type: "email", Message: fmt.Sprintf("Транзакционный лог №%d", i)})
	}

	// Даем системе 2 секунды, чтобы фоновый воркер очереди успел разобрать повторные попытки (Exponential Backoff)
	// Cooldown the main runtime thread for 2 seconds to let the background workers retry failed jobs via backoff loops
	time.Sleep(2 * time.Second)

	// Безопасная остановка воркеров конвейера (Graceful Shutdown)
	// Terminating active processing streams smoothly (Graceful Shutdown sequence)
	sys.Stop()

	// ВЫВОД МОНИТОРИНГА И СБОРА МЕТРИК (ОБЕСПЕЧЕНИЕ НАБЛЮДАЕМОСТИ / OBSERVABILITY)
	// EXPORTING LIVE TRACING METRICS AND AGGREGATED TELEMETRY FOR OBSERVABILITY PLUGINS
	success, failed, retries := sys.GetMetrics()
	fmt.Println("\n=== МОНИТОРИНГ И СБОР МЕТРИК (OBSERVABILITY) ===")
	fmt.Printf("[METRICS] Успешно отправлено провайдерами: %d\n", success)
	fmt.Printf("[METRICS] Отрезано системой / Сбои: %d\n", failed)
	fmt.Printf("[METRICS] Запущено повторных попыток через Message Queue: %d\n", retries)
}
