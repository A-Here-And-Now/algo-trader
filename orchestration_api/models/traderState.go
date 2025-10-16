package models

type TraderState struct {
	PendingOrder                *PendingOrder
	ActualPositionToken         float64
	ActualPositionUSD           float64 // actual position in USD
	UsdAmountPerFulfilledOrders float64 // actual position in USD without gains or losses
	TargetPositionUSD           float64 // target position in USD
	CurrentPriceUSDPerToken     float64
}