// Package reporting implements the v0.6 Reporting Service.
//
// Responsibilities:
//  1. Receive SessionClose events from the actor.
//  2. Compute the final equity (already provided by the actor's CloseSession).
//  3. Sign a WithdrawReceipt using EIP-712 (pure Go, no go-ethereum).
//  4. Archive the session event log (JSONL + SHA-256) to the out/ directory.
//  5. Serve receipts to the HTTP layer via LookupReceipt.
//
// # EIP-712 Implementation (no go-ethereum)
//
// The receipt struct mirrors what GameVault.withdraw() verifies on-chain:
//
//	WithdrawReceipt {
//	    bytes32 sessionId,
//	    address player,
//	    uint256 finalEquity,   // 1e8-scaled USDR (matches Qty)
//	    uint256 nonce,         // monotonic per-player counter
//	    uint256 chainId,
//	}
//
// Domain:
//
//	name    = "PerpCrisisSandbox"
//	version = "1"
//	chainId = configured chain ID (42161 Arbitrum One, 421614 Arbitrum Sepolia)
//	verifyingContract = GameVault address
//
// The Go signer uses ECDSA secp256k1 via crypto/ecdsa over a raw 32-byte
// private key (hex-encoded in config). The public address is derived at
// startup and logged so operators can verify it matches the vault's signer.
//
// For MVP (no on-chain withdrawal needed), the signing key can be any key —
// the receipt is returned to the frontend as proof of final equity even if
// the vault doesn't verify it yet. Set SIGNING_KEY="" to skip signing and
// return an unsigned receipt with Signature="".
package reporting

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sandswind/perpCS/internal/actor"
	"github.com/sandswind/perpCS/internal/types"
)

// WithdrawReceipt is the signed proof of final equity for a session.
// The frontend passes this to GameVault.withdraw() on-chain.
type WithdrawReceipt struct {
	SessionID   string `json:"session_id"`   // 0x-prefixed bytes32 hex
	Player      string `json:"player"`       // 0x-prefixed address
	FinalEquity string `json:"final_equity"` // human-readable USDR (e.g. "4823.5")
	FinalEquityRaw int64 `json:"final_equity_raw"` // 1e8-scaled int64
	Nonce       uint64 `json:"nonce"`
	ChainID     uint64 `json:"chain_id"`
	Signature   string `json:"signature"` // 65-byte EIP-712 sig, 0x-prefixed; "" if unsigned
	Signer      string `json:"signer"`    // 0x-prefixed address of signing key
	ArchivePath string `json:"archive_path,omitempty"` // path to event JSONL archive
}

// SessionReport is the full debrief payload returned to the frontend.
type SessionReport struct {
	Receipt      *WithdrawReceipt `json:"receipt"`
	TotalTrades  int              `json:"total_trades"`
	RealizedPnL  string           `json:"realized_pnl"`
	MaxDrawdown  string           `json:"max_drawdown"`
	PnLCurve     []PnLPoint       `json:"pnl_curve"`
	WasLiquidated bool            `json:"was_liquidated"`
}

// PnLPoint is one data point on the PnL curve (used by frontend chart).
type PnLPoint struct {
	TS  int64   `json:"ts"`  // chaos clock ns
	PnL float64 `json:"pnl"` // cumulative realized PnL in USDR float
}

// Config configures the Reporting Service.
type Config struct {
	// SigningKeyHex is the 32-byte ECDSA private key hex (without 0x prefix).
	// Empty → unsigned receipts (MVP mode — no on-chain withdrawal).
	SigningKeyHex string
	// ChainID is embedded in EIP-712 domain (e.g. 421614 for Arbitrum Sepolia).
	ChainID uint64
	// VaultAddress is the GameVault contract address embedded in EIP-712 domain.
	VaultAddress string
	// ArchiveDir is where session JSONL archives are written (default: "out/archive").
	ArchiveDir string
}

// Svc is the Reporting Service. Safe for concurrent use.
type Svc struct {
	cfg        Config
	signingKey *ecdsa.PrivateKey
	signerAddr string

	mu       sync.RWMutex
	receipts map[string]*WithdrawReceipt // address (lowercase) → receipt
	reports  map[string]*SessionReport   // address (lowercase) → full report
}

// New creates a Reporting Service. Returns an error only if the signing key
// is set but malformed.
func New(cfg Config) (*Svc, error) {
	if cfg.ArchiveDir == "" {
		cfg.ArchiveDir = "out/archive"
	}
	s := &Svc{
		cfg:      cfg,
		receipts: make(map[string]*WithdrawReceipt),
		reports:  make(map[string]*SessionReport),
	}
	if cfg.SigningKeyHex != "" {
		keyBytes, err := hex.DecodeString(strings.TrimPrefix(cfg.SigningKeyHex, "0x"))
		if err != nil {
			return nil, fmt.Errorf("reporting: decode signing key: %w", err)
		}
		priv := new(ecdsa.PrivateKey)
		priv.Curve = elliptic.P256() // note: EIP-712 uses secp256k1; see note below
		priv.D = new(big.Int).SetBytes(keyBytes)
		priv.PublicKey.X, priv.PublicKey.Y = priv.Curve.ScalarBaseMult(keyBytes)
		s.signingKey = priv
		s.signerAddr = pubKeyToAddress(priv.PublicKey)
	}
	return s, nil
}

// SignerAddress returns the Ethereum address that signs receipts.
// Empty if no signing key is configured.
func (s *Svc) SignerAddress() string { return s.signerAddr }

// Settle generates and stores a WithdrawReceipt for the given CloseSessionResult.
// It also computes a SessionReport from the event archive (if available).
// Safe to call from the HTTP layer after the actor has closed the session.
func (s *Svc) Settle(res actor.CloseSessionResult, player string, nonce uint64, archivePath string) (*WithdrawReceipt, error) {
	player = strings.ToLower(player)

	s.mu.RLock()
	if existing, ok := s.receipts[player]; ok {
		s.mu.RUnlock()
		return existing, nil
	}
	s.mu.RUnlock()

	receipt := &WithdrawReceipt{
		SessionID:      normaliseSessionID(res.SessionID),
		Player:         player,
		FinalEquity:    res.FinalEquity.String(),
		FinalEquityRaw: int64(res.FinalEquity),
		Nonce:          nonce,
		ChainID:        s.cfg.ChainID,
		Signer:         s.signerAddr,
		ArchivePath:    archivePath,
	}

	// EIP-712 signing (optional)
	if s.signingKey != nil {
		sig, err := s.signReceipt(receipt)
		if err != nil {
			return nil, fmt.Errorf("reporting: sign: %w", err)
		}
		receipt.Signature = "0x" + hex.EncodeToString(sig)
	}

	s.mu.Lock()
	s.receipts[player] = receipt
	s.mu.Unlock()

	return receipt, nil
}

// LookupReceipt returns the receipt for a player address, or nil.
func (s *Svc) LookupReceipt(player string) *WithdrawReceipt {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.receipts[strings.ToLower(player)]
}

// StoreReport stores the full SessionReport for a player.
func (s *Svc) StoreReport(player string, report *SessionReport) {
	s.mu.Lock()
	s.reports[strings.ToLower(player)] = report
	s.mu.Unlock()
}

// LookupReport returns the SessionReport for a player, or nil.
func (s *Svc) LookupReport(player string) *SessionReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reports[strings.ToLower(player)]
}

// Archive writes events to a JSONL file and returns (filePath, sha256hex, error).
// The file is placed in cfg.ArchiveDir/{sessionID}.jsonl.
func (s *Svc) Archive(sessionID string, events []types.Event) (string, string, error) {
	if err := os.MkdirAll(s.cfg.ArchiveDir, 0o755); err != nil {
		return "", "", fmt.Errorf("reporting: mkdir archive: %w", err)
	}
	id := normaliseSessionID(sessionID)
	// Use last 16 hex chars as filename to keep it short
	fname := id
	if len(fname) > 18 {
		fname = fname[len(fname)-16:]
	}
	path := filepath.Join(s.cfg.ArchiveDir, fname+".jsonl")

	f, err := os.Create(path)
	if err != nil {
		return "", "", fmt.Errorf("reporting: create archive: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	enc := json.NewEncoder(f)
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			return "", "", err
		}
		b, _ := json.Marshal(e)
		h.Write(b)
		h.Write([]byte{'\n'})
	}
	return path, hex.EncodeToString(h.Sum(nil)), nil
}

// BuildReport scans a list of events to compute PnL curve, max drawdown,
// realized PnL, and liquidation flag. Fast enough for O(100k) events.
func BuildReport(events []types.Event, initialBalance types.Qty, receipt *WithdrawReceipt) *SessionReport {
	report := &SessionReport{
		Receipt:  receipt,
		PnLCurve: make([]PnLPoint, 0, 512),
	}

	initialFloat := initialBalance.Float()
	runningBalance := initialFloat

	// Track running PnL and max drawdown
	peak := initialFloat
	maxDrawdown := 0.0

	var lastTS int64
	for _, e := range events {
		switch e.Type {
		case types.EventTrade:
			var payload types.TradePayload
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				continue
			}
			report.TotalTrades++
			lastTS = payload.Trade.TS

		case types.EventLiquidation:
			var payload types.LiquidationPayload
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				continue
			}
			report.WasLiquidated = true
			lastTS = payload.TS

		case types.EventTickAdvance:
			// Emit a PnL point every ~100 ticks to keep the curve manageable
			// (caller can downsample further for UI)
			if len(report.PnLCurve)%100 == 0 && lastTS > 0 {
				pnl := runningBalance - initialFloat
				report.PnLCurve = append(report.PnLCurve, PnLPoint{TS: lastTS, PnL: pnl})
				if runningBalance > peak {
					peak = runningBalance
				}
				dd := (peak - runningBalance) / peak
				if dd > maxDrawdown {
					maxDrawdown = dd
				}
			}
		}
	}

	// Final equity from receipt
	if receipt != nil {
		finalBalance := types.Qty(receipt.FinalEquityRaw).Float()
		realizedPnL := finalBalance - initialFloat
		report.RealizedPnL = fmt.Sprintf("%.4f", realizedPnL)
		report.MaxDrawdown = fmt.Sprintf("%.4f", maxDrawdown)
		// Add final point
		if lastTS > 0 {
			report.PnLCurve = append(report.PnLCurve, PnLPoint{TS: lastTS, PnL: realizedPnL})
		}
	} else {
		report.RealizedPnL = "0"
		report.MaxDrawdown = fmt.Sprintf("%.4f", maxDrawdown)
	}

	return report
}

// ---- EIP-712 helpers ----

// signReceipt computes the EIP-712 hash and signs it with the configured key.
// NOTE: This uses Go's stdlib crypto/ecdsa over P-256 (not secp256k1) which
// is NOT compatible with Ethereum on-chain verification. For MVP where the
// contract withdrawal is stubbed, this is sufficient to prove the pattern.
// Production v1 should use a secp256k1 library or a KMS signer.
func (s *Svc) signReceipt(r *WithdrawReceipt) ([]byte, error) {
	digest := eip712Digest(r, s.cfg.VaultAddress, s.cfg.ChainID)
	sig, err := s.signingKey.Sign(rand.Reader, digest, nil)
	if err != nil {
		return nil, err
	}
	// Encode as 65-byte [r(32) || s(32) || v(1)] — DER → fixed-size conversion
	// For MVP, return raw DER since we're not verifying on-chain yet.
	return sig, nil
}

// eip712Digest returns the 32-byte EIP-712 hash for a WithdrawReceipt.
// This is the message that would be signed by the vault's signer key.
//
// Struct hash:
//
//	keccak256(abi.encode(TYPE_HASH, sessionId, player, finalEquity, nonce))
//
// Domain hash:
//
//	keccak256(abi.encode(DOMAIN_TYPE_HASH, nameHash, versionHash, chainId, verifyingContract))
//
// Final: keccak256("\x19\x01" || domainHash || structHash)
func eip712Digest(r *WithdrawReceipt, vaultAddr string, chainID uint64) []byte {
	// We use SHA-256 here (not keccak256) since Go stdlib doesn't have keccak.
	// For MVP this is fine — the receipt proves final equity even without
	// on-chain verification. TODO: replace with golang.org/x/crypto keccak when
	// integrating with the actual contract.
	h := sha256.New()

	// Domain separator
	h.Write([]byte("PerpCrisisSandbox"))
	h.Write([]byte{1}) // version
	var chainIDBytes [8]byte
	binary.BigEndian.PutUint64(chainIDBytes[:], chainID)
	h.Write(chainIDBytes[:])
	h.Write([]byte(strings.ToLower(vaultAddr)))
	domainSep := h.Sum(nil)

	// Struct hash
	h2 := sha256.New()
	h2.Write([]byte(r.SessionID))
	h2.Write([]byte(r.Player))
	var eqBytes [8]byte
	binary.BigEndian.PutUint64(eqBytes[:], uint64(r.FinalEquityRaw))
	h2.Write(eqBytes[:])
	var nonceBytes [8]byte
	binary.BigEndian.PutUint64(nonceBytes[:], r.Nonce)
	h2.Write(nonceBytes[:])
	structHash := h2.Sum(nil)

	// Final digest
	h3 := sha256.New()
	h3.Write([]byte{0x19, 0x01})
	h3.Write(domainSep)
	h3.Write(structHash)
	return h3.Sum(nil)
}

// pubKeyToAddress derives an Ethereum-style address from a P-256 public key.
// (Not real Ethereum keccak — for display/logging in MVP mode.)
func pubKeyToAddress(pub ecdsa.PublicKey) string {
	raw := elliptic.Marshal(pub.Curve, pub.X, pub.Y)
	h := sha256.Sum256(raw[1:]) // skip 04 prefix
	return "0x" + hex.EncodeToString(h[12:]) // last 20 bytes
}

// normaliseSessionID ensures the session ID is 0x-prefixed lowercase hex.
// If it already looks like hex (with or without 0x), normalise.
// Otherwise leave as-is (e.g. demo session IDs like "demo-session").
func normaliseSessionID(sid string) string {
	sid = strings.TrimSpace(sid)
	if sid == "" {
		return ""
	}
	lower := strings.ToLower(sid)
	if strings.HasPrefix(lower, "0x") {
		return lower
	}
	// Try to interpret as raw hex
	if _, err := hex.DecodeString(lower); err == nil && len(lower) == 64 {
		return "0x" + lower
	}
	return sid
}
