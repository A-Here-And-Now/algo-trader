package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

var (
	apiKey        = "YOUR_API_KEY"
	privateKeyPem = "PRIVATE_KEY_PEM"
	apiSecret     = "API_SECRET"
)
var tokens = []string{"ETH-USD", "WBTC-USD", "LINK-USD", "UNI-USD", "AAVE-USD", "DOT-USD", "ENA-USD", "MNT-USD", "OKB-USD", "POL-USD"}
var tokenToggles = make(map[string]bool)

var availableFunds = 50000.0
var mgr *Manager
var coinbaseClient *CoinbaseClient
type ctxKey struct{}

var loggerKey = ctxKey{}

// ---------- HANDLERS ----------
func toggleTokenHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if _, ok := tokenToggles[token]; !ok {
		http.Error(w, "token not found", http.StatusNotFound)
		return
	}

	log := LoggerFrom(r)
	log.Printf("Toggling token: %s: %t", token, tokenToggles[token])
	tokenToggles[token] = !tokenToggles[token]

	var status string
	if tokenToggles[token] {
		if err := mgr.Start(token); err != nil {
			http.Error(w, "cannot start: "+err.Error(), http.StatusConflict)
			return
		}
		status = "started"
	} else {
		if err := mgr.Stop(token); err != nil {
			http.Error(w, "cannot stop: "+err.Error(), http.StatusNotFound)
			return
		}
		status = "stopped"
	}

	log.Printf("Token is now: %s ('true' is ON, 'false' is OFF)", tokenToggles[token])

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  status,
		"toggles": tokenToggles,
	})
}

func updateTradingStrategyHandler(w http.ResponseWriter, r *http.Request) {
	str := r.URL.Query().Get("strategy")
	strategy := GetStrategy(str)
	log := LoggerFrom(r)
	log.Printf("Updating trading philosophy from %s to %s", mgr.Cfg.strategy, strategy)

	mgr.updateStrategy(strategy)
	for _, trader := range mgr.safeGetTraderResources() {
		trader.cfg.Strategy = strategy
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":   "updated",
		"strategy": strategy,
	})
}

func updateMaxPLHandler(w http.ResponseWriter, r *http.Request) {
	maxPL := r.URL.Query().Get("maxPL")
	maxPLInt, err := strconv.ParseInt(maxPL, 10, 64)

	if err != nil {
		http.Error(w, "invalid maxPL", http.StatusBadRequest)
		return
	}

	mgr.updates <- ManagerCfg{
		maxPL:    maxPLInt,
		strategy: mgr.Cfg.strategy,
		funds:    mgr.Cfg.funds,
	}
}

func updateStrategyHandler(w http.ResponseWriter, r *http.Request) {
	mgr.updateStrategy(GetStrategy(r.URL.Query().Get("strategy")))
}

func priceHistoryHandler(w http.ResponseWriter, r *http.Request) {
	allPriceHistory := make(map[string][]Ticker)
	for symbol, resource := range mgr.marketPriceResources {
		allPriceHistory[symbol] = resource.priceHistory
	}
	json.NewEncoder(w).Encode(allPriceHistory)
}

func candleHistoryHandler(w http.ResponseWriter, r *http.Request) {
	allCandleHistory := make(map[string][]Candle)
	for symbol, resource := range mgr.marketPriceResources {
		allCandleHistory[symbol] = resource.candleHistory
	}
	json.NewEncoder(w).Encode(allCandleHistory)
}

// ---------- MAIN ----------
func main() {
	// Logger

	l, err := newLogger()
	if err != nil {
		log.Printf("⚠️ Could not open log file, falling back to stdout: %v", err)
		l = &logger{log.New(os.Stdout, "", log.LstdFlags)}
	}

	// init token toggles
	for _, token := range tokens {
		tokenToggles[token] = false
	}

	// create shutdown context
	shutdownCtx, shutdown := context.WithCancel(context.Background())

	// propagate manager lifecycle context so we can skip reallocations during shutdown
	mgr = NewManager(availableFunds, 1000, TrendFollowing, shutdownCtx)

	// listen to OS signals
	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-sigCtx.Done()
		shutdown()
	}()

	// mux
	mux := http.NewServeMux()
	mux.HandleFunc("/toggleToken", toggleTokenHandler)
	mux.HandleFunc("/updateTradingPhilosophy", updateTradingStrategyHandler)
	mux.HandleFunc("/updateMaxPL", updateMaxPLHandler)
	mux.HandleFunc("/updateStrategy", updateStrategyHandler)
	mux.HandleFunc("/ws", mgr.wsHandler) // note: `mgr` is a *value* of type *Manager
	mux.HandleFunc("/priceHistory", priceHistoryHandler)
	mux.HandleFunc("/candleHistory", candleHistoryHandler)

	// wrap with logging
	handler := loggingMiddleware(mux, l)

	// server
	srv := &http.Server{
		Addr:    ":8080",
		Handler: handler,
		// all requests inherit shutdownCtx automatically
		BaseContext: func(_ net.Listener) context.Context { return shutdownCtx },
	}

	// start server
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server stopped with error: %v", err)
		}
	}()

	mgr.startCoinbaseFeed(shutdownCtx, "wss://advanced-trade-ws.coinbase.com")
	mgr.startOrderAndPositionValuationWebSocket(shutdownCtx, "wss://advanced-trade-ws-user.coinbase.com")
	coinbaseClient = NewCoinbaseClient(os.Getenv("COINBASE_URL"))

	// wait for shutdown signal
	<-shutdownCtx.Done()
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	//stop all traders
	mgr.StopAll()

	log.Println("Server exiting")
}
