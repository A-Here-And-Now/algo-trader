// components/RadioGroup.tsx
import React from "react";

type Option<T extends string | number> = {
  /** The value that will be sent to the parent */
  value: T;
  /** What the user reads */
  label: string;
  /** Optional custom colour (Tailwind class, e.g. "blue", "pink") */
  color?: "blue" | "green" | "purple" | "orange" | "red";
};

type RadioGroupProps<T extends string | number> = {
  /** Array of radio options */
  options: Option<T>[];
  /** Currently selected value – **controlled** */
  value: T;
  /** Called when the user picks a different option */
  onChange: (value: T) => void;
  /** Optional group title (for accessibility) */
  legend?: string;
};

export function RadioGroup<T extends string | number>({
  options,
  value,
  onChange,
  legend,
}: RadioGroupProps<T>) {
  return (
    <fieldset className="space-y-2">
      {legend && (
        <legend className="text-sm font-medium text-gray-700 mb-1">{legend}</legend>
      )}
      <div className="flex flex-wrap gap-2">
        {options.map(({ value: optVal, label, color = "blue" }) => {
          const isChecked = optVal === value;
          const colorCls = {
            blue: "border-blue-500 focus:ring-blue-500",
            green: "border-green-500 focus:ring-green-500",
            purple: "border-purple-500 focus:ring-purple-500",
            orange: "border-orange-500 focus:ring-orange-500",
            red: "border-red-500 focus:ring-red-500",
          }[color];

          return (
            <label
              key={String(optVal)}
              className={`
                relative flex cursor-pointer select-none items-center rounded-md
                border-2 px-4 py-2
                ${colorCls}
                ${isChecked ? "bg-" + color + "-100 text-" + color + "-800" : "bg-white"}
                transition-colors duration-150
                hover:bg-${color}-50
              `}
            >
              <input
                type="radio"
                className="absolute -inset-0.5 opacity-0"
                name="radio-group"
                value={String(optVal)}
                checked={isChecked}
                onChange={() => onChange(optVal)}
              />
              <span className="pointer-events-none">{label}</span>
            </label>
          );
        })}
      </div>
    </fieldset>
  );
}
