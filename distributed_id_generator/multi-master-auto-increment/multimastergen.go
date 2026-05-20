package multimastergen

import (
	"sync/atomic"
)

// MultiMasterGen реализует схему автоинкремента с шагом для конкретной ноды.
// MultiMasterGen implements the auto-increment step scheme for a specific node.
type MultiMasterGen struct {
	currentID atomic.Uint64 // Текущее значение счетчика / Current counter value
	step      uint64        // Шаг инкремента (равен общему числу серверов) / Increment step (equals total server count)
	serverID  uint64        // Смещение текущего сервера / Offset index of the current server
}

// NewMultiMasterGen инициализирует генератор для конкретного мастера.
// NewMultiMasterGen initializes the generator for a specific master instance.
func NewMultiMasterGen(serverID, totalServers uint64) *MultiMasterGen {
	return &MultiMasterGen{
		step:     totalServers,
		serverID: serverID,
	}
}

// NextID генерирует следующий уникальный ID за O(1) времени без блокировок процессора.
// NextID provisions the next unique ID in O(1) time complexity utilizing lock-free atomics.
func (g *MultiMasterGen) NextID() uint64 {
	for {
		current := g.currentID.Load()
		var next uint64

		if current == 0 {
			// Самый первый шаг инициализации сервера / The very first server initialization step
			next = g.serverID
		} else {
			next = current + g.step
		}

		// Атомарно обновляем значение, защищая от состояния гонки / Atomically swap the values, preventing data races
		if g.currentID.CompareAndSwap(current, next) {
			return next
		}
	}
}
