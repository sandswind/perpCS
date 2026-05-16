package actor

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/sandswind/perpCS/internal/types"
)

// MemorySink collects all events in memory. Used for testing and small replays.
type MemorySink struct {
	mu     sync.Mutex
	Events []types.Event
}

func (s *MemorySink) Emit(e types.Event) error {
	s.mu.Lock()
	s.Events = append(s.Events, e)
	s.mu.Unlock()
	return nil
}

// Hash returns the SHA-256 of the canonically serialised event log.
// Two identical runs MUST produce the same hash — this is the determinism test.
func (s *MemorySink) Hash() ([]byte, error) {
	h := sha256.New()
	for _, e := range s.Events {
		b, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		h.Write(b)
		h.Write([]byte{'\n'})
	}
	return h.Sum(nil), nil
}

// JSONLSink writes events as newline-delimited JSON to a file.
// It also maintains a running SHA-256 hash for integrity checking.
type JSONLSink struct {
	mu    sync.Mutex
	f     *os.File
	w     *bufio.Writer
	h     io.Writer // sha256.Hash implements io.Writer
	hsum  func() []byte
	path  string
	count int64
}

// NewJSONLSink creates (or truncates) the file at path and returns a sink.
func NewJSONLSink(path string) (*JSONLSink, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create sink dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create events file: %w", err)
	}
	h := sha256.New()
	return &JSONLSink{
		f:    f,
		w:    bufio.NewWriterSize(f, 64*1024),
		h:    h,
		hsum: func() []byte { return h.Sum(nil) },
		path: path,
	}, nil
}

func (s *JSONLSink) Emit(e types.Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.w.Write(b); err != nil {
		return err
	}
	if err := s.w.WriteByte('\n'); err != nil {
		return err
	}
	s.h.Write(b)
	s.h.Write([]byte{'\n'})
	s.count++
	return nil
}

// Close flushes and closes the underlying file.
func (s *JSONLSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.w.Flush(); err != nil {
		return err
	}
	return s.f.Close()
}

// Hash returns the SHA-256 of all emitted events.
func (s *JSONLSink) Hash() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hsum()
}

// Count returns the number of events emitted.
func (s *JSONLSink) Count() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

// Path returns the file path.
func (s *JSONLSink) Path() string { return s.path }

// TeeSink duplicates events to two sinks (e.g. memory + file).
type TeeSink struct {
	A, B EventSink
}

func (t *TeeSink) Emit(e types.Event) error {
	if err := t.A.Emit(e); err != nil {
		return fmt.Errorf("tee A: %w", err)
	}
	return t.B.Emit(e)
}

// NullSink discards all events (useful for benchmarks).
type NullSink struct{ Count int64 }

func (n *NullSink) Emit(_ types.Event) error {
	n.Count++
	return nil
}

// ReadJSONL reads a JSONL events file and returns all events.
// Used for offline replay and determinism verification.
func ReadJSONL(path string) ([]types.Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return DecodeJSONL(f)
}

// DecodeJSONL decodes all events from a reader.
func DecodeJSONL(r io.Reader) ([]types.Event, error) {
	var events []types.Event
	dec := json.NewDecoder(r)
	for {
		var e types.Event
		if err := dec.Decode(&e); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("decode event: %w", err)
		}
		events = append(events, e)
	}
	return events, nil
}
