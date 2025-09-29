package signaler

import (
	"fmt"

	"github.com/A-Here-And-Now/algo-trader/orchestration_api/enum"
	"github.com/A-Here-And-Now/algo-trader/orchestration_api/models"
)

func NewIndicator(indicatorType enum.IndicatorType, candleHistory []models.Candle, priceHistory []models.Ticker, candleHistory26Days []models.Candle) Indicator {
	switch indicatorType {
	case enum.EMA:
		return NewEMA(candleHistory, priceHistory, candleHistory26Days)
	case enum.RSI:
		return NewRSI(candleHistory, priceHistory, candleHistory26Days)
	case enum.MACD:
		return NewMACD(candleHistory, priceHistory, candleHistory26Days)
	case enum.Stochastic:
		return NewStochastic(candleHistory, priceHistory, candleHistory26Days)
	case enum.BollingerBands:
		return NewBollingerBands(candleHistory, priceHistory, candleHistory26Days)
	case enum.ADX:
		return NewADX(candleHistory, priceHistory, candleHistory26Days)
	case enum.ATR:
		return NewATR(candleHistory, priceHistory, candleHistory26Days)
	case enum.CCI:
		return NewCCI(candleHistory, priceHistory, candleHistory26Days)
	default:
		panic(fmt.Sprintf("Unknown indicator type: %s", indicatorType))
	}
}

type Indicator interface {
	AddNewCandle(candle models.Candle)
	ReplaceLatestCandle(candle models.Candle)
	GetValue() float64
	recalculate()
}

type EMA struct {
	period int
	value float64
}

/*******************************************/
/********************EMA********************/
/*******************************************/
func NewEMA(candleHistory []models.Candle, priceHistory []models.Ticker, candleHistory26Days []models.Candle) *EMA {
	return &EMA{
		period: 0,
		value: 0,
	}
}

func (e *EMA) AddNewCandle(candle models.Candle) {
	e.value = 0
	e.recalculate()
}

func (e *EMA) ReplaceLatestCandle(candle models.Candle) {
	e.value = 0
	e.recalculate()
}

func (e *EMA) GetValue() float64 {
	return e.value
}

func (e *EMA) recalculate() {
	e.value = 0
}

/*******************************************/
/********************RSI********************/
/*******************************************/
type RSI struct {
	period int
	value float64
}

func NewRSI(candleHistory []models.Candle, priceHistory []models.Ticker, candleHistory26Days []models.Candle) *RSI {
	return &RSI{
		period: 0,
		value: 0,
	}
}

func (r *RSI) AddNewCandle(candle models.Candle) {
	r.value = 0
	r.recalculate()
}

func (r *RSI) ReplaceLatestCandle(candle models.Candle) {
	r.value = 0
	r.recalculate()
}

func (r *RSI) GetValue() float64 {
	return r.value
}

func (r *RSI) recalculate() {
	r.value = 0
}

/*******************************************/
/********************MACD*******************/
/*******************************************/
type MACD struct {
	period int
	value float64
}

func NewMACD(candleHistory []models.Candle, priceHistory []models.Ticker, candleHistory26Days []models.Candle) *MACD {
	return &MACD{
		period: 0,
		value: 0,
	}
}

func (m *MACD) AddNewCandle(candle models.Candle) {
	m.value = 0
	m.recalculate()
}

func (m *MACD) ReplaceLatestCandle(candle models.Candle) {
	m.value = 0
	m.recalculate()
}

func (m *MACD) GetValue() float64 {
	return m.value
}

func (m *MACD) recalculate() {
	m.value = 0
}

/*******************************************/
/********************Stochastic*************/
/*******************************************/
type Stochastic struct {
	period int
	value float64
}

func NewStochastic(candleHistory []models.Candle, priceHistory []models.Ticker, candleHistory26Days []models.Candle) *Stochastic {
	return &Stochastic{
		period: 0,
		value: 0,
	}
}

func (s *Stochastic) AddNewCandle(candle models.Candle) {
	s.value = 0
	s.recalculate()
}

func (s *Stochastic) ReplaceLatestCandle(candle models.Candle) {
	s.value = 0
	s.recalculate()
}

func (s *Stochastic) GetValue() float64 {
	return s.value
}

func (s *Stochastic) recalculate() {
	s.value = 0
}

/*******************************************/
/********************BollingerBands*********/
/*******************************************/
type BollingerBands struct {
	period int
	value float64
}

func NewBollingerBands(candleHistory []models.Candle, priceHistory []models.Ticker, candleHistory26Days []models.Candle) *BollingerBands {
	return &BollingerBands{
		period: 0,
		value: 0,
	}
}

func (b *BollingerBands) AddNewCandle(candle models.Candle) {
	b.value = 0
	b.recalculate()
}

func (b *BollingerBands) ReplaceLatestCandle(candle models.Candle) {
	b.value = 0
	b.recalculate()
}

func (b *BollingerBands) GetValue() float64 {
	return b.value
}

func (b *BollingerBands) recalculate() {
	b.value = 0
}

/*******************************************/
/********************ADX********************/
/*******************************************/
type ADX struct {
	period int
	value float64
}

func NewADX(candleHistory []models.Candle, priceHistory []models.Ticker, candleHistory26Days []models.Candle) *ADX {
	return &ADX{
		period: 0,
		value: 0,
	}
}

func (a *ADX) AddNewCandle(candle models.Candle) {
	a.value = 0
	a.recalculate()
}

func (a *ADX) ReplaceLatestCandle(candle models.Candle) {
	a.value = 0
	a.recalculate()
}

func (a *ADX) GetValue() float64 {
	return a.value
}

func (a *ADX) recalculate() {
	a.value = 0
}

/*******************************************/
/********************ATR********************/
/*******************************************/
type ATR struct {
	period int
	value float64
}

func NewATR(candleHistory []models.Candle, priceHistory []models.Ticker, candleHistory26Days []models.Candle) *ATR {
	return &ATR{
		period: 0,
		value: 0,
	}
}

func (a *ATR) AddNewCandle(candle models.Candle) {
	a.value = 0
	a.recalculate()
}

func (a *ATR) ReplaceLatestCandle(candle models.Candle) {
	a.value = 0
	a.recalculate()
}

func (a *ATR) GetValue() float64 {
	return a.value
}

func (a *ATR) recalculate() {
	a.value = 0
}

/*******************************************/
/********************CCI********************/
/*******************************************/
type CCI struct {
	period int
	value float64
}

func NewCCI(candleHistory []models.Candle, priceHistory []models.Ticker, candleHistory26Days []models.Candle) *CCI {
	return &CCI{
		period: 0,
		value: 0,
	}
}

func (c *CCI) AddNewCandle(candle models.Candle) {
	c.value = 0
	c.recalculate()
}

func (c *CCI) ReplaceLatestCandle(candle models.Candle) {
	c.value = 0
	c.recalculate()
}

func (c *CCI) GetValue() float64 {
	return c.value
}

func (c *CCI) recalculate() {
	c.value = 0
}
