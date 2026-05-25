package base62

import "strings"

// Алфавит из 62 символов для кодирования URL
const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Encode переводит число uint64 в строку Base62 (RU / EN)
func Encode(num uint64) string {
	if num == 0 {
		return string(alphabet[0])
	}

	var sb strings.Builder
	for num > 0 {
		rem := num % 62
		sb.WriteByte(alphabet[rem])
		num = num / 62
	}

	// Разворачиваем строку, так как остатки собирались с конца
	bytes := []byte(sb.String())
	for i, j := 0, len(bytes)-1; i < j; i, j = i+1, j-1 {
		bytes[i], bytes[j] = bytes[j], bytes[i]
	}
	return string(bytes)
}

// Decode переводит строку Base62 обратно в число uint64
func Decode(str string) uint64 {
	var result uint64
	for i := 0; i < len(str); i++ {
		idx := strings.IndexByte(alphabet, str[i])
		result = result*62 + uint64(idx)
	}
	return result
}
