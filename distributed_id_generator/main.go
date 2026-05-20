package main

import (
	"fmt"
	"log"

	// Подставь свой путь к модулю / Replace with your actual module import path
	flickrticketserver "distributed_id_generator/flickr-ticket-server"
	multimastergen "distributed_id_generator/multi-master-auto-increment"
	twittersnowflakeid "distributed_id_generator/twitter-snowflake-id"
	uuidgen "distributed_id_generator/uuid-gen"
)

func main() {
	fmt.Println("=== СРАВНЕНИЕ СПОСОБОВ ГЕНЕРАЦИИ ID В РАСПРЕДЕЛЕННЫХ СИСТЕМАХ ===")
	fmt.Println("=== COMPARING ID GENERATION PATHWAYS IN DISTRIBUTED ENVIROMENTS ===\n")

	// 1. Способ: Репликация с несколькими источниками (Сервер №2 из 5)
	multiMaster := multimastergen.NewMultiMasterGen(2, 5)
	fmt.Printf("[1. Multi-Master] ID от Сервера №2: %d, Следующий: %d\n", multiMaster.NextID(), multiMaster.NextID())

	// 2. Способ: UUIDv4
	uuidStr, _ := uuidgen.GenerateUUIDv4()
	fmt.Printf("[2. UUIDv4] Автономный строковый ID: %s (Длина: %d байт)\n", uuidStr, len(uuidStr))

	// 3. Способ: Сервер тикетов (Централизованный пул)
	ticketSrv := flickrticketserver.NewTicketServer(1000)
	fmt.Printf("[3. Ticket Server] Выдан тикет по сети: %d\n", ticketSrv.GetTicket())

	// 4. Способ: Twitter Snowflake
	snowflake, err := twittersnowflakeid.NewSnowflakeNode(1, 5) // ДЦ 1, Сервер 5
	if err != nil {
		log.Fatal(err)
	}
	sfID, _ := snowflake.Generate()
	fmt.Printf("[4. Snowflake] Собранный битовый ID: %d (Компактные 64 бита)\n", sfID)
}
