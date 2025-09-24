import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { Candle } from "../models/Candle";

type CandleState = {
  candles: { [symbol: string]: Candle[] };
  updateCandles: (symbol: string, candle: Candle) => void;
};
export const useCandleStore = create<CandleState>()(
  immer((set) => ({
    candles: {},
    updateCandles: (symbol, candle) =>
      set((state) => {
        if (!state.candles[symbol]) {
          state.candles[symbol] = [];
        }
        state.candles[symbol].push(candle);
      }),
  }))
);

type PriceState = {
  prices: { [symbol: string]: number };
  updatePrice: (symbol: string, price: number) => void;
};
export const usePriceStore = create<PriceState>((set) => ({
    prices: {},
    updatePrice: (symbol, price) =>
        set((state) => ({ prices: { ...state.prices, [symbol]: price } })),
}));
