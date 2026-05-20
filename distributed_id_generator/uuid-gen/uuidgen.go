package uuidgen

import (
	"crypto/rand"
	"fmt"
)

// GenerateUUIDv4 генерирует случайный 128-битный UUID согласно стандарту RFC 4122.
// GenerateUUIDv4 provisions a pseudorandom 128-bit UUID compliant with RFC 4122.
func GenerateUUIDv4() (string, error) {
	uuid := make([]byte, 16)
	// Используем криптографически безопасный генератор случайных чисел
	// Utilize a cryptographically secure pseudorandom number generator
	if _, err := rand.Read(uuid); err != nil {
		return "", err
	}

	// Установка обязательных битов версии (4) и варианта (RFC 4122)
	// Set mandatory bits for version (4) and variant (RFC 4122)
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Версия 4 / Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Вариант RFC 4122 / Variant RFC 4122

	// Форматируем в каноническую строку 8-4-4-4-12
	// Format into the canonical 8-4-4-4-12 string layout
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}
