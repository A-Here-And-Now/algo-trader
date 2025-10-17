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

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/entities/manager"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
)

var (
	apiKey = os.Getenv("COINBASE_API_KEY")
	// privateKeyPem = os.Getenv("PRIVATE_KEY_PEM")
	apiSecret = os.Getenv("COINBASE_API_SECRET")
)
var tokens = []string{"ETH-USD", "WBTC-USD", "LINK-USD", "UNI-USD", "AAVE-USD", "DOT-USD", "ENA-USD", "MNT-USD", "OKB-USD", "POL-USD"}

var mgr *manager.Manager

type ctxKey struct{}

var loggerKey = ctxKey{}

// ---------- HANDLERS ----------
func ToggleTokenHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	tokenToggles := mgr.GetTokenToggles()
	if _, ok := tokenToggles[token]; !ok {
		http.Error(w, "token not found", http.StatusNotFound)
		return
	}

	log := LoggerFrom(r)
	log.Printf("Toggling token: %s: %t", token, tokenToggles[token])
	mgr.ToggleToken(token)

	newTokenToggles := mgr.GetTokenToggles()
	var status string
	if newTokenToggles[token] {
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

func UpdateTradingStrategyHandler(w http.ResponseWriter, r *http.Request) {
	str := r.URL.Query().Get("strategy")
	token := r.URL.Query().Get("token")
	strategy := enum.GetStrategy(str)
	log := LoggerFrom(r)
	log.Printf("Updating trading philosophy from %s to %s for token %s", mgr.GetStrategy(token).String(), strategy.String(), token)
	mgr.UpdateStrategy(token, strategy)

	w.Header().Set("Content-Type", "application/json")
}

func UpdateCandleSizeHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	candleSize := r.URL.Query().Get("candleSize")
	candleSizeEnum := enum.GetCandleSizeFromString(candleSize)
	log := LoggerFrom(r)
	log.Printf("Updating candle size from %s to %s for token %s", mgr.GetCandleSize(token).String(), candleSizeEnum.String(), token)
	mgr.UpdateCandleSize(token, candleSizeEnum)
	
	w.Header().Set("Content-Type", "application/json")
}

func UpdateMaxPLHandler(w http.ResponseWriter, r *http.Request) {
	maxPL := r.URL.Query().Get("maxPL")
	maxPLInt, err := strconv.ParseInt(maxPL, 10, 64)

	if err != nil {
		http.Error(w, "invalid maxPL", http.StatusBadRequest)
		return
	}

	mgr.UpdateMaxPL(maxPLInt)
}

func PriceHistoryHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(mgr.GetAllPriceHistory())
}

func CandleHistoryHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(mgr.GetAllCandleHistory())
}

func UpdateAllocatedFundsHandler(w http.ResponseWriter, r *http.Request) {
	allocatedFunds := r.URL.Query().Get("allocatedFunds")
	allocatedFundsFloat, err := strconv.ParseFloat(allocatedFunds, 64)
	if err != nil {
		http.Error(w, "invalid allocated funds", http.StatusBadRequest)
		return
	}
	mgr.UpdateAllocatedFunds(allocatedFundsFloat)
}

func UpdateExchangeHandler(w http.ResponseWriter, r *http.Request) {
	exchange := r.URL.Query().Get("exchange")
	exchangeEnum := enum.GetExchangeFromString(exchange)
	mgr.UpdateExchange(exchangeEnum)
}

// ---------- MAIN ----------
func main() {
	// Logger

	l, err := NewLogger()
	if err != nil {
		log.Printf("⚠️ Could not open log file, falling back to stdout: %v", err)
		l = &logger{log.New(os.Stdout, "", log.LstdFlags)}
	}

	// create shutdown context
	shutdownCtx, shutdown := context.WithCancel(context.Background())

	// propagate manager lifecycle context so we can skip reallocations during shutdown
	mgr = manager.NewManager(50000.0, 1000, enum.TrendFollowing, enum.CandleSize5m, shutdownCtx, apiKey, apiSecret, tokens)

	// listen to OS signals
	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-sigCtx.Done()
		shutdown()
	}()

	// mux
	mux := http.NewServeMux()
	mux.HandleFunc("/toggleToken", ToggleTokenHandler)
	mux.HandleFunc("/updateTradingStrategy", UpdateTradingStrategyHandler)
	mux.HandleFunc("/updateMaxPL", UpdateMaxPLHandler)
	mux.HandleFunc("/updateCandleSize", UpdateCandleSizeHandler)
	mux.HandleFunc("/updateAllocatedFunds", UpdateAllocatedFundsHandler)
	mux.HandleFunc("/updateExchange", UpdateExchangeHandler)
	mux.HandleFunc("/ws", mgr.WebSocketHandler) // note: `mgr` is a *value* of type *Manager
	mux.HandleFunc("/priceHistory", PriceHistoryHandler)
	mux.HandleFunc("/candleHistory", CandleHistoryHandler)

	// wrap with logging
	handler := LoggingMiddleware(mux, l)

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

	mgr.RefreshTokenBalances()

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
