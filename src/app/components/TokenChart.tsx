import React from "react";
import { 
  ComposedChart, 
  Bar,
  XAxis, 
  YAxis, 
  Tooltip, 
  CartesianGrid, 
  ResponsiveContainer
} from "recharts";
import { useCandleStore } from "../services/store";
import Price from "../models/Price";
import { Candle } from "../models/Candle";

type WSMessage = Price | Candle;

type TokenChartProps = {
  /** e.g. "WETH" */
  symbol: string;
  /** className for the container */
  className?: string;
};


// Custom candlestick shape
const CandlestickShape = (props: any): React.ReactElement => {
  const { payload, x, y, width, height } = props;
  
  if (!payload) return <g />; // Return empty group instead of null
  
  const { open, high, low, close } = payload;
  const isBullish = close >= open;
  
  // Colors
  const bodyColor = isBullish ? "#10b981" : "#ef4444"; // green : red
  const wickColor = "#6b7280"; // gray
  
  // Calculate positions
  const centerX = x + width / 2;
  const bodyWidth = width * 0.6;
  const bodyX = x + (width - bodyWidth) / 2;
  
  // Y-axis scaling (assumes the chart has set up proper domain)
  const chartHeight = 200; // This should ideally come from chart context
  const priceRange = high - low;
  const scale = chartHeight / priceRange;
  
  const highY = y;
  const lowY = y + chartHeight;
  const openY = y + (high - open) * scale;
  const closeY = y + (high - close) * scale;
  
  const bodyTop = Math.min(openY, closeY);
  const bodyBottom = Math.max(openY, closeY);
  const bodyHeight = Math.max(1, Math.abs(bodyBottom - bodyTop));
  
  return (
    <g>
      {/* High-Low wick */}
      <line
        x1={centerX}
        y1={highY}
        x2={centerX}
        y2={lowY}
        stroke={wickColor}
        strokeWidth={1}
        />
      
      {/* Body */}
      <rect
        x={bodyX}
        y={bodyTop}
        width={bodyWidth}
        height={bodyHeight}
        fill={bodyColor}
        stroke={bodyColor}
        strokeWidth={1}
      />
    </g>
  );
};

const MemoizedCandlestick = React.memo(CandlestickShape, (prev, next) => {
  // Only re-render if candle data changed
  return prev.payload.startInSeconds == next.payload.startInSeconds;
});  

// Custom tooltip for candlestick data
const CandlestickTooltip = ({ active, payload }: any) => {
  if (active && payload && payload.length && payload[0].payload) {
    const data = payload[0].payload as Candle;
    const change = data.close - data.open;
    const changePercent = ((change / data.open) * 100);
    const isPositive = change >= 0;
    
    return (
      <div className="bg-white p-3 border border-gray-200 rounded-lg shadow-lg">
        <p className="font-medium text-gray-900 mb-2">
          {data.symbol} - {data.start.toLocaleString()}
        </p>
        <div className="space-y-1 text-sm">
          <div className="flex justify-between gap-4">
            <span className="text-gray-600">Open:</span>
            <span className="font-mono">${data.open.toFixed(4)}</span>
          </div>
          <div className="flex justify-between gap-4">
            <span className="text-gray-600">High:</span>
            <span className="font-mono">${data.high.toFixed(4)}</span>
          </div>
          <div className="flex justify-between gap-4">
            <span className="text-gray-600">Low:</span>
            <span className="font-mono">${data.low.toFixed(4)}</span>
          </div>
          <div className="flex justify-between gap-4">
            <span className="text-gray-600">Close:</span>
            <span className={`font-mono ${isPositive ? 'text-green-600' : 'text-red-600'}`}>
              ${data.close.toFixed(4)}
            </span>
          </div>
          <div className="flex justify-between gap-4">
            <span className="text-gray-600">Change:</span>
            <span className={`font-mono ${isPositive ? 'text-green-600' : 'text-red-600'}`}>
              {isPositive ? '+' : ''}${change.toFixed(4)} ({isPositive ? '+' : ''}{changePercent.toFixed(2)}%)
            </span>
          </div>
          <div className="flex justify-between gap-4">
            <span className="text-gray-600">Volume:</span>
            <span className="font-mono">{data.volume.toLocaleString()}</span>
          </div>
        </div>
      </div>
    );
  }
  return null;
};

export const TokenChart: React.FC<TokenChartProps> = ({
  symbol,
  className,
}) => {
  const candles = useCandleStore((s) => s.candles[symbol]) || [];
    
  // Calculate Y-axis domain
  const allPrices = candles.flatMap(c => [c.high, c.low]);
  const minPrice = allPrices.length > 0 ? Math.min(...allPrices) : 0;
  const maxPrice = allPrices.length > 0 ? Math.max(...allPrices) : 100;
  const padding = (maxPrice - minPrice) * 0.05;
  const yDomain = [Math.max(0, minPrice - padding), maxPrice + padding];
  
  if (!candles || candles.length === 0) {
    return (
      <div className={`${className} flex items-center justify-center`}>
        <div className="text-center text-gray-500">
          <p className="text-lg font-medium">No data available</p>
          <p className="text-sm">Waiting for {symbol} candle data...</p>
        </div>
      </div>
    );
  }

  return (
    <div className={className}>
      <div className="mb-2">
        <h3 className="text-lg font-semibold text-gray-900">{symbol} Candlestick Chart</h3>
        <p className="text-sm text-gray-500">{candles.length} candles</p>
      </div>
      
      <ResponsiveContainer width="100%" height={300}>
        <ComposedChart
          data={candles}
          margin={{ top: 20, right: 30, left: 20, bottom: 20 }}
        >
          <CartesianGrid strokeDasharray="3 3" stroke="#f3f4f6" />
          <XAxis
            dataKey="startInSeconds"
            type="number"
            scale="time"
            domain={['dataMin', 'dataMax']}
            tickFormatter={(ms: number) =>
              new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false })
            }
            tick={{ fontSize: 11, fill: '#6b7280' }}
            axisLine={{ stroke: '#d1d5db' }}
            tickLine={{ stroke: '#d1d5db' }}
          />
          <YAxis
            domain={yDomain}
            tickFormatter={(value: number) => `$${value.toFixed(2)}`}
            tick={{ fontSize: 11, fill: '#6b7280' }}
            axisLine={{ stroke: '#d1d5db' }}
            tickLine={{ stroke: '#d1d5db' }}
            width={60}
          />
          <Tooltip content={<CandlestickTooltip />} />
          
          {/* Use Bar with custom shape to render candlesticks */}
          <Bar 
            dataKey="high" 
            shape={(props: any) => <MemoizedCandlestick {...props} />}
            fill="transparent"
          />
        </ComposedChart>
      </ResponsiveContainer>
    </div>
  );
};
