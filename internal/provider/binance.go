package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/sandswind/perpCS/internal/types"
)

// binanceBaseURL is the Binance USDT-Margined Futures REST API base.
const binanceBaseURL = "https://fapi.binance.com"

// Binance is a live data provider backed by the Binance Futures REST API.
// It uses only public, unauthenticated endpoints.
//
// For historical data older than ~30 days (e.g. D-312) use the
// Binance public data archive: https://data.binance.vision/
// The FetchAggTrades method here covers the live/recent window; for
// historical batch ETL use cmd/ingest (out of scope for v0.1 unit tests).
type Binance struct {
	client *http.Client
}

// NewBinance creates a Binance provider with a sensible default HTTP client.
func NewBinance() *Binance {
	return &Binance{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (b *Binance) Name() string { return "binance" }

// FetchKlines calls GET /fapi/v1/klines.
// interval: "1m", "5m", "1h", etc.
// Max 1500 candles per call; this implementation fetches one page only.
// For multi-day datasets use the ingest pipeline.
func (b *Binance) FetchKlines(ctx context.Context, symbol string, from, to time.Time, interval string) ([]types.Kline, error) {
	fromMS := from.UnixMilli()
	toMS := to.UnixMilli()

	url := fmt.Sprintf("%s/fapi/v1/klines?symbol=%s&interval=%s&startTime=%d&endTime=%d&limit=1500",
		binanceBaseURL, symbol, interval, fromMS, toMS)

	body, err := b.get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("binance klines: %w", err)
	}

	// Binance returns: [[openTime, open, high, low, close, volume, closeTime,
	//                     quoteVol, numTrades, takerBuyBaseVol, takerBuyQuoteVol, ignore]]
	var raw [][]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("binance klines unmarshal: %w", err)
	}

	klines := make([]types.Kline, 0, len(raw))
	for _, row := range raw {
		if len(row) < 11 {
			continue
		}
		k, err := parseKlineRow(symbol, row)
		if err != nil {
			return nil, fmt.Errorf("binance klines parse row: %w", err)
		}
		klines = append(klines, k)
	}
	return klines, nil
}

// FetchAggTrades calls GET /fapi/v1/aggTrades.
// Returns at most 1000 trades per call. For large ranges, callers must paginate.
func (b *Binance) FetchAggTrades(ctx context.Context, symbol string, from, to time.Time) ([]types.AggTrade, error) {
	fromMS := from.UnixMilli()
	toMS := to.UnixMilli()

	url := fmt.Sprintf("%s/fapi/v1/aggTrades?symbol=%s&startTime=%d&endTime=%d&limit=1000",
		binanceBaseURL, symbol, fromMS, toMS)

	body, err := b.get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("binance aggTrades: %w", err)
	}

	// {"a":id,"p":"price","q":"qty","f":firstId,"l":lastId,"T":ts,"m":isBuyerMaker}
	type rawTrade struct {
		TS           int64  `json:"T"`
		Price        string `json:"p"`
		Qty          string `json:"q"`
		IsBuyerMaker bool   `json:"m"`
	}
	var raw []rawTrade
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("binance aggTrades unmarshal: %w", err)
	}

	trades := make([]types.AggTrade, 0, len(raw))
	for _, r := range raw {
		p, err := strconv.ParseFloat(r.Price, 64)
		if err != nil {
			return nil, fmt.Errorf("parse price %q: %w", r.Price, err)
		}
		q, err := strconv.ParseFloat(r.Qty, 64)
		if err != nil {
			return nil, fmt.Errorf("parse qty %q: %w", r.Qty, err)
		}
		side := types.SideBuy
		if r.IsBuyerMaker {
			// Buyer is maker → seller is taker
			side = types.SideSell
		}
		trades = append(trades, types.AggTrade{
			Symbol:    types.Symbol(symbol),
			TS:        r.TS * int64(time.Millisecond),
			Price:     types.PriceFromFloat(p),
			Quantity:  types.QtyFromFloat(q),
			TakerSide: side,
		})
	}
	return trades, nil
}

// FetchFundingRates calls GET /fapi/v1/fundingRate.
func (b *Binance) FetchFundingRates(ctx context.Context, symbol string, from, to time.Time) ([]FundingPoint, error) {
	fromMS := from.UnixMilli()
	toMS := to.UnixMilli()

	url := fmt.Sprintf("%s/fapi/v1/fundingRate?symbol=%s&startTime=%d&endTime=%d&limit=1000",
		binanceBaseURL, symbol, fromMS, toMS)

	body, err := b.get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("binance fundingRate: %w", err)
	}

	type rawFunding struct {
		FundingTime int64  `json:"fundingTime"`
		FundingRate string `json:"fundingRate"`
	}
	var raw []rawFunding
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("binance fundingRate unmarshal: %w", err)
	}

	pts := make([]FundingPoint, 0, len(raw))
	for _, r := range raw {
		rate, err := strconv.ParseFloat(r.FundingRate, 64)
		if err != nil {
			return nil, fmt.Errorf("parse funding rate %q: %w", r.FundingRate, err)
		}
		pts = append(pts, FundingPoint{
			TS:   r.FundingTime * int64(time.Millisecond),
			Rate: rate,
		})
	}
	return pts, nil
}

// get performs a GET request and returns the response body.
func (b *Binance) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", url, resp.StatusCode, body)
	}
	return io.ReadAll(resp.Body)
}

// ---- parse helpers ----

func parseKlineRow(symbol string, row []json.RawMessage) (types.Kline, error) {
	parseTS := func(v json.RawMessage) (int64, error) {
		var n int64
		return n, json.Unmarshal(v, &n)
	}
	parseStr := func(v json.RawMessage) (float64, error) {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return 0, err
		}
		return strconv.ParseFloat(s, 64)
	}
	parseInt32 := func(v json.RawMessage) (int32, error) {
		var n int32
		return n, json.Unmarshal(v, &n)
	}

	openTS, err := parseTS(row[0])
	if err != nil {
		return types.Kline{}, fmt.Errorf("openTS: %w", err)
	}
	open, err := parseStr(row[1])
	if err != nil {
		return types.Kline{}, fmt.Errorf("open: %w", err)
	}
	high, err := parseStr(row[2])
	if err != nil {
		return types.Kline{}, fmt.Errorf("high: %w", err)
	}
	low, err := parseStr(row[3])
	if err != nil {
		return types.Kline{}, fmt.Errorf("low: %w", err)
	}
	close, err := parseStr(row[4])
	if err != nil {
		return types.Kline{}, fmt.Errorf("close: %w", err)
	}
	vol, err := parseStr(row[5])
	if err != nil {
		return types.Kline{}, fmt.Errorf("volume: %w", err)
	}
	closeTS, err := parseTS(row[6])
	if err != nil {
		return types.Kline{}, fmt.Errorf("closeTS: %w", err)
	}
	quoteVol, err := parseStr(row[7])
	if err != nil {
		return types.Kline{}, fmt.Errorf("quoteVol: %w", err)
	}
	numTrades, err := parseInt32(row[8])
	if err != nil {
		return types.Kline{}, fmt.Errorf("numTrades: %w", err)
	}
	takerBuy, err := parseStr(row[9])
	if err != nil {
		return types.Kline{}, fmt.Errorf("takerBuy: %w", err)
	}

	return types.Kline{
		Symbol:      types.Symbol(symbol),
		OpenTS:      openTS * int64(time.Millisecond),
		CloseTS:     (closeTS + 1) * int64(time.Millisecond),
		Open:        types.PriceFromFloat(open),
		High:        types.PriceFromFloat(high),
		Low:         types.PriceFromFloat(low),
		Close:       types.PriceFromFloat(close),
		Volume:      types.QtyFromFloat(vol),
		QuoteVol:    int64(quoteVol * float64(types.Scale)),
		NumTrades:   numTrades,
		TakerBuyVol: types.QtyFromFloat(takerBuy),
	}, nil
}
