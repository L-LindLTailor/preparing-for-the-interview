package kvstore

import (
	"bufio"
	"fmt"
	"hash/fnv"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// КОМПОНЕНТ 1: БЛУМ-ФИЛЬТР / COMPONENT 1: BLOOM FILTER
// ============================================================================

// BloomFilter представляет собой компактный вероятностный фильтр в RAM.
// BloomFilter represents a compact, probabilistic in-memory filter.
type BloomFilter struct {
	bitSet []bool // Битовый массив / The underlying bit array
	size   int    // Размер массива в битах / The size of the bit array
	hashes int    // Количество используемых хэш-функций / Number of hash functions to utilize
}

// NewBloomFilter инициализирует новый Блум-фильтр.
// NewBloomFilter initializes a new Bloom Filter instance.
func NewBloomFilter(size, hashes int) *BloomFilter {
	return &BloomFilter{
		bitSet: make([]bool, size),
		size:   size,
		hashes: hashes,
	}
}

// Add добавляет ключ в Блум-фильтр, выставляя соответствующие биты в true.
// Add inserts a key into the Bloom Filter, setting the respective bits to true.
func (bf *BloomFilter) Add(key string) {
	for i := 0; i < bf.hashes; i++ {
		idx := bf.getHashIndex(key, i)
		bf.bitSet[idx] = true
	}
}

// MaybeExists возвращает false, если ключа точно нет. Возвращает true, если ключ ВОЗМОЖНО есть.
// MaybeExists returns false if the key is definitely absent. Returns true if the key MIGHT be present.
func (bf *BloomFilter) MaybeExists(key string) bool {
	for i := 0; i < bf.hashes; i++ {
		idx := bf.getHashIndex(key, i)
		// Если хотя бы один бит равен false — ключа точно никогда не было в системе
		// If even a single bit is false — the key has definitely never been added
		if !bf.bitSet[idx] {
			return false
		}
	}
	return true
}

// getHashIndex вычисляет индекс в битовом массиве для конкретной хэш-функции (используя соль).
// getHashIndex computes the bit array offset index for a specific hash iteration (using salt).
func (bf *BloomFilter) getHashIndex(key string, salt int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	_, _ = h.Write([]byte{byte(salt)}) // Добавляем соль для имитации разных хэш-функций / Add salt to emulate different hash functions
	return int(h.Sum32()) % bf.size
}

// ============================================================================
// КОМПОНЕНТ 2: СТРУКТУРЫ ХРАНИЛИЩА / COMPONENT 2: STORAGE STRUCTURES
// ============================================================================

// SSTableMetadata хранит путь к файлу на диске и его персональный Блум-фильтр в памяти.
// SSTableMetadata tracks the disk file path and its corresponding in-memory Bloom Filter.
type SSTableMetadata struct {
	path   string       // Путь к файлу SSTable на диске / Path to the SSTable file on disk
	filter *BloomFilter // Персональный Блум-фильтр этой таблицы в RAM / This table's personal Bloom Filter in RAM
}

// KeyValueNode представляет собой единую ноду хранилища с архитектурой LSM-Tree.
// KeyValueNode represents a single storage node utilizing the LSM-Tree architecture.
type KeyValueNode struct {
	mu         sync.RWMutex       // RWMutex для безопасной параллельной работы / RWMutex for safe concurrent operations
	dirPath    string             // Путь к директории с файлами БД / Path to the DB files directory
	memTable   map[string]string  // Хранилище в оперативной памяти / In-memory data storage (RAM)
	walFile    *os.File           // Файл журнала упреждающей записи (Write-Ahead Log) / Write-Ahead Log file handler
	maxMemSize int                // Максимальный размер MemTable перед сбросом на диск / Max MemTable size before flushing to disk
	sstables   []*SSTableMetadata // Список метаданных всех файлов SSTable / Metadata list of all SSTable files
}

// NewKeyValueNode инициализирует новую ноду хранилища и открывает WAL.
// NewKeyValueNode initializes a new storage node instance and opens the WAL.
func NewKeyValueNode(dirPath string, maxMemSize int) (*KeyValueNode, error) {
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return nil, err
	}

	node := &KeyValueNode{
		dirPath:    dirPath,
		memTable:   make(map[string]string),
		maxMemSize: maxMemSize,
		sstables:   make([]*SSTableMetadata, 0),
	}

	// Открываем или создаем Commit Log (WAL) в режиме Append-Only
	// Open or create the Commit Log (WAL) in Append-Only mode
	walPath := dirPath + "/commit.log"
	file, err := os.OpenFile(walPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	node.walFile = file

	return node, nil
}

// ============================================================================
// КОМПОНЕНТ 3: ЗАПИСЬ И СБРОС (PUT & FLUSH) / COMPONENT 3: PUT & FLUSH
// ============================================================================

// Put записывает данные сначала в WAL, затем в MemTable за O(1) времени.
// Put writes data first to the WAL, then to the MemTable in O(1) time complexity.
func (n *KeyValueNode) Put(key, value string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 1. Пишем в Commit Log (WAL) для защиты от сбоев. Формат: "key:value\n"
	// 1. Write to Commit Log (WAL) for durability. Format: "key:value\n"
	logEntry := fmt.Sprintf("%s:%s\n", key, value)
	if _, err := n.walFile.WriteString(logEntry); err != nil {
		return fmt.Errorf("failed to write to WAL: %w", err)
	}
	_ = n.walFile.Sync() // Принудительно сбрасываем буфер на диск / Flush disk buffers

	// 2. Пишем в MemTable (RAM)
	// 2. Write to MemTable (RAM)
	n.memTable[key] = value

	// 3. Если MemTable заполнилась, инициируем сброс на диск (Flush)
	// 3. If memory capacity is reached, trigger a disk flush (SSTable creation)
	if len(n.memTable) >= n.maxMemSize {
		if err := n.flushToSSTable(); err != nil {
			return fmt.Errorf("failed to flush MemTable: %w", err)
		}
	}

	return nil
}

// flushToSSTable сбрасывает текущую MemTable на диск в виде отсортированной SSTable.
// flushToSSTable dumps the current MemTable to disk as a sorted SSTable file.
func (n *KeyValueNode) flushToSSTable() error {
	// Собираем и сортируем ключи (SSTable требует строгой сортировки ключей)
	// Extract and sort keys (SSTable requires strict key ordering)
	keys := make([]string, 0, len(n.memTable))
	for k := range n.memTable {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Генерируем уникальное имя файла на базе наносекунд
	// Generate a unique file name based on nanoseconds
	sstablePath := fmt.Sprintf("%s/data_%d.db", n.dirPath, time.Now().UnixNano())
	file, err := os.OpenFile(sstablePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Инициализируем Блум-фильтр для новой SSTable (1000 бит, 3 хэш-функции)
	// Initialize a Bloom Filter for the new SSTable (1000 bits, 3 hash functions)
	filter := NewBloomFilter(1000, 3)

	// Переносим данные из памяти в файл
	// Write data from memory into the file
	for _, k := range keys {
		entry := fmt.Sprintf("%s:%s\n", k, n.memTable[k])
		if _, err := file.WriteString(entry); err != nil {
			return err
		}
		// Параллельно обучаем Блум-фильтр этому ключу
		// Populate the Bloom Filter with the key concurrently
		filter.Add(k)
	}

	// Добавляем метаданные новой SSTable В НАЧАЛО слайса (новые файлы сканируются первыми)
	// Prepend new SSTable metadata to the slice (newer files are scanned first)
	n.sstables = append([]*SSTableMetadata{{path: sstablePath, filter: filter}}, n.sstables...)

	// Очищаем MemTable в оперативной памяти
	// Reset MemTable in RAM
	n.memTable = make(map[string]string)

	// Очищаем (ротируем) WAL, так как данные уже надежно сохранены в SSTable
	// Truncate (rotate) the WAL, as data is now safely persisted inside the SSTable
	_ = n.walFile.Close()
	walPath := n.dirPath + "/commit.log"
	_ = os.Truncate(walPath, 0)
	newWal, err := os.OpenFile(walPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	n.walFile = newWal

	return nil
}

// ============================================================================
// КОМПОНЕНТ 4: ВЫСОКОНАГРУЖЕННОЕ ЧТЕНИЕ (GET) / COMPONENT 4: READS (GET)
// ============================================================================

// Get ищет значение по ключу, используя иерархию памяти и защиту Блум-фильтрами.
// Get locates a value by its key utilizing memory hierarchy and Bloom Filter protection layers.
func (n *KeyValueNode) Get(key string) (string, bool, error) {
	n.mu.RLock() // RLock позволяет множеству потоков читать данные параллельно / RLock permits concurrent reads
	defer n.mu.RUnlock()

	// 1. Ищем в MemTable (RAM) — самый быстрый путь
	// 1. Search inside MemTable (RAM) — the fastest pathway
	if val, exists := n.memTable[key]; exists {
		return val, true, nil
	}

	// 2. Ищем в файлах SSTable на диске (от самых новых к самым старым)
	// 2. Search through disk SSTable files (from newest to oldest)
	for _, sstable := range n.sstables {
		// ЗАЩИТА: Блум-фильтр мгновенно отсекает чтение диска, если ключа точно нет в файле
		// SHIELD: Bloom Filter instantly skips expensive disk I/O if the key is definitely absent
		if !sstable.filter.MaybeExists(key) {
			continue
		}

		// Если фильтр сказал "возможно есть", открываем файл и ищем ключ внутри
		// If the filter flags a potential match, open the file and scan for the key
		val, found, err := n.searchInFile(sstable.path, key)
		if err != nil {
			return "", false, err
		}
		if found {
			return val, true, nil // Возвращаем самую свежую версию / Return the latest version
		}
	}

	return "", false, nil
}

// searchInFile сканирует файл SSTable на диске на наличие ключа.
// searchInFile scans a specific SSTable file on disk for the target key.
func (n *KeyValueNode) searchInFile(path, key string) (string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && parts[0] == key {
			return parts[1], true, nil
		}
	}

	return "", false, scanner.Err()
}
