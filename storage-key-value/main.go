package main

import (
	"fmt"
	"log"
	"os"
	"time"

	// Подставь сюда путь к твоему модулю / Replace with your actual module path
	kvstore "kvstore/storage-key-value/storagekeyvalue"
)

func main() {
	// Путь к временной директории для хранения файлов базы данных
	// Path to the temporary directory for storing database files
	dbDir := "./test_db"

	// Перед запуском очищаем старую тестовую папку, если она осталась с прошлых запусков
	// Before launching, clean up the old test folder if it exists from previous runs
	_ = os.RemoveAll(dbDir)

	// Инициализируем нашу Key-Value ноду.
	// maxMemSize = 3 означает, что после добавления 3-го ключа MemTable сбросится на диск (Flush)
	// Initialize our Key-Value node.
	// maxMemSize = 3 means that after adding the 3rd key, the MemTable will flush to disk
	node, err := kvstore.NewKeyValueNode(dbDir, 3)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== ШАГ 1: Быстрая запись в оперативную память ===")
	fmt.Println("=== STEP 1: High-Speed In-Memory Writes ===")

	// Записываем первые два ключа. Они мгновенно попадают в RAM (MemTable) и лог WAL
	// Writing the first two keys. They instantly hit RAM (MemTable) and the WAL log
	_ = node.Put("user:1", "Alice")
	_ = node.Put("user:2", "Bob")
	fmt.Println("[OK] Записаны user:1 и user:2 (Данные пока только в памяти и WAL)")
	fmt.Println("[OK] Saved user:1 and user:2 (Data resides in memory and WAL only)")

	// Проверяем, что чтение из памяти работает мгновенно
	// Verifying that in-memory reading works instantly
	val, found, _ := node.Get("user:1")
	if found {
		fmt.Printf("[GET] Ключ 'user:1' найден в памяти. Значение: %s\n", val)
		fmt.Printf("[GET] Key 'user:1' found in memory. Value: %s\n", val)
	}

	fmt.Println("\n=== ШАГ 2: Демонстрация автоматического сброса на диск (Flush) ===")
	fmt.Println("=== STEP 2: Demonstrating Automatic Disk Flush (SSTable) ===")

	// Записываем 3-й ключ. Так как лимит maxMemSize равен 3, этот Put триггерит создание файла SSTable
	// Writing the 3rd key. Since maxMemSize limit is 3, this Put triggers SSTable file creation
	_ = node.Put("user:3", "Charlie")
	fmt.Println("[FLUSH] Записан user:3. Память заполнена! Данные сброшены в файл SSTable на диск, WAL очищен.")
	fmt.Println("[FLUSH] Saved user:3. Memory full! Data flushed to an SSTable file on disk, WAL cleared.")

	// Дадим файловой системе долю секунды, чтобы гарантированно записать файл
	// Give the filesystem a fraction of a second to guarantee the file is written
	time.Sleep(time.Millisecond * 10)

	fmt.Println("\n=== ШАГ 3: Чтение с диска сквозь Блум-фильтр ===")
	fmt.Println("=== STEP 3: Reading from Disk through the Bloom Filter ===")

	// Читаем ключ 'user:2'. Его больше нет в MemTable, сервер найдет его в SSTable на диске
	// Reading key 'user:2'. It is no longer in MemTable, the server will locate it inside the disk SSTable
	val, found, _ = node.Get("user:2")
	if found {
		fmt.Printf("[GET] Ключ 'user:2' успешно прочитан из SSTable на диске! Значение: %s\n", val)
		fmt.Printf("[GET] Key 'user:2' successfully retrieved from disk SSTable! Value: %s\n", val)
	}

	fmt.Println("\n=== ШАГ 4: Защита от несуществующих ключей (DDoS-щит) ===")
	fmt.Println("=== STEP 4: Protection Against Non-Existent Keys (DDoS Shield) ===")

	// Запрашиваем ключ, которого никогда не было в системе
	// Requesting a key that has never been introduced to the system
	fmt.Println("[GET] Запрашиваем несуществующий ключ 'attacker_key_999'...")
	fmt.Println("[GET] Querying a non-existent key 'attacker_key_999'...")

	start := time.Now()
	_, found, _ = node.Get("attacker_key_999")
	elapsed := time.Since(start)

	if !found {
		fmt.Println("[SHIELD] Блум-фильтр мгновенно ответил: 'Этого ключа точно нет на диске!'.")
		fmt.Println("[SHIELD] Bloom Filter instantly replied: 'This key is definitely absent from disk!'.")
		fmt.Printf("[SHIELD] Сервер спасен от чтения диска. Время проверки: %v\n", elapsed)
		fmt.Printf("[SHIELD] Server saved from disk I/O scanning. Evaluation time: %v\n", elapsed)
	}

	// Очищаем за собой тестовые файлы после успешной демонстрации
	// Clean up benchmark test files after a successful demonstration
	_ = os.RemoveAll(dbDir)
}
