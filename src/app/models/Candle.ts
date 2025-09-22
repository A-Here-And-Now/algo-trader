type Candle = {
    type: "candle";
    symbol: string;
    start: string;
    open: number;
    high: number;
    low: number;
    close: number;
    volume: number;
  };

export default Candle;