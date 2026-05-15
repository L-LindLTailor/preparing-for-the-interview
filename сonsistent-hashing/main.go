package main

import (
	"fmt"

	consistenthash "consistenthashing/consistent-hash"
)

func main() {
	// Создаем кольцо, где у каждого сервера будет по 3 виртуальных узла
	// Creating a ring where each server will have 3 virtual nodes
	ring := consistenthash.NewRing(3, nil)
	ring.AddServer("Server-A")
	ring.AddServer("Server-B")

	// Быстро находим, куда отправить пользователя
	// Quickly find where to send the user
	userKey := "user_40"
	targetServer := ring.GetServer(userKey)

	fmt.Printf("Пользователь %s направлен на %s\n", userKey, targetServer)
}
