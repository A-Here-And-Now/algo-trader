package trader

import (
	"context"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

type TraderResource struct {
	SignalChan               chan models.Signal
	Cancel                   context.CancelFunc // call to stop the goroutine
	Done                     chan struct{}      // closed when Run() exits
	Cfg                      TradeCfg           // keep the config for introspection / restart
	Updates                  chan TradeCfg
}

func NewTraderResource(cfg TradeCfg, done chan struct{}, cancel context.CancelFunc, updates chan TradeCfg) *TraderResource {
	return &TraderResource{
		SignalChan:               make(chan models.Signal),
		Cancel:                   cancel,
		Done:                     done,
		Cfg:                      cfg,
		Updates:                  updates,
	}
}

func (t *TraderResource) Stop() {
	t.Cancel()
	close(t.SignalChan)
}
