// components/PriceTicker.tsx
import React from "react";
import { usePriceStore } from "../services/store";

export interface PriceTickerProps {
  /** A unique identifier – useful for React’s `key` prop */
  id: number;
  symbol: string;
}

function formatPrice(value: number): string {
  return "$" + (Math.abs(value) < 100 ? value.toFixed(2) : Math.round(value).toString());
}

export const PriceTicker: React.FC<PriceTickerProps> = ({ id, symbol }) => {
  const price = usePriceStore((s) => s.prices[symbol]) || 0;

  return (
    <div className="relative inline-flex h-10 w-36 overflow-hidden rounded-lg shadow-sm ring-1 ring-black/10 dark:ring-white/10">
      {/* Background with diagonal split */}
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
          <span className="font-mono text-sm font-semibold tabular-nums">{formatPrice(price)}</span>
        </div>
        <div className="flex items-center justify-center px-2">
          <span className="font-mono text-sm font-semibold tabular-nums">{"N/A"}</span>
        </div>
      </div>
    </div>
  );
};