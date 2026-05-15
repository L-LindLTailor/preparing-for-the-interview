package consistenthash

import (
	"hash/crc32"
	"sort"
	"strconv"
)

// Hash — это тип функции для кастомного хэширования (если потребуется заменить crc32)
// Hash defines a custom function type for generating hashes
type Hash func(data []byte) uint32

// Ring реализует архитектуру согласованного хэширования.
// Ring implements the consistent hashing architecture.
type Ring struct {
	hash     Hash           // Функция хэширования / The hashing function algorithm
	vnodes   int            // Количество виртуальных узлов на один реальный сервер / Number of virtual nodes per physical server
	ring     []int          // Отсортированный массив хэшей (виртуальное кольцо) / Sorted array of hashes (virtual hash ring)
	nodesMap map[int]string // Карта соответствия: Хэш узла -> Имя реального сервера / Mapping: Node hash -> Physical server name
}

// NewRing создает новый инстанс хэш-кольца.
// NewRing initializes a new consistent hash ring instance.
func NewRing(vnodes int, fn Hash) *Ring {
	r := &Ring{
		vnodes:   vnodes,
		hash:     fn,
		nodesMap: make(map[int]string),
	}
	// Если хэш-функция не передана, используем стандартный IEEE CRC32
	// Default to standard IEEE CRC32 if no custom hash function is provided
	if r.hash == nil {
		r.hash = crc32.ChecksumIEEE
	}
	return r
}

// AddServer добавляет физический сервер на кольцо, генерируя для него виртуальные узлы.
// AddServer provisions a physical server onto the ring by generating its virtual nodes.
func (r *Ring) AddServer(server string) {
	for i := 0; i < r.vnodes; i++ {
		// Создаем уникальный маркер для каждого виртуального узла
		// Create a unique token identifier for each virtual node
		vnodeKey := server + "#" + strconv.Itoa(i)
		hash := int(r.hash([]byte(vnodeKey)))

		r.ring = append(r.ring, hash)
		r.nodesMap[hash] = server
	}
	// ВАЖНО: Сортируем массив для корректной работы бинарного поиска
	// CRITICAL: Sort the slice to ensure proper binary search evaluation
	sort.Ints(r.ring)
}

// GetServer находит целевой сервер для ключа за O(log N) времени с помощью бинарного поиска.
// GetServer locates the target server for a given key in O(log N) time using binary search.
func (r *Ring) GetServer(key string) string {
	if len(r.ring) == 0 {
		return ""
	}

	hash := int(r.hash([]byte(key)))

	// Имитируем "движение по часовой стрелке" через бинарный поиск (ищем первый элемент >= hash)
	// Emulate "clockwise movement" via binary search (locating the first index >= hash)
	idx := sort.Search(len(r.ring), func(i int) bool {
		return r.ring[i] >= hash
	})

	// ЗАМЫКАНИЕ КОЛЬЦА: если ушли за правый край, возвращаемся к началу (0-й индекс)
	// RING CLOSURE: if we overshoot the right boundary, wrap around back to the beginning (0th index)
	if idx == len(r.ring) {
		idx = 0
	}

	return r.nodesMap[r.ring[idx]]
}
