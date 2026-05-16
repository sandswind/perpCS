package chain

import (
	"math/big"
	"testing"
)

// Reference vector produced by `forge test` running deposit() on local anvil
// and capturing the emitted SessionStarted log via cast.  See
// contracts/test/GameVault.t.sol::test_DepositSucceeds_EmitsSessionStarted.
//
// player    = 0x328809Bc894f92807417D2dAD6b7C998c1aFdac6 (alice via makeAddr)
// levelId   = keccak256("D-312-BTC")
//           = 0x8a94c9452e5b27b050f1cf8886942611d6ce7d556be4b80396d14bf70a2560fe
// nonce     = 1
// sessionId = keccak256(abi.encode(player, levelId, nonce))
// amount    = 500e18 = 0x1b1ae4d6e2ef500000
// blockNum  = 1
//
// We build a synthetic Log from these values rather than coupling to a
// running anvil instance.

func TestDecodeSessionStarted_HappyPath(t *testing.T) {
	// 32-byte left-pad of the alice address.
	playerTopic := "0x000000000000000000000000328809bc894f92807417d2dad6b7c998c1afdac6"
	levelTopic := "0x8a94c9452e5b27b050f1cf8886942611d6ce7d556be4b80396d14bf70a2560fe"
	sessionTopic := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	// data = abi.encode(uint256 amount, uint256 nonce, uint256 blockNumber)
	// 500e18 = 1b1ae4d6e2ef500000  (left-padded to 32 bytes)
	data := "0x" +
		"00000000000000000000000000000000000000000000001b1ae4d6e2ef500000" + // amount = 500e18
		"0000000000000000000000000000000000000000000000000000000000000001" + // nonce = 1
		"000000000000000000000000000000000000000000000000000000000000000c" //   blockNumber = 12

	log := Log{
		Topics:           []string{SessionStartedTopic, playerTopic, levelTopic, sessionTopic},
		Data:             data,
		TransactionHash:  "0xdeadbeef",
		LogIndex:         "0x2a",
		BlockNumber:      "0xc",
	}

	ev, err := DecodeSessionStarted(log)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if ev.Player != "0x328809bc894f92807417d2dad6b7c998c1afdac6" {
		t.Errorf("player = %s", ev.Player)
	}
	wantAmount := new(big.Int)
	wantAmount.SetString("500000000000000000000", 10) // 500e18
	if ev.Amount.Cmp(wantAmount) != 0 {
		t.Errorf("amount = %s want %s", ev.Amount, wantAmount)
	}
	if ev.Nonce.Uint64() != 1 {
		t.Errorf("nonce = %d", ev.Nonce.Uint64())
	}
	if ev.BlockNumber.Uint64() != 12 {
		t.Errorf("blockNumber = %d", ev.BlockNumber.Uint64())
	}
	if ev.LogIdx != 42 {
		t.Errorf("logIdx = %d", ev.LogIdx)
	}
	if ev.LevelIDHex() != levelTopic {
		t.Errorf("levelIdHex = %s", ev.LevelIDHex())
	}
	if ev.SessionIDHex() != sessionTopic {
		t.Errorf("sessionIdHex = %s", ev.SessionIDHex())
	}
}

func TestDecodeSessionStarted_RejectsWrongTopic(t *testing.T) {
	log := Log{
		Topics: []string{
			"0x0000000000000000000000000000000000000000000000000000000000000000",
			"0x000000000000000000000000328809bc894f92807417d2dad6b7c998c1afdac6",
			"0x" + paddedHex32("level"),
			"0x" + paddedHex32("session"),
		},
		Data: "0x" + threeUint256(0, 0, 0),
	}
	if _, err := DecodeSessionStarted(log); err == nil {
		t.Fatal("expected error on wrong topic[0]")
	}
}

func TestDecodeSessionStarted_RejectsBadDataLength(t *testing.T) {
	log := Log{
		Topics: []string{
			SessionStartedTopic,
			"0x000000000000000000000000328809bc894f92807417d2dad6b7c998c1afdac6",
			"0x" + paddedHex32("level"),
			"0x" + paddedHex32("session"),
		},
		Data: "0x1234", // way too short
	}
	if _, err := DecodeSessionStarted(log); err == nil {
		t.Fatal("expected error on short data")
	}
}

func TestDecodeSessionStarted_RejectsTopicCount(t *testing.T) {
	log := Log{
		Topics: []string{SessionStartedTopic}, // only 1
		Data:   "0x" + threeUint256(0, 0, 0),
	}
	if _, err := DecodeSessionStarted(log); err == nil {
		t.Fatal("expected error on missing topics")
	}
}

func TestParseHexUint64(t *testing.T) {
	cases := map[string]uint64{
		"0x0":       0,
		"0x1":       1,
		"0xff":      255,
		"0x100":     256,
		"0xc":       12,
		"0xdeadbe":  0xdeadbe,
	}
	for in, want := range cases {
		got, err := parseHexUint64(in)
		if err != nil {
			t.Errorf("parse %q: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parse %q = %d, want %d", in, got, want)
		}
	}

	if _, err := parseHexUint64("0xggg"); err == nil {
		t.Error("expected error on bad hex")
	}
}

func TestDecodeLevelID_ASCII(t *testing.T) {
	// "BTC-MED" left-aligned, NUL-padded → returns the trimmed string.
	var b [32]byte
	copy(b[:], "BTC-MED")
	if got := decodeLevelID(b); got != "BTC-MED" {
		t.Errorf("ASCII decode: got %q", got)
	}
}

func TestDecodeLevelID_RandomBytesFallToHex(t *testing.T) {
	var b [32]byte
	for i := range b {
		b[i] = byte(i + 1)
	}
	got := decodeLevelID(b)
	if got[:2] != "0x" {
		t.Errorf("random bytes should fall back to hex, got %q", got)
	}
}

// ---- helpers ----

func paddedHex32(s string) string {
	out := make([]byte, 32)
	copy(out, s)
	return toHex(out)
}

func threeUint256(a, b, c uint64) string {
	return uint256Hex(a) + uint256Hex(b) + uint256Hex(c)
}

func uint256Hex(n uint64) string {
	out := make([]byte, 32)
	for i := 0; i < 8; i++ {
		out[31-i] = byte(n >> (8 * i))
	}
	return toHex(out)
}

func toHex(b []byte) string {
	const hexchars = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[2*i] = hexchars[v>>4]
		out[2*i+1] = hexchars[v&0x0f]
	}
	return string(out)
}
