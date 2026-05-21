package twittersnowflakeid

import (
	"errors"
	"sync"
	"time"
)

const (
	epoch          int64 = 1716123456789 // Наша кастомная эпоха в мс / Our custom epoch base milestone in ms
	datacenterBits uint  = 5             // 5 бит под дата-центры (макс 32) / 5 bits for datacenters
	machineBits    uint  = 5             // 5 бит под сервера (макс 32) / 5 bits for physical machines
	sequenceBits   uint  = 12            // 12 бит под счетчик (макс 4096) / 12 bits for sequence counter

	maxDatacenterID int64 = -1 ^ (-1 << datacenterBits)
	maxMachineID    int64 = -1 ^ (-1 << machineBits)
	maxSequence     int64 = -1 ^ (-1 << sequenceBits)

	machineShift    uint = sequenceBits
	datacenterShift uint = sequenceBits + machineBits
	timestampShift  uint = sequenceBits + machineBits + datacenterBits
)

// SnowflakeNode представляет собой независимый боевой генератор ID от Twitter.
// SnowflakeNode represents an independent production-ready Twitter Snowflake ID generator.
type SnowflakeNode struct {
	mu            sync.Mutex
	lastTimestamp int64
	datacenterID  int64
	machineID     int64
	sequence      int64
}

func NewSnowflakeNode(datacenterID, machineID int64) (*SnowflakeNode, error) {
	if datacenterID < 0 || datacenterID > maxDatacenterID || machineID < 0 || machineID > maxMachineID {
		return nil, errors.New("error: infrastructure coordinates out of bounds")
	}

	return &SnowflakeNode{
		datacenterID:  datacenterID,
		machineID:     machineID,
		lastTimestamp: -1,
		sequence:      0,
	}, nil
}

// Generate собирает 64-битный ID с помощью побитовых сдвигов и логического ИЛИ (|).
// Generate packs the 64-bit ID using binary shifts and bitwise OR (|) gates.
func (n *SnowflakeNode) Generate() (uint64, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	now := time.Now().UnixNano() / int64(time.Millisecond)

	if now < n.lastTimestamp {
		return 0, errors.New("error: clock drift detected, refusing generation")
	}

	if now == n.lastTimestamp {
		n.sequence = (n.sequence + 1) & maxSequence
		if n.sequence == 0 {
			for now <= n.lastTimestamp {
				now = time.Now().UnixNano() / int64(time.Millisecond)
			}
		}
	} else {
		n.sequence = 0
	}

	n.lastTimestamp = now

	// Процессор соберет это число за 1 такт / The CPU compiles this integer layout within 1 clock cycle
	id := uint64(
		((now - epoch) << timestampShift) |
			(n.datacenterID << datacenterShift) |
			(n.machineID << machineShift) |
			n.sequence,
	)

	return id, nil
}
