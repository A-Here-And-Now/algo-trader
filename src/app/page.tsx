"use client";

import React from "react";
import { RadioGroup } from "./components/RadioButton";
import { Toggle } from "./components/Toggle";
import { Slider } from "./components/Slider";
import { PriceTicker } from "./components/PriceTicker";
import { TokenChart } from "./components/TokenChart";
import { usePriceStore, useCandleStore } from "./services/store";
import Price from "./models/Price";
import Candle from "./models/Candle";

type Philosophy = "trend" | "mean" | "arbitrage" | "momentum";

const TOKENS = [
  { symbol: "ETH-USD", containerBg: "bg-gradient-to-br from-sky-50 to-sky-100" },
  { symbol: "BTC-USD", containerBg: "bg-gradient-to-br from-amber-50 to-amber-100" },
  { symbol: "LINK-USD", containerBg: "bg-gradient-to-br from-blue-50 to-blue-100" },
  { symbol: "UNI-USD", containerBg: "bg-gradient-to-br from-rose-50 to-rose-100" },
  { symbol: "AAVE-USD", containerBg: "bg-gradient-to-br from-teal-50 to-teal-100" },
  { symbol: "DOT-USD", containerBg: "bg-gradient-to-br from-indigo-50 to-indigo-100" },
  { symbol: "ENA-USD", containerBg: "bg-gradient-to-br from-emerald-50 to-emerald-100" },
  { symbol: "MNT-USD", containerBg: "bg-gradient-to-br from-lime-50 to-lime-100" },
  { symbol: "OKB-USD", containerBg: "bg-gradient-to-br from-cyan-50 to-cyan-100" },
  { symbol: "POL-USD", containerBg: "bg-gradient-to-br from-violet-50 to-violet-100" },
];

type Token = {
  symbol: string;
  containerBg: string;
};

const DUMMY_PRICES: Record<string, { cb: number; uni: number }> = {
  "ETH-USD": { cb: 2345.12, uni: 2342.87 },
  "BTC-USD": { cb: 62300.44, uni: 62280.10 },
  "LINK-USD": { cb: 18.22, uni: 18.18 },
  "UNI-USD": { cb: 7.84, uni: 7.82 },
  "AAVE-USD": { cb: 96.10, uni: 96.02 },
  "DOT-USD": { cb: 6.05, uni: 6.03 },
  "ENA-USD": { cb: 0.62, uni: 0.61 },
  "MNT-USD": { cb: 0.48, uni: 0.47 },
  "OKB-USD": { cb: 42.10, uni: 42.03 },
  "POL-USD": { cb: 0.72, uni: 0.71 },
};

function isPriceMessage(msg: any): msg is Price {
  return msg.type === "price" && typeof msg.price === "number";
}

function isCandleMessage(msg: any): msg is Candle {
  return msg.type === "candle" && typeof msg.start === "number";
}

export default function Home() {
  const [tokens, setTokens] = React.useState<Token[]>(TOKENS);
  const [newToken, setNewToken] = React.useState<string>("");
  const [philosophy, setPhilosophy] = React.useState<Philosophy>("trend");
  const [exchangeIsUniswap, setExchangeIsUniswap] = React.useState<boolean>(false); // false = Coinbase
  const [globalTradingOn, setGlobalTradingOn] = React.useState<boolean>(false);
  const [maxDailyAbsPnl, setMaxDailyAbsPnl] = React.useState<number>(50);
  const [currentPnl, setCurrentPnl] = React.useState<number>(0); // placeholder – would come from a service
  const [notice, setNotice] = React.useState<string>("");
  const [tokenToggles, setTokenToggles] = React.useState<Record<string, boolean>>(
    () => Object.fromEntries(TOKENS.map((t) => [t.symbol, false]))
  );
  const updatePrice = usePriceStore((s) => s.updatePrice);
  const updateCandles = useCandleStore((s) => s.updateCandles);
  // keep a ref to the websocket so we can close it on unmount
  const wsRef = React.useRef<WebSocket | null>(null);

  React.useEffect(() => {
    const url = "ws://localhost:8080/ws";
    const ws = new WebSocket(url);
    wsRef.current = ws;
    ws.onmessage = (event) => {
      const raw = JSON.parse(event.data);

      if (isPriceMessage(raw)) {
        updatePrice(raw.symbol, raw.price);
      } else if (isCandleMessage(raw)) {
        updateCandles(raw.symbol, raw);
      } else {
        console.warn("Unknown message", raw);
      }
    };
    ws.onopen = () => {
      console.log("[WS] connected to", url);
      for (const token of TOKENS) {
        ws.send(JSON.stringify({ type: "subscribe", symbols: tokens.map((t) => t.symbol) }));
      }
    };
    ws.onclose = () => console.log("[WS] closed");
    ws.onerror = (e) => console.error("[WS] error", e);

    return () => ws.close();
  }, [updatePrice, updateCandles]);

  const isThresholdMet = maxDailyAbsPnl > 0 && Math.abs(currentPnl) >= maxDailyAbsPnl;

  // --- Handlers (placeholders instead of real services) -----------------------
  const handlePhilosophyChange = (val: Philosophy) => {
    setPhilosophy(val);
    // Placeholder: pretend to call a state service to set the strategy
    console.log("[placeholder] setTradingPhilosophy:", val);
  };

  const handleExchangeToggle = (newValue: boolean) => {
    setExchangeIsUniswap(newValue);
    console.log("[placeholder] setPreferredExchange:", newValue ? "Uniswap" : "Coinbase");
  };

  const handleGlobalTradingToggle = (newValue: boolean) => {
    if (newValue && isThresholdMet) {
      setNotice("Threshold met – cannot start trading. (placeholder)");
      console.log("[placeholder] preventStart: threshold met");
      return;
    }
    setNotice("");
    setGlobalTradingOn(newValue);
    console.log("[placeholder] setGlobalTrading:", newValue);
  };

  const handleTokenToggle = (symbol: string, newValue: boolean) => {
    setTokenToggles((prev) => ({ ...prev, [symbol]: newValue }));
    console.log("[placeholder] setTokenTrading:", symbol, newValue);
  };

  const addSymbol = (symbol: string) => {
    setTokens([...TOKENS, { symbol, containerBg: "bg-gradient-to-br from-sky-50 to-sky-100" }]);
    wsRef.current?.send(JSON.stringify({ type: "subscribe", symbol: symbol }));
  };

  return (
    <div className="min-h-screen w-full relative overflow-hidden text-slate-800 p-8">
      {/* Page background with diagonal split (blue left, pink right) */}
      <div className="absolute inset-0">
        <div
          className="absolute inset-0 bg-gradient-to-br from-blue-50 to-blue-200"
          style={{ clipPath: "polygon(0% 0%, 55% 0%, 55% 100%, 0% 100%)" }}
        />
        <div
          className="absolute inset-0 bg-gradient-to-br from-pink-50 to-pink-200"
          style={{ clipPath: "polygon(55% 0%, 100% 0%, 100% 100%, 45% 100%)" }}
        />
      </div>
      <main className="relative z-10 mx-auto w-full max-w-6xl space-y-8">
        {/* Header */}
        <div className="flex items-baseline justify-between">
          <h1 className="text-2xl font-semibold tracking-tight text-slate-700">Algo Trading Console</h1>
          <span className="text-sm text-slate-500">Pastel theme • UI only (placeholders)</span>
        </div>

        {/* Controls Row */}
        <div className="grid gap-6 md:grid-cols-3">
          {/* Philosophy selector */}
          <div className="rounded-xl bg-white/70 p-4 shadow-sm ring-1 ring-slate-200">
            <RadioGroup<Philosophy>
              legend="Trading philosophy"
              value={philosophy}
              onChange={handlePhilosophyChange}
              options={[
                { value: "trend", label: "Trend Following", color: "blue" },
                { value: "mean", label: "Mean Reversion", color: "green" },
                { value: "arbitrage", label: "Arbitrage", color: "purple" },
                { value: "momentum", label: "Momentum", color: "orange" },
              ]}
            />
          </div>

          {/* Exchange toggle */}
          <div className="flex items-center justify-between gap-4 rounded-xl bg-white/70 p-4 shadow-sm ring-1 ring-slate-200">
            <div>
              <div className="text-sm font-medium text-slate-700">Exchange</div>
            </div>
            <Toggle
              optionA="CB"
              optionB="Uni"
              value={exchangeIsUniswap}
              onChange={handleExchangeToggle}
              inactiveBgClass="bg-gradient-to-r from-sky-100 to-sky-200"
              activeBgClass="bg-gradient-to-r from-pink-100 to-pink-200"
              thumbClass="bg-white"
              activeLabelClass="text-pink-900"
              inactiveLabelClass="text-sky-900"
            />
          </div>

          {/* Global Start/Stop with threshold check */}
          <div className="flex items-center justify-between gap-4 rounded-xl bg-white/70 p-4 shadow-sm ring-1 ring-slate-200">
            <div>
              <div className="text-sm font-medium text-slate-700">Trading Activity</div>
            </div>
            <Toggle
              value={globalTradingOn}
              onChange={handleGlobalTradingToggle}
              inactiveBgClass="bg-gradient-to-r from-slate-100 to-slate-200"
              activeBgClass="bg-gradient-to-r from-emerald-100 to-emerald-200"
              thumbClass="bg-white"
              activeLabelClass="text-emerald-900"
              inactiveLabelClass="text-slate-700"
            />
          </div>
        </div>

        {/* Risk controls */}
        <div className="grid gap-6 md:grid-cols-3">
          <div className="rounded-xl bg-white/70 p-4 shadow-sm ring-1 ring-slate-200">
            <div className="mb-2 text-sm font-medium text-slate-700">Max daily P/L (abs)</div>
            <Slider value={maxDailyAbsPnl} onChange={setMaxDailyAbsPnl} filledColor="bg-emerald-300" thumbColor="#ffffff" min={0} max={200} />
            <div className="mt-2 flex items-center justify-between text-xs text-slate-600">
              <span>Threshold: ${""}{maxDailyAbsPnl}</span>
              <span>Current P/L: ${""}{currentPnl} <span className="text-slate-400">(placeholder)</span></span>
            </div>
            {isThresholdMet && (
              <div className="mt-2 rounded-md bg-rose-50 px-2 py-1 text-xs text-rose-700 ring-1 ring-rose-200">
                Threshold met – starting is blocked.
              </div>
            )}
          </div>

          <div className="rounded-xl bg-white/70 p-4 shadow-sm ring-1 ring-slate-200">
            <div className="text-sm font-medium text-slate-700">Notes</div>
            <p className="mt-1 text-xs text-slate-600">
              This UI is wired to placeholders. Replace console logs with service calls for prices, state,
              and execution when backend is ready.
            </p>
            {notice && (
              <div className="mt-2 rounded-md bg-amber-50 px-2 py-1 text-xs text-amber-800 ring-1 ring-amber-200">{notice}</div>
            )}
          </div>

          <div className="rounded-xl bg-white/70 p-4 shadow-sm ring-1 ring-slate-200">
            <div className="text-sm font-medium text-slate-700">Debug (placeholder)
            </div>
            <div className="mt-1 flex items-center gap-2 text-xs text-slate-600">
              <button
                className="rounded-md bg-slate-200 px-2 py-1 text-slate-700 hover:bg-slate-300"
                onClick={() => setCurrentPnl((v) => v + 10)}
              >
                +$10 P/L
              </button>
              <button
                className="rounded-md bg-slate-200 px-2 py-1 text-slate-700 hover:bg-slate-300"
                onClick={() => setCurrentPnl((v) => Math.max(0, v - 10))}
              >
                -$10 P/L
              </button>
              <button
                className="rounded-md bg-slate-200 px-2 py-1 text-slate-700 hover:bg-slate-300"
                onClick={() => setCurrentPnl(0)}
              >
                Reset P/L
              </button>
            </div>
          </div>
        </div>
        <div>
          <input type="text" placeholder="Enter a symbol" value={newToken} onChange={(e) => setNewToken(e.target.value)} />
          <button onClick={addSymbol(newToken)}>Add</button>
        </div>

        {/* Tickers + per-token toggles */}
        <div className="rounded-xl bg-white/60 p-4 shadow-sm ring-1 ring-slate-200">
          <div className="mb-3 text-sm font-medium text-slate-700">Market Prices</div>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-2">
            {TOKENS.map((t) => (
              <div key={t.symbol} className={`flex items-center justify-between gap-3 rounded-med p-3 ring-1 ring-slate-200 ${t.containerBg}`}>
                <div className="flex items-center gap-3">
                  <div className="w-12 text-xs font-semibold text-slate-600">{t.symbol}</div>
                  <PriceTicker id={Math.abs(t.symbol.split("").reduce((a, c) => a + c.charCodeAt(0), 0))} />
                </div>
                <Toggle
                  value={tokenToggles[t.symbol]}
                  onChange={(nv) => handleTokenToggle(t.symbol, nv)}
                  inactiveBgClass="bg-gradient-to-r from-slate-100 to-slate-200"
                  activeBgClass="bg-gradient-to-r from-emerald-100 to-emerald-200"
                  thumbClass="bg-white"
                  activeLabelClass="text-emerald-900"
                  inactiveLabelClass="text-slate-700"
                />
              </div>
            ))}
          </div>
        </div>
        <div>
          <h1>Coinbase Advanced Trade – live feed demo</h1>

          <div className="grid grid-cols-2 gap-8 max-w-5xl mx-auto">
            {TOKENS.map((sym) => (
              <TokenChart key={sym.symbol} symbol={sym.symbol} className="w-full h-full" />
            ))}
          </div>
        </div>
      </main>
    </div>
  );
}
