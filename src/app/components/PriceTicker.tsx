// components/PriceTicker.tsx
import React from "react";

export interface PriceTickerProps {
  /** A unique identifier – useful for React’s `key` prop */
  id: number;
  /** Value that changes over time (e.g. fetched data) */
  coinbasePrice: number;
  uniswapPrice: number;
}

function formatPrice(value: number): string {
  return "$" + (Math.abs(value) < 100 ? value.toFixed(2) : Math.round(value).toString());
}

export const PriceTicker: React.FC<PriceTickerProps> = ({ id }) => {
  
  return (
    <div className="relative inline-flex h-10 w-36 overflow-hidden rounded-lg shadow-sm ring-1 ring-black/10 dark:ring-white/10">
      {/* Backgrounds with true diagonal split */}
      <div className="absolute inset-0">
        <div
          className="absolute inset-0 bg-gradient-to-br from-blue-500 to-blue-600"
          style={{ clipPath: "polygon(0% 0%, 55% 0%, 45% 100%, 0% 100%)" }}
        />
        <div
          className="absolute inset-0 bg-gradient-to-br from-pink-500 to-pink-600"
          style={{ clipPath: "polygon(55% 0%, 100% 0%, 100% 100%, 45% 100%)" }}
        />
      </div>

      {/* Content */}
      <div className="relative z-10 grid h-full w-full grid-cols-2 text-white">
        <div className="flex items-center justify-center px-2">
          <span className="font-mono text-sm font-semibold tabular-nums">{formatPrice(coinbasePrice)}</span>
        </div>
        <div className="flex items-center justify-center px-2">
          <span className="font-mono text-sm font-semibold tabular-nums">{formatPrice(uniswapPrice)}</span>
        </div>
      </div>
    </div>
  );
};