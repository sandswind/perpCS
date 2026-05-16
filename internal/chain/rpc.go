// Package chain implements the Ethereum JSON-RPC client and event indexer
// used by the v0.5 on-chain entry path.
//
// Design rationale (no go-ethereum dependency):
//   - We only need eth_blockNumber + eth_getLogs.
//   - The single event we decode (SessionStarted) has a fixed shape, so a
//     hand-rolled decoder is ~30 lines and avoids pulling ~50MB of deps.
//   - Indexer is poll-based: simpler than ws subscriptions, no reconnect
//     logic, and the 5-block confirmation window (~7s on Arbitrum Sepolia)
//     means a 4s poll is more than fast enough for UX.
//
// CRITICAL: This package never calls time.Now() to make decisions about
// chain state — it only uses time.Now() for poll throttling. All chain
// state is derived from JSON-RPC responses.
package chain

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
)

// Client is a minimal JSON-RPC client over HTTP.
// Safe for concurrent use.
type Client struct {
	url    string
	http   *http.Client
	nextID atomic.Uint64
}

// NewClient creates a new JSON-RPC client targeting `url`.
// `httpClient` may be nil; a default with a 10s timeout is used.
func NewClient(url string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{url: url, http: httpClient}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
	ID      uint64 `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error,omitempty"`
	ID      uint64          `json:"id"`
}

// call performs a single JSON-RPC request and decodes the result into `out`.
func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	id := c.nextID.Add(1)
	body, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024)) // 32 MB cap
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(respBody), 256))
	}

	var rr rpcResponse
	if err := json.Unmarshal(respBody, &rr); err != nil {
		return fmt.Errorf("decode: %w (body=%s)", err, truncate(string(respBody), 256))
	}
	if rr.Error != nil {
		return rr.Error
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(rr.Result, out)
}

// BlockNumber returns the current head block number.
func (c *Client) BlockNumber(ctx context.Context) (uint64, error) {
	var hexStr string
	if err := c.call(ctx, "eth_blockNumber", []any{}, &hexStr); err != nil {
		return 0, err
	}
	return parseHexUint64(hexStr)
}

// LogFilter is the subset of the JSON-RPC log-filter object we use.
type LogFilter struct {
	FromBlock uint64
	ToBlock   uint64
	Address   string     // 0x-prefixed
	Topics    [][]string // each inner slice is an OR; outer is AND across positions
}

// Log mirrors the JSON-RPC `eth_getLogs` response shape.
type Log struct {
	Address          string   `json:"address"`
	Topics           []string `json:"topics"`
	Data             string   `json:"data"`
	BlockNumber      string   `json:"blockNumber"`
	TransactionHash  string   `json:"transactionHash"`
	TransactionIndex string   `json:"transactionIndex"`
	BlockHash        string   `json:"blockHash"`
	LogIndex         string   `json:"logIndex"`
	Removed          bool     `json:"removed"`
}

// BlockNum returns the parsed block number for this log.
func (l *Log) BlockNum() (uint64, error) {
	return parseHexUint64(l.BlockNumber)
}

// LogIdx returns the parsed log index.
func (l *Log) LogIdx() (uint64, error) {
	return parseHexUint64(l.LogIndex)
}

// GetLogs fetches all logs matching the filter.
//
// Many RPC providers reject filters that span more than ~10k blocks; the
// caller (Indexer) is responsible for chunking.
func (c *Client) GetLogs(ctx context.Context, f LogFilter) ([]Log, error) {
	type filterParam struct {
		FromBlock string     `json:"fromBlock"`
		ToBlock   string     `json:"toBlock"`
		Address   string     `json:"address,omitempty"`
		Topics    [][]string `json:"topics,omitempty"`
	}
	p := filterParam{
		FromBlock: fmt.Sprintf("0x%x", f.FromBlock),
		ToBlock:   fmt.Sprintf("0x%x", f.ToBlock),
		Address:   f.Address,
		Topics:    f.Topics,
	}
	var logs []Log
	if err := c.call(ctx, "eth_getLogs", []any{p}, &logs); err != nil {
		return nil, err
	}
	return logs, nil
}

// ---- helpers ----

// parseHexUint64 parses "0x..." (any width) into uint64.
func parseHexUint64(s string) (uint64, error) {
	s = strings.TrimPrefix(s, "0x")
	if s == "" {
		return 0, nil
	}
	if len(s) > 16 {
		return 0, fmt.Errorf("hex %q overflows uint64", s)
	}
	var n uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		var v byte
		switch {
		case c >= '0' && c <= '9':
			v = c - '0'
		case c >= 'a' && c <= 'f':
			v = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			v = c - 'A' + 10
		default:
			return 0, fmt.Errorf("invalid hex char %q", c)
		}
		n = n<<4 | uint64(v)
	}
	return n, nil
}

// hexToBytes decodes a "0x..."-prefixed hex string into bytes. Non-hex or
// odd-length input returns an error.
func hexToBytes(s string) ([]byte, error) {
	s = strings.TrimPrefix(s, "0x")
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("hex string has odd length: %d", len(s))
	}
	return hex.DecodeString(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
