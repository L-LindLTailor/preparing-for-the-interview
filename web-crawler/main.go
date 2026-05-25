package main

import (
	"fmt"
	"time"
	"web-crawler/crawler" // Замени на твой путь импорта / Replace with your actual module import path
)

func main() {
	fmt.Println("=== ЗАПУСК ПОТОКОБЕЗОПАСНОГО ВЕБ-КРАУЛЕРА ===")
	fmt.Println("=== LAUNCHING CONCURRENT PRODUCTION-READY WEB CRAWLER ===")

	// Создаем краулер: 5 параллельных воркеров, буфер Frontier очереди = 100
	// Initialize crawler: 5 concurrent workers, Frontier pipeline capacity = 100
	pipe := crawler.NewWebCrawler(5, 100)

	// Стартовые точки обхода (Seed URLs)
	// Seed URLs configuration boundary
	seeds := []string{
		"https://go.dev",
		"https://habr.com",
		"https://github.com",
	}

	// Запуск конвейера обхода
	// Trigger the reactive web crawl pipeline
	results := pipe.Start(seeds)

	// Таймер для автоматической остановки краулера через 10 секунд
	// Fail-safe timer to interrupt the frontier loop after 10 seconds
	stopTimer := time.After(10 * time.Second)

	for {
		select {
		case res, ok := <-results:
			if !ok {
				fmt.Println("\n[FINISH] Все доступные ссылки обойдены. Очередь пуста.")
				return
			}
			fmt.Printf("[CRAWLED] URL: %s -> %s\n", res.URL, res.Title)

		case <-stopTimer:
			fmt.Println("\n[TIMEOUT] Время вышло. Принудительная остановка URL Frontier...")
			pipe.Stop() // Закрываем входящую очередь, воркеры доработают текущие задачи и выйдут
		}
	}
}
