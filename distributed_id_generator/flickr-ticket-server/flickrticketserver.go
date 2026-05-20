package flickrticketserver

import (
	"sync"
)

// TicketServer имитирует централизованную ноду выдачи последовательных ID.
// TicketServer emulates a centralized sequential ID distribution node.
type TicketServer struct {
	mu      sync.Mutex
	counter uint64 // Единый глобальный счетчик / The underlying sequential counter
}

func NewTicketServer(startValue uint64) *TicketServer {
	return &TicketServer{counter: startValue}
}

// GetTicket выдает следующий монотонный ID по сети (имитация).
// GetTicket distributes the next monotonic ID over the network mesh (emulated).
func (ts *TicketServer) GetTicket() uint64 {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.counter++
	return ts.counter
}
