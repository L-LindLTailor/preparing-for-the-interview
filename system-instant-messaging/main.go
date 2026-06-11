package main

import (
	"log"
	"net"

	"news-feed-system/pb" // Сгенерированный gRPC пакет / Generated gRPC package

	"google.golang.org/grpc"
)

func main() {
	// 1. Открываем сетевой TCP-порт 50053 для обработки бинарных фреймов HTTP/2 чат-ядра
	// 1. Spawning network TCP listener on port 50053 to handle binary HTTP/2 frames of the IM core
	listener, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatalf("[FATAL] Не удалось открыть TCP-порт 50053: %v / Failed to bind TCP port 50053: %v", err, err)
	}

	// 2. Инициализируем высокопроизводительный gRPC-сервер
	// 2. Initializing high-performance gRPC server instance
	grpcServer := grpc.NewServer()

	// 3. Создаем экземпляр нашей бизнес-логики чата
	// 3. Instantiating our core chat business logic server
	serverImpl := NewIMServer()

	// 4. Регистрируем чат-движок в рантайме gRPC
	// 4. Registering the chat engine within the gRPC runtime stack
	pb.RegisterInstantMessagingEngineServer(grpcServer, serverImpl)

	log.Printf("[IM ENGINE] 🚀 gRPC Чат-ядро успешно запущено на порту :50053 / gRPC IM Engine successfully spawned on port :50053")

	// 5. Запускаем бесконечный цикл обработки входящих пакетов планировщиком GMP
	// 5. Commencing infinite processing event loop under control of the GMP scheduler
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("[FATAL] Крах сервера чата: %v / Chat server runtime panic: %v", err, err)
	}
}
