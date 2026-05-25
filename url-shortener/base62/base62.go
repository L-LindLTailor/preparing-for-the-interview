package base62

import "strings"

// Алфавит из 62 символов для кодирования URL (фиксированный порядок важен для детерминизма)
// Base62 conversion dictionary array alphabet (fixed sequencing guarantees determinism)
const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Encode переводит число uint64 в компактную строку Base62 без лишних аллокаций памяти.
// Encode transforms a uint64 integer into a dense Base62 token string optimizing allocation overhead.
func Encode(num uint64) string {
	// Крайний случай: если ID равен нулю, возвращаем первый символ алфавита
	// Edge case boundary: if the ID is zero, return the initial alphabet index character
	if num == 0 {
		return string(alphabet[0])
	}

	// Использование strings.Builder предотвращает фрагментацию памяти в куче (Heap Allocation)
	// Utilizing strings.Builder blocks excessive heap allocations and memory fragmentation
	var sb strings.Builder
	for num > 0 {
		rem := num % 62
		sb.WriteByte(alphabet[rem]) // Добавляем байт символа в буфер / Append character byte to the buffer
		num = num / 62
	}

	// Разворачиваем срез байт, так как остатки от деления собирались с конца
	// Reverse the underlying byte slice as mathematical remainders accumulated backward
	bytes := []byte(sb.String())
	for i, j := 0, len(bytes)-1; i < j; i, j = i+1, j-1 {
		bytes[i], bytes[j] = bytes[j], bytes[i]
	}
	return string(bytes)
}

// Decode переводит строку Base62 обратно в оригинальное числовое значение uint64.
// Decode translates a Base62 token string layout back into its source uint64 numerical metrics.
func Decode(str string) uint64 {
	var result uint64
	for i := 0; i < len(str); i++ {
		// Находим позицию текущего символа в алфавите, восстанавливая разряд числа
		// Locate the active character offset inside the alphabet to restore the numerical radix
		idx := strings.IndexByte(alphabet, str[i])
		result = result*62 + uint64(idx)
	}
	return result
}
