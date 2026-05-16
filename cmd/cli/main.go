// cmd/cli is the v0.2 interactive REPL for CLI trading.
//
// Usage:
//
//	go run ./cmd/cli [flags]
//	make cli
//
// Flags:
//
//	--server   HTTP server address (default: http://localhost:8080)
//	--level    level ID for display (default: D-312-BTC)
//	--seed     not used here, kept for symmetry with cmd/server
//	--balance  initial balance (for display only — actual balance from server)
//
// Commands:
//
//	buy market <qty>          — market buy
//	buy limit <qty> <price>   — limit buy
//	sell market <qty>         — market sell
//	sell limit <qty> <price>  — limit sell
//	cancel <order_id>         — cancel order
//	account / pos             — show account state
//	help                      — show help
//	quit / exit               — exit REPL
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	server := flag.String("server", "http://localhost:8080", "server address")
	level := flag.String("level", "D-312-BTC", "level ID")
	flag.Uint64("seed", 42, "chaos seed (unused in CLI)")
	flag.Float64("balance", 10000, "initial balance hint (actual from server)")
	flag.Parse()

	c := &cliClient{
		serverURL: *server,
		level:     *level,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	c.run()
}

// cliClient holds state for the REPL session.
type cliClient struct {
	serverURL  string
	level      string
	httpClient *http.Client
}

// run is the main REPL loop.
func (c *cliClient) run() {
	// Determine the symbol from the level (D-312-BTC → BTC-MED)
	symbol := levelToSymbol(c.level)
	prompt := fmt.Sprintf("%s> ", symbol)

	fmt.Printf("╔══════════════════════════════════════════════════════╗\n")
	fmt.Printf("║         PERP CRISIS SANDBOX — v0.2 CLI Trader        ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  Server : %-42s ║\n", c.serverURL)
	fmt.Printf("║  Level  : %-42s ║\n", c.level)
	fmt.Printf("║  Type 'help' for commands                            ║\n")
	fmt.Printf("╚══════════════════════════════════════════════════════╝\n\n")

	// Wait for server to be ready
	if err := c.waitReady(5); err != nil {
		fmt.Printf("[error] server not reachable at %s: %v\n", c.serverURL, err)
		fmt.Printf("        Start the server with: make server\n")
		os.Exit(1)
	}

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(prompt)
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if done := c.handle(line); done {
			break
		}
	}
	fmt.Println("\n[bye] See you in the next crisis.")
}

// handle processes one REPL command. Returns true to exit.
func (c *cliClient) handle(line string) bool {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return false
	}
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "quit", "exit", "q":
		return true

	case "help", "h":
		c.printHelp()

	case "account", "acc", "pos":
		c.cmdAccount()

	case "cancel":
		if len(parts) < 2 {
			fmt.Println("[error] usage: cancel <order_id>")
			return false
		}
		c.cmdCancel(parts[1])

	case "buy", "sell":
		c.cmdOrder(parts)

	default:
		fmt.Printf("[error] unknown command %q — type 'help' for commands\n", cmd)
	}
	return false
}

// cmdOrder handles buy/sell commands.
func (c *cliClient) cmdOrder(parts []string) {
	if len(parts) < 3 {
		fmt.Printf("[error] usage: %s market <qty> | %s limit <qty> <price>\n", parts[0], parts[0])
		return
	}
	side := strings.ToLower(parts[0])
	orderType := strings.ToLower(parts[1])
	qty := parts[2]
	price := "0"
	if orderType == "limit" {
		if len(parts) < 4 {
			fmt.Printf("[error] usage: %s limit <qty> <price>\n", side)
			return
		}
		price = parts[3]
	}

	reqBody := map[string]string{
		"side":     side,
		"type":     orderType,
		"quantity": qty,
		"price":    price,
	}
	raw, _ := json.Marshal(reqBody)

	resp, err := c.httpClient.Post(c.serverURL+"/orders", "application/json", bytes.NewReader(raw))
	if err != nil {
		fmt.Printf("[error] %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		_ = json.Unmarshal(body, &errResp)
		fmt.Printf("[error] %s\n", errResp["error"])
		return
	}

	var result struct {
		OrderID uint64 `json:"order_id"`
		Trades  []struct {
			Price    string `json:"price"`
			Quantity string `json:"quantity"`
		} `json:"trades"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Printf("[error] decode response: %v\n", err)
		return
	}

	if len(result.Trades) == 0 {
		fmt.Printf("[order#%d] %s %s %s @ %s — resting (no fills yet)\n",
			result.OrderID, side, orderType, qty, price)
	} else {
		totalQty := 0.0
		totalCost := 0.0
		lastPrice := ""
		for _, t := range result.Trades {
			fmt.Printf("[fill]  %s @ %s  (order#%d)\n", t.Quantity, t.Price, result.OrderID)
			var q, p float64
			fmt.Sscanf(t.Quantity, "%f", &q)
			fmt.Sscanf(t.Price, "%f", &p)
			totalQty += q
			totalCost += q * p
			lastPrice = t.Price
		}
		_ = lastPrice
		if totalQty > 0 {
			avgPrice := totalCost / totalQty
			fmt.Printf("[trade] %s %s %.8f BTC @ %.2f  cost %.2f USDR\n",
				side, orderType, totalQty, avgPrice, totalCost)
		}
	}
	// Print account state after order
	c.cmdAccount()
}

// cmdCancel handles cancel commands.
func (c *cliClient) cmdCancel(idStr string) {
	req, _ := http.NewRequest(http.MethodDelete, c.serverURL+"/orders/"+idStr, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		fmt.Printf("[error] %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		_ = json.Unmarshal(body, &errResp)
		fmt.Printf("[error] %s\n", errResp["error"])
		return
	}
	fmt.Printf("[cancel] order#%s cancelled\n", idStr)
}

// cmdAccount prints the current account state.
func (c *cliClient) cmdAccount() {
	resp, err := c.httpClient.Get(c.serverURL + "/account")
	if err != nil {
		fmt.Printf("[error] %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]string
		_ = json.Unmarshal(body, &errResp)
		fmt.Printf("[account] error: %s\n", errResp["error"])
		return
	}

	var acc struct {
		Balance    string `json:"balance"`
		Positions  []struct {
			Symbol   string `json:"symbol"`
			Side     string `json:"side"`
			Size     string `json:"size"`
			AvgEntry string `json:"avg_entry"`
			Margin   string `json:"margin"`
			UPnL     string `json:"upnl"`
		} `json:"positions"`
		OpenOrders []struct {
			ID       uint64 `json:"id"`
			Side     string `json:"side"`
			Type     string `json:"type"`
			Price    string `json:"price"`
			Quantity string `json:"quantity"`
		} `json:"open_orders"`
	}
	if err := json.Unmarshal(body, &acc); err != nil {
		fmt.Printf("[error] decode account: %v\n", err)
		return
	}

	fmt.Printf("[account] balance: %s USDR\n", acc.Balance)
	if len(acc.Positions) == 0 {
		fmt.Printf("[pos]     no open positions\n")
	} else {
		for _, p := range acc.Positions {
			side := strings.ToUpper(p.Side)
			fmt.Printf("[pos]     %s %s %s @ %s  uPnL %s USDR  margin %s USDR\n",
				side, p.Symbol, p.Size, p.AvgEntry, p.UPnL, p.Margin)
		}
	}
	if len(acc.OpenOrders) > 0 {
		for _, o := range acc.OpenOrders {
			fmt.Printf("[orders]  #%d %s %s %s @ %s\n",
				o.ID, o.Side, o.Type, o.Quantity, o.Price)
		}
	}
}

// waitReady pings /health until it responds or maxAttempts is exhausted.
func (c *cliClient) waitReady(maxAttempts int) error {
	for i := 0; i < maxAttempts; i++ {
		resp, err := c.httpClient.Get(c.serverURL + "/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		if i < maxAttempts-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	return fmt.Errorf("server not ready after %d attempts", maxAttempts)
}

func (c *cliClient) printHelp() {
	fmt.Println(`
Commands:
  buy market <qty>           market buy (e.g. buy market 0.1)
  buy limit <qty> <price>    limit buy  (e.g. buy limit 0.1 7800)
  sell market <qty>          market sell
  sell limit <qty> <price>   limit sell
  cancel <order_id>          cancel a resting order
  account / acc / pos        show balance and positions
  help                       show this help
  quit / exit                exit REPL`)
}

// levelToSymbol maps a level ID to a trading symbol.
func levelToSymbol(level string) string {
	switch {
	case strings.Contains(level, "BTC"):
		return "BTC-MED"
	default:
		return "BTC-MED"
	}
}
