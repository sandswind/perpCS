// Package chain — Indexer reads SessionStarted logs from GameVault and
// dispatches them to a Handler once they are buried 5 blocks deep.
//
// Reorg model:
//   - Arbitrum Sepolia / Sepolia have shallow reorgs (~1-2 blocks under
//     normal conditions). Confirming at depth=5 makes "session created on
//     a reorged block" effectively impossible.
//   - The indexer NEVER processes blocks newer than `head - confirmations`,
//     so a reorg that doesn't go that deep is invisible to us.
//   - If a reorg DOES go deeper than `confirmations`, we still won't replay
//     events because each event is keyed by (txHash, logIndex) and the
//     Handler dedupes via the SessionSvc; in the worst case the user sees
//     a "session pending" timeout on the FE.
//
// State persistence:
//   - A single JSON file (`out/indexer-state.json` by default) stores the
//     last fully-processed block number. This survives restarts.
//   - The file is written atomically (write tmp + rename).

package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// DefaultConfirmations is the safety depth the indexer waits before
// dispatching a log. Tuned for Arbitrum Sepolia.
const DefaultConfirmations uint64 = 5

// DefaultPollInterval is how often the indexer polls eth_blockNumber.
// At ~250ms block time on Arbitrum, 4s = ~16 blocks of latency.
const DefaultPollInterval = 4 * time.Second

// DefaultMaxRange is the largest [from, to] window in a single eth_getLogs.
// Most public RPCs cap at 10k; we stay well under that.
const DefaultMaxRange uint64 = 2000

// Handler is the callback invoked for each confirmed SessionStarted event.
// It runs on the Indexer's goroutine; the implementation MUST be quick or
// dispatch async (e.g. push onto a channel).
type Handler interface {
	HandleSessionStarted(ctx context.Context, ev *SessionStartedEvent) error
}

// HandlerFunc is an adapter so plain functions satisfy Handler.
type HandlerFunc func(context.Context, *SessionStartedEvent) error

func (f HandlerFunc) HandleSessionStarted(ctx context.Context, ev *SessionStartedEvent) error {
	return f(ctx, ev)
}

// Config configures an Indexer.
type Config struct {
	Client          *Client
	VaultAddress    string // 0x-prefixed GameVault contract address
	StartBlock      uint64 // initial fromBlock if no state file is present
	StatePath       string // path to JSON state file (e.g. out/indexer-state.json)
	Confirmations   uint64 // 0 → DefaultConfirmations
	PollInterval    time.Duration
	MaxRange        uint64
	Handler         Handler
	Logger          *log.Logger // nil → log.Default()
}

// Indexer polls for SessionStarted logs and dispatches confirmed ones.
// One Indexer per chain. Not safe for concurrent use; call Run once.
type Indexer struct {
	cfg     Config
	state   indexerState
	logger  *log.Logger
}

type indexerState struct {
	LastProcessed uint64 `json:"last_processed"`
	Updated       int64  `json:"updated_unix_ms"`
}

// New constructs an Indexer with sane defaults applied.
func New(cfg Config) (*Indexer, error) {
	if cfg.Client == nil {
		return nil, errors.New("chain.Indexer: Client is required")
	}
	if cfg.VaultAddress == "" {
		return nil, errors.New("chain.Indexer: VaultAddress is required")
	}
	if cfg.Handler == nil {
		return nil, errors.New("chain.Indexer: Handler is required")
	}
	if cfg.StatePath == "" {
		cfg.StatePath = "out/indexer-state.json"
	}
	if cfg.Confirmations == 0 {
		cfg.Confirmations = DefaultConfirmations
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = DefaultPollInterval
	}
	if cfg.MaxRange == 0 {
		cfg.MaxRange = DefaultMaxRange
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}

	idx := &Indexer{cfg: cfg, logger: cfg.Logger}
	if err := idx.loadState(); err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	return idx, nil
}

// loadState reads the persisted last-processed block, falling back to
// cfg.StartBlock if the file does not exist.
func (i *Indexer) loadState() error {
	b, err := os.ReadFile(i.cfg.StatePath)
	if errors.Is(err, os.ErrNotExist) {
		// First run — initialise from configured StartBlock.
		// We treat StartBlock as "first block we have NOT yet processed",
		// so LastProcessed = StartBlock - 1 (saturating at 0).
		var lp uint64
		if i.cfg.StartBlock > 0 {
			lp = i.cfg.StartBlock - 1
		}
		i.state = indexerState{LastProcessed: lp}
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &i.state)
}

// saveState atomically persists the current state to disk.
func (i *Indexer) saveState() error {
	if err := os.MkdirAll(filepath.Dir(i.cfg.StatePath), 0o755); err != nil {
		return err
	}
	i.state.Updated = time.Now().UnixMilli()
	b, err := json.MarshalIndent(&i.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := i.cfg.StatePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, i.cfg.StatePath)
}

// LastProcessed returns the highest block number whose logs have been
// dispatched to the Handler.
func (i *Indexer) LastProcessed() uint64 { return i.state.LastProcessed }

// Run blocks the calling goroutine and polls until ctx is done.
// It is safe to call once per Indexer instance.
func (i *Indexer) Run(ctx context.Context) error {
	i.logger.Printf("[indexer] starting; vault=%s startBlock=%d confirmations=%d poll=%s",
		i.cfg.VaultAddress, i.state.LastProcessed+1, i.cfg.Confirmations, i.cfg.PollInterval)

	t := time.NewTicker(i.cfg.PollInterval)
	defer t.Stop()

	// Tick once immediately so we don't wait `PollInterval` on startup.
	if err := i.tick(ctx); err != nil {
		i.logger.Printf("[indexer] initial tick error: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			i.logger.Printf("[indexer] shutting down (last_processed=%d)", i.state.LastProcessed)
			return ctx.Err()
		case <-t.C:
			if err := i.tick(ctx); err != nil {
				// Log and continue — transient RPC errors are normal.
				i.logger.Printf("[indexer] tick error: %v", err)
			}
		}
	}
}

// tick performs one polling cycle: fetches head, walks the unprocessed
// (and confirmed) range, dispatches logs, and persists state.
func (i *Indexer) tick(ctx context.Context) error {
	head, err := i.cfg.Client.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("eth_blockNumber: %w", err)
	}

	// Compute the highest block we may safely process.
	if head < i.cfg.Confirmations {
		return nil // chain too young
	}
	confirmed := head - i.cfg.Confirmations

	if confirmed <= i.state.LastProcessed {
		return nil // nothing new
	}

	from := i.state.LastProcessed + 1
	to := confirmed

	// Walk in chunks of MaxRange.
	for from <= to {
		end := from + i.cfg.MaxRange - 1
		if end > to {
			end = to
		}
		if err := i.scan(ctx, from, end); err != nil {
			return err
		}
		i.state.LastProcessed = end
		if err := i.saveState(); err != nil {
			i.logger.Printf("[indexer] save state: %v", err)
		}
		from = end + 1
	}
	return nil
}

// scan fetches logs in [from, to] and dispatches them in order.
func (i *Indexer) scan(ctx context.Context, from, to uint64) error {
	logs, err := i.cfg.Client.GetLogs(ctx, LogFilter{
		FromBlock: from,
		ToBlock:   to,
		Address:   i.cfg.VaultAddress,
		Topics:    [][]string{{SessionStartedTopic}},
	})
	if err != nil {
		return fmt.Errorf("eth_getLogs [%d,%d]: %w", from, to, err)
	}
	if len(logs) == 0 {
		return nil
	}
	i.logger.Printf("[indexer] %d SessionStarted log(s) in [%d,%d]", len(logs), from, to)

	for j := range logs {
		l := logs[j]
		if l.Removed {
			i.logger.Printf("[indexer] skipping removed log %s/%s", l.TransactionHash, l.LogIndex)
			continue
		}
		ev, err := DecodeSessionStarted(l)
		if err != nil {
			i.logger.Printf("[indexer] decode error tx=%s: %v", l.TransactionHash, err)
			continue
		}
		if err := i.cfg.Handler.HandleSessionStarted(ctx, ev); err != nil {
			// Handler errors are fatal for this batch — we surface them so
			// the operator can investigate. State is NOT advanced on error.
			return fmt.Errorf("handler: %w", err)
		}
	}
	return nil
}
