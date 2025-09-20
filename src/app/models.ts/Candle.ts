type Candle = {
    type: "candle";
    symbol: string;
    time: string;
    open: number;
    high: number;
    low: number;
    close: number;
    volume: number;
  };

export default Candle;