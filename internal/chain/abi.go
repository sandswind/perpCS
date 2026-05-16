// Package chain — ABI decoder for the SessionStarted event only.
//
// SessionStarted(
//     address indexed player,
//     bytes32 indexed levelId,
//     bytes32 indexed sessionId,
//     uint256 amount,
//     uint256 nonce,
//     uint256 blockNumber
// )
//
// Topic hash: keccak256("SessionStarted(address,bytes32,bytes32,uint256,uint256,uint256)")
//   = 0x73df37d8b93084e759a92f1c43076f44ef3eb6b3e0fb975767ae0413517bb30f
//
// Layout:
//   topics[0] = event signature
//   topics[1] = player (address, left-padded to 32 bytes)
//   topics[2] = levelId (bytes32)
//   topics[3] = sessionId (bytes32)
//   data      = abi.encode(amount, nonce, blockNumber) = 96 bytes (3 × 32)

package chain

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
)

// SessionStartedTopic is the keccak256 of the event signature. Computed via
// `cast keccak "SessionStarted(address,bytes32,bytes32,uint256,uint256,uint256)"`.
const SessionStartedTopic = "0x73df37d8b93084e759a92f1c43076f44ef3eb6b3e0fb975767ae0413517bb30f"

// SessionStartedEvent is the decoded payload of one SessionStarted log.
type SessionStartedEvent struct {
	Player      string   // 0x-prefixed lowercase address
	LevelID     [32]byte // raw bytes32
	SessionID   [32]byte // raw bytes32
	Amount      *big.Int // raw token units (wei, 18 decimals)
	Nonce       *big.Int // per-player monotonic counter
	BlockNumber *big.Int // contract-provided block.number at deposit time

	// Provenance (filled by indexer for downstream auditing).
	TxHash string // 0x-prefixed
	LogIdx uint64 // log index within the block
}

// LevelIDString returns a printable best-effort decode of LevelID (trims
// trailing NULs and falls back to hex if the result isn't ASCII).
func (e *SessionStartedEvent) LevelIDString() string {
	return decodeLevelID(e.LevelID)
}

// SessionIDHex returns the sessionId as 0x-hex (matches contract event arg).
func (e *SessionStartedEvent) SessionIDHex() string {
	return "0x" + hex.EncodeToString(e.SessionID[:])
}

// LevelIDHex returns the levelId as 0x-hex.
func (e *SessionStartedEvent) LevelIDHex() string {
	return "0x" + hex.EncodeToString(e.LevelID[:])
}

// DecodeSessionStarted parses one Log into a SessionStartedEvent.
// Returns an error if the log's topic[0] doesn't match or the data is
// malformed.
func DecodeSessionStarted(l Log) (*SessionStartedEvent, error) {
	if len(l.Topics) != 4 {
		return nil, fmt.Errorf("SessionStarted: expected 4 topics, got %d", len(l.Topics))
	}
	if !equalIgnoreCase(l.Topics[0], SessionStartedTopic) {
		return nil, fmt.Errorf("SessionStarted: topic mismatch (have %s)", l.Topics[0])
	}

	playerBytes, err := hexToBytes(l.Topics[1])
	if err != nil || len(playerBytes) != 32 {
		return nil, fmt.Errorf("SessionStarted: bad player topic: %v", err)
	}
	// Address is the right-most 20 bytes.
	player := "0x" + hex.EncodeToString(playerBytes[12:])

	levelBytes, err := hexToBytes(l.Topics[2])
	if err != nil || len(levelBytes) != 32 {
		return nil, fmt.Errorf("SessionStarted: bad levelId topic: %v", err)
	}
	sessionBytes, err := hexToBytes(l.Topics[3])
	if err != nil || len(sessionBytes) != 32 {
		return nil, fmt.Errorf("SessionStarted: bad sessionId topic: %v", err)
	}

	data, err := hexToBytes(l.Data)
	if err != nil {
		return nil, fmt.Errorf("SessionStarted: bad data hex: %w", err)
	}
	if len(data) != 96 {
		return nil, fmt.Errorf("SessionStarted: data must be 96 bytes, got %d", len(data))
	}

	logIdx, _ := l.LogIdx() // safe to ignore — already validated by caller in normal use

	ev := &SessionStartedEvent{
		Player:      player,
		Amount:      new(big.Int).SetBytes(data[0:32]),
		Nonce:       new(big.Int).SetBytes(data[32:64]),
		BlockNumber: new(big.Int).SetBytes(data[64:96]),
		TxHash:      l.TransactionHash,
		LogIdx:      logIdx,
	}
	copy(ev.LevelID[:], levelBytes)
	copy(ev.SessionID[:], sessionBytes)
	return ev, nil
}

// ErrTopicMismatch is returned when DecodeSessionStarted is called with a log
// whose first topic doesn't match the SessionStarted signature.
var ErrTopicMismatch = errors.New("topic mismatch")

func equalIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// decodeLevelID does a best-effort ASCII decode of the levelId bytes32.
// Many of our level IDs are short ASCII like "D-312-BTC" packed left-aligned
// — but in v0.5 the contract's caller uses keccak256(string), so the bytes
// are random and we should fall back to hex.
func decodeLevelID(b [32]byte) string {
	// Detect "looks like ASCII" vs random bytes: if all non-NUL bytes are in
	// printable ASCII range AND the trailing bytes are NUL, treat as string.
	end := 32
	for end > 0 && b[end-1] == 0 {
		end--
	}
	if end == 0 {
		return ""
	}
	for i := 0; i < end; i++ {
		if b[i] < 0x20 || b[i] > 0x7e {
			return "0x" + hex.EncodeToString(b[:])
		}
	}
	return string(b[:end])
}
