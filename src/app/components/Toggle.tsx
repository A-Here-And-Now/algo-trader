// components/Toggle.tsx
"use client";

import React from "react";
import clsx from "clsx";

/**
 * Props
 * -----------------------------------------------------------------
 * optionA / optionB      – the two text labels.
 * value                  – current boolean state (false = optionA selected).
 * onChange               – called with the new boolean when the user toggles.
 *
 * The following props allow the parent to colour‑customise the toggle.
 * They accept **any Tailwind class string** (or a raw CSS colour via `style`).
 *
 * activeBgClass          – background when `value === true`  (right side active)
 * inactiveBgClass        – background when `value === false` (left side active)
 *
 * thumbClass             – extra classes for the sliding thumb.
 *
 * activeLabelClass       – extra classes for the *active* label text.
 * inactiveLabelClass     – extra classes for the *inactive* label text.
 *
 * If a prop is omitted the component falls back to a friendly default.
 */
export interface ToggleProps {
  optionA?: string;
  optionB?: string;
  value: boolean;
  onChange: (newValue: boolean) => void;

  // ----- colour customisation -------------------------------------------------
  activeBgClass?: string; // e.g. "bg-gradient-to-r from-pink-500 to-purple-500"
  inactiveBgClass?: string; // e.g. "bg-gray-300"
  thumbClass?: string; // e.g. "bg-white" or "bg-indigo-100"

  activeLabelClass?: string; // e.g. "text-white font-bold"
  inactiveLabelClass?: string; // e.g. "text-gray-800"

  /** Optional inline style for the thumb (useful for raw hex/rgb values) */
  thumbStyle?: React.CSSProperties;
}

/**
 * A colourful two‑option switch whose colours are supplied by the parent.
 */
export const Toggle: React.FC<ToggleProps> = ({
  optionA,
  optionB,
  value,
  onChange,

  // defaults – keep the visual spirit of the original component
  activeBgClass = "bg-gradient-to-r from-indigo-500 to-purple-500",
  inactiveBgClass = "bg-gray-200",
  thumbClass = "bg-white",
  activeLabelClass = "text-white",
  inactiveLabelClass = "text-gray-800",
  thumbStyle,
}) => {
  // --------- Interaction -------------------------------------------------------
  const toggle = () => onChange(!value);

  // --------- Choose the proper classes for the current state ------------------
  const containerBg = value ? activeBgClass : inactiveBgClass;
  const leftLabelCls = value ? inactiveLabelClass : activeLabelClass; // left side is *inactive* when value===true
  const rightLabelCls = value ? activeLabelClass : inactiveLabelClass; // right side is *active* when value===true

  // --------- Render -----------------------------------------------------------
  return (
    <button
      type="button"
      role="switch"
      onClick={toggle}
      className={clsx(
        "relative inline-flex items-center h-10 w-36 rounded-full overflow-hidden focus:outline-none focus-visible:ring-2 focus-visible:ring-white transition-colors",
        containerBg
      )}
    >
      {/* Moving highlight (covers half the control) */}
      <span
        className={clsx(
          "absolute top-1 bottom-1 rounded-full shadow-md transition-all duration-300",
          thumbClass
        )}
        style={{
          left: value ? "calc(50% + 0.25rem)" : "0.25rem",
          width: "calc(50% - 0.25rem)",
          ...(thumbStyle || {}),
        }}
      />

      {/* Labels (two columns, non-overlapping) */}
      <div className="absolute inset-0 z-10 grid grid-cols-2 items-center text-sm font-medium select-none pointer-events-none">
        <span className={clsx("pl-3 text-left transition-colors", leftLabelCls)}>{optionA}</span>
        <span className={clsx("pr-3 text-right transition-colors", rightLabelCls)}>{optionB}</span>
      </div>
    </button>
  );
};
