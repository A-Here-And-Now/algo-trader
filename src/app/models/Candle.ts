type Candle = {
  type: "candle";
  symbol: string;
  start: Date;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
  startInSeconds: number;
  index: number;
};

type IncomingCandle = {
  type: "candle";
  symbol: string;
  start: number; // seconds
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
};

export type { Candle, IncomingCandle };