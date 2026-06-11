package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"
	"unicode/utf8"

	"news-feed-system/pb" // Путь к Protobuf-каркасу / Path to generated Protobuf code

	"google.golang.org/protobuf/types/known/timestamppb"
)

// UserActivity хранит метку времени последнего действия пользователя для Heartbeat-пресенса
// UserActivity encapsulates the timestamp of the last tracked user action for heartbeat presence tracking
type UserActivity struct {
	LastSeen time.Time
}

// IMServer оркеструет Full-Duplex бинарные потоки сообщений и кэширует сессии в RAM
// IMServer orchestrates Full-Duplex binary message streams and caches runtime sessions in RAM
type IMServer struct {
	pb.UnimplementedInstantMessagingEngineServer
	activeStreams sync.Map // Карта активных gRPC стримов / Thread-safe map of active gRPC streams: UserID -> StreamServer
	presenceMap   sync.Map // Карта активности пользователей / Thread-safe presence tracking map: UserID -> UserActivity
}

func NewIMServer() *IMServer {
	return &IMServer{}
}

// 1. RealTimeChatStream — двусторонний Full-Duplex стрим для мгновенного обмена сообщениями
// 1. RealTimeChatStream — Full-Duplex bi-directional streaming for instant real-time message exchange
func (s *IMServer) RealTimeChatStream(stream pb.InstantMessagingEngine_RealTimeChatStreamServer) error {
	var userID string
	log.Println("[gRPC IM] Новое дуплексное чат-соединение инициализировано / New full-duplex chat stream initialized")

	for {
		// Выгребаем входящие бинарные фреймы из открытого сокета HTTP/2
		// Fetching incoming binary frames from the active HTTP/2 socket
		req, err := stream.Recv()
		if err == io.EOF {
			log.Printf("[gRPC IM] Клиент %s отключился штатно (EOF) / Client %s disconnected gracefully (EOF)", userID)
			s.activeStreams.Delete(userID)
			return nil
		}
		if err != nil {
			log.Printf("[gRPC IM] Ошибка соединения с клиентом %s: %v / Stream error for client %s: %v", userID, err, userID, err)
			s.activeStreams.Delete(userID)
			return err
		}

		// Авторизуем сессию при первом прилетевшем пакете
		// Authorize the session upon receiving the first data packet
		if userID == "" {
			userID = fmt.Sprintf("usr_%s", req.ClientUuid[:4])
			s.activeStreams.Store(userID, stream)
		}

		// ПУНКТ 4 ТЗ: Валидация размера сообщения строго до 1000 символов (учитывая руны UTF-8)
		// SRS REQ 4: Validating message text size strictly up to 1000 characters (counting UTF-8 runes)
		runeCount := utf8.RuneCountInString(req.TextContent)
		if runeCount > 1000 {
			log.Printf("[SECURITY] Отклонено сообщение от %s: %d символов (Лимит 1000) / Dropped message from %s: %d chars (Limit 1000)", userID, runeCount, userID, runeCount)
			// Пропускаем запись на диск, не обрывая сетевое соединение пользователя
			// Skip disk persistence layer without terminating the active user network stream
			continue
		}

		// ПУНКТ 2 ТЗ: Фиксируем активность за последнюю минуту (Обновляем Heartbeat)
		// SRS REQ 2: Registering activity state within the last minute (Updating Heartbeat state)
		s.presenceMap.Store(userID, UserActivity{LastSeen: time.Now()})

		log.Printf("[CHAT LOG] [%s] шлет в комнату [%s]: %s (%d рун)", userID, req.RoomId, req.TextContent, runeCount)

		// Паттерн Fan-Out: Рассылаем бинарный фрейм всем активным участникам в RAM-кластере
		// Fan-Out Pattern: Broadcasting the binary frame to all active connections within the RAM cluster
		s.activeStreams.Range(func(key, value any) bool {
			targetUID := key.(string)
			if targetUID == userID {
				return true // Пропускаем отправку самому себе / Skip eco back to sender
			}

			targetStream := value.(pb.InstantMessagingEngine_RealTimeChatStreamServer)
			_ = targetStream.Send(&pb.MessageFromServer{
				MessageId:      fmt.Sprintf("msg_%d", time.Now().UnixNano()),
				RoomId:         req.RoomId,
				SenderId:       userID,
				TextContent:    req.TextContent,
				Timestamp:      timestamppb.Now(),
				DeliveryStatus: "DELIVERED",
			})
			return true
		})
	}
}

// 2. GetChatHistory — ПУНКТ 3 и 5 ТЗ: Выдача истории строго кусками по 30 сообщений (Cursor Пагинация)
// 2. GetChatHistory — SRS REQ 3 & 5: Message history slice delivery in chunks of exactly 30 items (Cursor Pagination)
func (s *IMServer) GetChatHistory(ctx context.Context, req *pb.HistoryRequest) (*pb.HistoryResponse, error) {
	log.Printf("[gRPC HISTORY] Запрос истории для комнаты %s | Курсор: '%s' / History request for room %s | Cursor: '%s'", req.RoomId, req.CursorMessageId, req.RoomId, req.CursorMessageId)

	// Жесткое ограничение пагинации по ТЗ
	// Strict pagination envelope per SRS requirements
	limit := int(req.Limit)
	if limit != 30 {
		limit = 30
	}

	// Имитируем генерацию 30 сообщений из базы данных за константное время O(1) по B-Tree индексу
	// Simulating generation of 30 messages from the DB via B-Tree index within constant O(1) execution time
	var mockMessages []*pb.MessageFromServer
	for i := 1; i <= limit; i++ {
		msgIndex := i
		if req.CursorMessageId != "" {
			// Если курсор передан, сдвигаем историю назад по шкале смещения
			// If a cursor is present, shift the history backwards along the offset timeline
			fmt.Sscanf(req.CursorMessageId, "msg_offset_%d", &msgIndex)
			msgIndex += i
		}

		mockMessages = append(mockMessages, &pb.MessageFromServer{
			MessageId:      fmt.Sprintf("msg_offset_%d", msgIndex),
			RoomId:         req.RoomId,
			SenderId:       "usr_mock_author",
			TextContent:    fmt.Sprintf("Это историческое сообщение №%d для пагинации / This is historical message #%d for pagination", msgIndex, msgIndex),
			Timestamp:      timestamppb.New(time.Now().Add(-time.Duration(msgIndex) * time.Minute)),
			DeliveryStatus: "READ",
		})
	}

	// Вычисляем новый курсор для следующей ленивой подгрузки фронтендом еще 30 штук
	// Calculating the next cursor token for subsequent frontend lazy-loading of the next 30 items
	nextCursor := fmt.Sprintf("msg_offset_%d", req.Limit)
	if len(mockMessages) > 0 {
		nextCursor = mockMessages[len(mockMessages)-1].MessageId
	}

	return &pb.HistoryResponse{
		Messages:     mockMessages,
		NextCursorId: nextCursor,
		HasMore:      true, // Имитируем бесконечную историю для проверки подгрузки / Simulate infinite depth to test scrolling UI
	}, nil
}

// 3. GetUserPresence — ПУНКТ 2 ТЗ: Проверка статуса активности за последнюю минуту
// 3. GetUserPresence — SRS REQ 2: Verifying user online activity state within the strict 1-minute window
func (s *IMServer) GetUserPresence(ctx context.Context, req *pb.PresenceRequest) (*pb.PresenceResponse, error) {
	value, found := s.presenceMap.Load(req.UserId)

	if !found {
		// Если записи в RAM нет, значит, пользователя давно не было на сервере
		// If the record is missing from RAM, the user has been inactive for a long time
		return &pb.PresenceResponse{
			UserId:     req.UserId,
			IsOnline:   false,
			LastSeenAt: timestamppb.New(time.Now().Add(-5 * time.Hour)), // Дефолт: был 5 часов назад / Default fallback: 5 hours ago
		}, nil
	}

	activity := value.(UserActivity)
	// Проверяем физику ТЗ: прошел ли последний пинг в пределах 1 минуты (60 секунд)
	// Evaluating SRS rule: has the last heartbeat occurred within the 1-minute (60 seconds) window
	isOnline := time.Since(activity.LastSeen) <= 60*time.Second

	return &pb.PresenceResponse{
		UserId:     req.UserId,
		IsOnline:   isOnline,
		LastSeenAt: timestamppb.New(activity.LastSeen), // Точный час крайнего визита / Exact timestamp of the last activity
	}, nil
}
