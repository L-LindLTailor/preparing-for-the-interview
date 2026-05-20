package idgen

import (
	"testing"

	flickrticketserver "distributed_id_generator/flickr-ticket-server"
	multimastergen "distributed_id_generator/multi-master-auto-increment"
	twittersnowflakeid "distributed_id_generator/twitter-snowflake-id"
	uuidgen "distributed_id_generator/uuid-gen"
)

// Глобальные переменные-приемники для предотвращения оптимизаций компилятора
// Global sink variables to block compiler optimizations (dead code elimination)
var (
	sinkUint64 uint64
	sinkString string
)

// ============================================================================
// БЕНЧМАРК 1: РЕПЛИКАЦИЯ С НЕСКОЛЬКИМИ ИСТОЧНИКАМИ (LOCK-FREE ATOMICS)
// ============================================================================
func BenchmarkMultiMaster(b *testing.B) {
	// Инициализируем генератор (Сервер 1 из 3)
	// Initialize the generator (Server 1 out of 3)
	gen := multimastergen.NewMultiMasterGen(1, 3)

	var r uint64

	b.ResetTimer() // Сбрасываем таймер перед запуском цикла / Reset timer before execution loop
	for i := 0; i < b.N; i++ {
		r = gen.NextID()
	}

	sinkUint64 = r
}

// ============================================================================
// БЕНЧМАРК 2: UNIVERSALLY UNIQUE IDENTIFIER (UUIDv4)
// ============================================================================
func BenchmarkUUIDv4(b *testing.B) {
	var r string

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Каждая генерация задействует crypto/rand и форматирование строки
		// Each generation invokes crypto/rand and string formatting
		id, _ := uuidgen.GenerateUUIDv4()
		r = id
	}

	sinkString = r
}

// ============================================================================
// БЕНЧМАРК 3: СЕРВЕР ТИКЕТОВ (MUTEX СИНХРОНИЗАЦИЯ)
// ============================================================================
func BenchmarkTicketServer(b *testing.B) {
	srv := flickrticketserver.NewTicketServer(0)
	var r uint64

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r = srv.GetTicket()
	}

	sinkUint64 = r
}

// ============================================================================
// БЕНЧМАРК 4: TWITTER SNOWFLAKE (ПОБИТОВЫЕ СДВИГИ В РЕГИСТРАХ CPU)
// ============================================================================
func BenchmarkTwitterSnowflake(b *testing.B) {
	// Нода в Дата-центре 1, Сервер 1
	node, _ := twittersnowflakeid.NewSnowflakeNode(1, 1)
	var r uint64

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id, _ := node.Generate()
		r = id
	}

	sinkUint64 = r
}

// ============================================================================
// Результаты 4: TWITTER SNOWFLAKE (ПОБИТОВЫЕ СДВИГИ В РЕГИСТРАХ CPU)
// ============================================================================
/*
>> go test -bench . -benchmem .\bench\latecy_test.go
	goos: windows
	goarch: amd64
	cpu: Intel(R) Core(TM) i7-10750H CPU @ 2.60GHz
	BenchmarkMultiMaster-12			166427056	7.143 ns/op		0 B/op		0 allocs/op
	BenchmarkUUIDv4-12				2248009		540.3 ns/op		184 B/op	7 allocs/op
	BenchmarkTicketServer-12		90023180	13.42 ns/op		0 B/op		0 allocs/op
	BenchmarkTwitterSnowflake-12	1851510		765.2 ns/op		0 B/op		0 allocs/op
	PASS
	ok      command-line-arguments  7.153s
*/
