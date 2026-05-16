// Package session bridges the chain Indexer to the actor.
//
// Flow:
//
//	GameVault.SessionStarted (5 blocks deep)
//	   └─→ chain.Indexer
//	         └─→ session.Svc.HandleSessionStarted
//	               ├─ dedupe in-memory by sessionId
//	               ├─ persist a Record to disk (JSONL audit log)
//	               └─ enqueue actor.OpenSessionRequest on the actor's mailbox
//
// Concurrency:
//   - Svc is safe for concurrent use; chain.Indexer calls into it from a
//     single goroutine, but the HTTP layer reads sessionId → record from
//     the same instance.
//   - The actor remains the only writer to the accounts map; we hand it the
//     OpenSessionRequest via the SessionQueue channel.
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sandswind/perpCS/internal/actor"
	"github.com/sandswind/perpCS/internal/chain"
	"github.com/sandswind/perpCS/internal/types"
)

// Record is what we persist for each confirmed session and serve via the
// /sessions/{addr} endpoint.
type Record struct {
	SessionID    string `json:"session_id"`     // 0x-prefixed bytes32 hex (matches contract)
	Player       string `json:"player"`         // 0x-prefixed lowercase address
	LevelID      string `json:"level_id"`       // 0x-prefixed bytes32 hex
	LevelLabel   string `json:"level_label"`    // ASCII decode where possible
	AmountWei    string `json:"amount_wei"`     // raw token units, base 10
	AmountUSDR   string `json:"amount_usdr"`    // human-readable (e.g. "500")
	Nonce        uint64 `json:"nonce"`
	BlockNumber  uint64 `json:"block_number"`
	TxHash       string `json:"tx_hash"`
	LogIndex     uint64 `json:"log_index"`
	CreatedUnix  int64  `json:"created_unix"`   // wall-clock when the indexer dispatched (ms)
}

// Svc is the session service: dedup, persist, and forward to the actor.
type Svc struct {
	mu        sync.RWMutex
	bySession map[string]*Record // sessionId hex → record
	byPlayer  map[string]string  // player addr (lowercase) → sessionId hex (latest)

	queue       chan<- *actor.OpenSessionRequest
	auditPath   string // JSONL file
	auditFile   *os.File

	// MaxRetainedPerPlayer caps the per-player session list; 1 means
	// "the latest replaces the previous". v0.5 keeps the latest only.
	maxPerPlayer int

	logger *log.Logger
}

// Config configures a session.Svc.
type Config struct {
	// Queue is the actor's SessionQueue receiver — the Svc is the producer.
	Queue chan<- *actor.OpenSessionRequest
	// AuditPath is a JSONL file path; appended on every confirmed session.
	// Empty disables persistence.
	AuditPath string
	// Logger; nil → log.Default().
	Logger *log.Logger
}

// New constructs a Svc. The audit file is opened (and its directory created)
// up front so we fail fast on bad paths.
func New(cfg Config) (*Svc, error) {
	if cfg.Queue == nil {
		return nil, fmt.Errorf("session: Queue is required")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	s := &Svc{
		bySession:    make(map[string]*Record),
		byPlayer:     make(map[string]string),
		queue:        cfg.Queue,
		auditPath:    cfg.AuditPath,
		maxPerPlayer: 1,
		logger:       logger,
	}
	if cfg.AuditPath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.AuditPath), 0o755); err != nil {
			return nil, fmt.Errorf("session: mkdir: %w", err)
		}
		f, err := os.OpenFile(cfg.AuditPath, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o644)
		if err != nil {
			return nil, fmt.Errorf("session: open audit: %w", err)
		}
		s.auditFile = f
		// Replay existing records from disk so a restart restores the dedup
		// set without re-issuing OpenSession requests to the actor.
		if err := s.replay(); err != nil {
			return nil, fmt.Errorf("session: replay: %w", err)
		}
	}
	return s, nil
}

// Close flushes and closes the audit file. Idempotent.
func (s *Svc) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.auditFile != nil {
		err := s.auditFile.Close()
		s.auditFile = nil
		return err
	}
	return nil
}

// replay reloads existing records from the audit file into the in-memory
// dedup tables. Called once at startup.
func (s *Svc) replay() error {
	f, err := os.Open(s.auditPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	for {
		var r Record
		if err := dec.Decode(&r); err != nil {
			if err.Error() == "EOF" {
				break
			}
			s.logger.Printf("[session] audit decode error: %v (continuing)", err)
			break
		}
		s.bySession[r.SessionID] = &r
		s.byPlayer[strings.ToLower(r.Player)] = r.SessionID
	}
	s.logger.Printf("[session] replayed %d session(s) from audit", len(s.bySession))
	return nil
}

// HandleSessionStarted satisfies chain.Handler. It is invoked once per
// confirmed event by the Indexer goroutine.
func (s *Svc) HandleSessionStarted(ctx context.Context, ev *chain.SessionStartedEvent) error {
	rec := recordFromEvent(ev)

	// Dedupe (idempotent across indexer restarts).
	s.mu.Lock()
	if _, dup := s.bySession[rec.SessionID]; dup {
		s.mu.Unlock()
		s.logger.Printf("[session] dedup: ignoring duplicate %s", rec.SessionID)
		return nil
	}
	s.bySession[rec.SessionID] = rec
	s.byPlayer[strings.ToLower(rec.Player)] = rec.SessionID
	auditFile := s.auditFile
	s.mu.Unlock()

	// Persist to JSONL audit (non-fatal on error).
	if auditFile != nil {
		b, err := json.Marshal(rec)
		if err == nil {
			if _, werr := auditFile.Write(append(b, '\n')); werr != nil {
				s.logger.Printf("[session] audit write: %v", werr)
			}
		}
	}

	// Forward to the actor mailbox. Block on send so we never silently drop
	// a confirmed event; the actor must keep its queue capacity reasonable.
	balanceQty := weiToQty(ev.Amount)
	req := &actor.OpenSessionRequest{
		SessionID: rec.SessionID,
		Address:   rec.Player,
		Balance:   balanceQty,
		ResultCh:  make(chan actor.OpenSessionResult, 1),
	}
	select {
	case s.queue <- req:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case res := <-req.ResultCh:
		if res.Err != nil {
			return fmt.Errorf("actor openSession: %w", res.Err)
		}
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return fmt.Errorf("actor openSession: timeout")
	}

	s.logger.Printf("[session] opened sid=%s player=%s level=%s amount=%s USDR",
		rec.SessionID, rec.Player, rec.LevelLabel, rec.AmountUSDR)
	return nil
}

// LookupBySession returns the record for a session id (case-insensitive hex),
// or nil if unknown.
func (s *Svc) LookupBySession(sessionID string) *Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bySession[strings.ToLower(sessionID)]
}

// LookupByPlayer returns the most recent confirmed session for a player
// address (case-insensitive), or nil if none.
func (s *Svc) LookupByPlayer(player string) *Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sid, ok := s.byPlayer[strings.ToLower(player)]
	if !ok {
		return nil
	}
	return s.bySession[sid]
}

// Count returns how many records the Svc currently knows about.
func (s *Svc) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.bySession)
}

// ---- helpers ----

func recordFromEvent(ev *chain.SessionStartedEvent) *Record {
	return &Record{
		SessionID:   strings.ToLower(ev.SessionIDHex()),
		Player:      strings.ToLower(ev.Player),
		LevelID:     strings.ToLower(ev.LevelIDHex()),
		LevelLabel:  ev.LevelIDString(),
		AmountWei:   ev.Amount.String(),
		AmountUSDR:  weiToHumanString(ev.Amount),
		Nonce:       ev.Nonce.Uint64(),
		BlockNumber: ev.BlockNumber.Uint64(),
		TxHash:      ev.TxHash,
		LogIndex:    ev.LogIdx,
		CreatedUnix: time.Now().UnixMilli(),
	}
}

// weiToQty converts an 18-decimal token amount to the actor's 8-decimal Qty.
// We divide by 1e10 to bridge the two scales.
func weiToQty(wei *big.Int) types.Qty {
	if wei == nil {
		return 0
	}
	// 1e10 = 10_000_000_000
	div := new(big.Int).SetInt64(10_000_000_000)
	q := new(big.Int).Quo(wei, div)
	if !q.IsInt64() {
		return types.Qty(int64(^uint64(0) >> 1)) // saturate at MaxInt64
	}
	return types.Qty(q.Int64())
}

// weiToHumanString returns "500" for 500e18, "12.5" for 12.5e18.
func weiToHumanString(wei *big.Int) string {
	if wei == nil || wei.Sign() == 0 {
		return "0"
	}
	one := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	whole := new(big.Int).Quo(wei, one)
	frac := new(big.Int).Mod(wei, one)
	if frac.Sign() == 0 {
		return whole.String()
	}
	// Trim trailing zeros from the frac part.
	fracStr := fmt.Sprintf("%018d", frac)
	// strip trailing zeros
	end := len(fracStr)
	for end > 0 && fracStr[end-1] == '0' {
		end--
	}
	return whole.String() + "." + fracStr[:end]
}
