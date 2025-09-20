// components/Slider.tsx
"use client";

import React from "react";

export interface SliderProps {
  /** current numeric value (will be used as the input's value) */
  value: number;
  /** called with the new numeric value whenever the user drags the thumb */
  onChange: (v: number) => void;

  /** optional Tailwind colour for the filled part of the track */
  filledColor?: string;   // e.g. "bg-indigo-500"
  /** optional Tailwind colour for the thumb */
  thumbColor?: string;    // e.g. "bg-white"
  /** minimum / maximum values (defaults to 0‑100) */
  min?: number;
  max?: number;
}

/**
 * A tiny, colourful `<input type="range">` that mirrors the parent's state.
 */
export const Slider: React.FC<SliderProps> = ({
  value,
  onChange,
  filledColor = "bg-indigo-500",
  thumbColor = "bg-white",
  min = 0,
  max = 100,
}) => {
  const percent = ((value - min) * 100) / (max - min);

  // The trick: we overlay a coloured div (the “filled” part) *behind* the native <input>.
  // The native range element is made transparent except for the thumb.
  return (
    <div className="relative w-64 h-8">
      {/* Filled track (background‑only) */}
      <div
        className={`absolute inset-y-1/2 h-1 -translate-y-1/2 ${filledColor} rounded-full`}
        style={{ width: `${percent}%` }}
      />
      {/* Native range input – invisible track, visible thumb */}
      <input
        type="range"
        min={min}
        max={max}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className={`
          absolute inset-0 w-full h-full
          appearance-none bg-transparent
          cursor-pointer
        `}
        // Thumb styling – works in Chrome/Edge (WebKit) and Firefox
        style={{
          /* WebKit (Chrome, Edge, Safari) */
          "--thumb-bg": thumbColor,
        } as React.CSSProperties}
      />
      <style jsx global>{`
        input[type="range"]::-webkit-Slider-thumb {
          -webkit-appearance: none;
          appearance: none;
          width: 1.5rem;
          height: 1.5rem;
          border-radius: 9999px;
          background: var(--thumb-bg);
          border: 2px solid #fff;
          box-shadow: 0 0 0 2px rgba(0,0,0,0.1);
          transition: transform 0.2s;
        }
        input[type="range"]::-webkit-Slider-thumb:hover {
          transform: scale(1.1);
        }
        input[type="range"]::-moz-range-thumb {
          width: 1.5rem;
          height: 1.5rem;
          border-radius: 9999px;
          background: var(--thumb-bg);
          border: 2px solid #fff;
          box-shadow: 0 0 0 2px rgba(0,0,0,0.1);
          transition: transform 0.2s;
        }
        input[type="range"]::-moz-range-thumb:hover {
          transform: scale(1.1);
        }
      `}</style>
    </div>
  );
};
