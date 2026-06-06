/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import React, { useState, useRef, useEffect } from 'react';
import { Eye, TrendingUp, Sliders, ChevronDown } from 'lucide-react';
import { Candle, Timeframe, MarketPair } from '../types/trading';
import { formatPrice } from '../utils/formatters';
import { type AssetInfo } from '../services/exchangeService';
import AssetIcon from './AssetIcon';

interface MarketChartProps {
  pair: MarketPair;
  candles: Candle[];
  timeframe: Timeframe;
  setTimeframe: (tf: Timeframe) => void;
  assetMetadata: Record<string, AssetInfo>;
}

export default function MarketChart({
  pair,
  candles,
  timeframe,
  setTimeframe,
  assetMetadata,
}: MarketChartProps) {
  const [hoveredCandle, setHoveredCandle] = useState<Candle | null>(null);
  const [crosshair, setCrosshair] = useState<{ x: number; y: number; price: number; time: string } | null>(null);
  const [showEMA, setShowEMA] = useState(true);
  const containerRef = useRef<HTMLDivElement>(null);
  const [dimensions, setDimensions] = useState({ width: 600, height: 350 });

  // Update dimensions based on parent container sizes
  useEffect(() => {
    if (!containerRef.current) return;
    const resizeObserver = new ResizeObserver((entries) => {
      for (let entry of entries) {
        if (entry.contentRect) {
          // Grant padding margins
          setDimensions({
            width: Math.max(300, entry.contentRect.width),
            height: Math.max(220, entry.contentRect.height),
          });
        }
      }
    });
    resizeObserver.observe(containerRef.current);
    return () => resizeObserver.disconnect();
  }, []);

  const timeframes: Timeframe[] = ['1m', '5m', '15m', '1h', '4h', '1d'];

  if (candles.length === 0) {
    return (
      <div className="w-full h-80 flex flex-col items-center justify-center text-gray-400 font-mono text-xs">
        <Sliders className="w-6 h-6 animate-spin mb-2 text-accent-1" />
        Prefetching historical blocks...
      </div>
    );
  }

  // Calculate EMA helper
  const calculateEMA = (data: Candle[], period: number): number[] => {
    const k = 2 / (period + 1);
    const ema: number[] = [];
    if (data.length === 0) return [];
    
    let currentEma = data[0].close;
    ema.push(currentEma);
    
    for (let i = 1; i < data.length; i++) {
      currentEma = data[i].close * k + currentEma * (1 - k);
      ema.push(currentEma);
    }
    return ema;
  };

  const ema9 = calculateEMA(candles, 9);
  const ema21 = calculateEMA(candles, 21);

  // SVG dimensions configs
  const paddingLeft = 10;
  const paddingRight = 65; // Price scale container
  const paddingTop = 25;
  const paddingBottom = 25; // Time x-scale
  
  const plotWidth = dimensions.width - paddingLeft - paddingRight;
  const plotHeight = dimensions.height - paddingTop - paddingBottom;

  // Extents
  const closes = candles.map(c => c.close);
  const highs = candles.map(c => c.high);
  const lows = candles.map(c => c.low);
  const maxPrice = Math.max(...highs) * 1.002;
  const minPrice = Math.min(...lows) * 0.998;
  const priceRange = maxPrice - minPrice;

  const maxVolume = Math.max(...candles.map(c => c.volume));

  // Render variables
  const numCandles = candles.length;
  const candleWidth = plotWidth / numCandles;

  // Coordinates converters
  const getX = (index: number) => paddingLeft + index * candleWidth + candleWidth / 2;
  const getY = (price: number) => paddingTop + plotHeight - ((price - minPrice) / priceRange) * plotHeight;
  const getVolHeight = (volume: number) => (volume / maxVolume) * (plotHeight * 0.15); // max 15% chart height

  // Interactive crosshair handling
  const handleMouseMove = (e: React.MouseEvent<SVGSVGElement, MouseEvent>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;

    // Convert x back to candle index
    const relativeX = x - paddingLeft;
    const index = Math.min(
      numCandles - 1,
      Math.max(0, Math.floor(relativeX / candleWidth))
    );

    const candle = candles[index];
    if (candle) {
      setHoveredCandle(candle);

      // Convert y back to price
      const relativeY = y - paddingTop;
      const pctY = 1 - relativeY / plotHeight;
      const hoverPrice = minPrice + pctY * priceRange;

      const candleDate = new Date(candle.time * 1000);
      const timeStr = timeframe.endsWith('d')
        ? candleDate.toLocaleDateString()
        : candleDate.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' });

      setCrosshair({
        x: getX(index),
        y,
        price: hoverPrice,
        time: timeStr,
      });
    }
  };

  const handleMouseLeave = () => {
    setHoveredCandle(null);
    setCrosshair(null);
  };

  const candleToDisplay = hoveredCandle || candles[candles.length - 1];
  const isUpClose = candleToDisplay.close >= candleToDisplay.open;

  // Render price grid ticks (4 horizontal lines)
  const priceTicks = [0.15, 0.45, 0.75, 0.95].map(pct => minPrice + pct * priceRange);

  return (
    <div className="flex flex-col bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm overflow-hidden text-gray-800 dark:text-gray-100 relative">
      
      {/* 24h Ticker Summary Widget Bar */}
      <div className="flex flex-wrap items-center justify-between gap-4 p-3 border-b border-[#e1e4e8] dark:border-[#21262d] bg-[#fafbfc] dark:bg-[#090d12] text-xs font-mono select-none">
        <div className="flex items-center gap-3">
          <div className="font-display font-medium text-sm text-gray-950 dark:text-gray-50 flex items-center gap-2">
            <span className="flex -space-x-1">
              <AssetIcon symbol={pair.baseAsset} iconURL={assetMetadata[pair.baseAsset]?.icon_url} size="sm" />
              <AssetIcon symbol={pair.quoteAsset} iconURL={assetMetadata[pair.quoteAsset]?.icon_url} size="sm" />
            </span>
            {pair.symbol}
          </div>
          <div className={`font-mono font-bold text-base ${isUpClose ? 'text-trade-green' : 'text-trade-red'}`}>
            {formatPrice(pair.lastPrice)}
          </div>
          <div className={`text-[11px] font-semibold flex items-center ${pair.change24h >= 0 ? 'text-trade-green' : 'text-trade-red'}`}>
            {pair.change24h >= 0 ? '▲' : '▼'} {Math.abs(pair.change24h).toFixed(2)}%
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-x-5 gap-y-1 text-[11px] text-gray-500 dark:text-gray-400">
          <div>
            <span className="block text-[9px] uppercase tracking-wider text-gray-400">24h High</span>
            <span className="font-semibold text-gray-700 dark:text-gray-200">
              {formatPrice(pair.high24h)}
            </span>
          </div>
          <div>
            <span className="block text-[9px] uppercase tracking-wider text-gray-400">24h Low</span>
            <span className="font-semibold text-gray-700 dark:text-gray-200">
              {formatPrice(pair.low24h)}
            </span>
          </div>
          <div>
            <span className="block text-[9px] uppercase tracking-wider text-gray-400">24h Vol ({pair.baseAsset})</span>
            <span className="font-semibold text-gray-700 dark:text-gray-200">
              {pair.volume24h.toLocaleString(undefined, { maximumFractionDigits: 0 })}
            </span>
          </div>
          <div className="hidden md:block">
            <span className="block text-[9px] uppercase tracking-wider text-gray-400">Net Depth Liquidity</span>
            <span className="font-semibold text-accent-1 font-mono">
              ${pair.liquidity.toFixed(1)}M USD
            </span>
          </div>
        </div>
      </div>

      {/* Candlestick Control Settings Bar */}
      <div className="flex items-center justify-between px-3 py-1.5 bg-[#f6f8fa] dark:bg-[#0d1117] border-b border-[#e1e4e8] dark:border-[#21262d] text-xs font-mono text-gray-500">
        <div className="flex items-center gap-1 overflow-x-auto scrollbar-hide py-0.5">
          {timeframes.map((tf) => (
            <button
              key={tf}
              onClick={() => setTimeframe(tf)}
              className={`px-2 py-0.5 rounded text-[11px] font-medium transition-colors cursor-pointer ${
                timeframe === tf
                  ? 'bg-accent-1 text-white shadow-sm'
                  : 'hover:bg-surface-3 hover:text-gray-800 dark:hover:text-gray-200'
              }`}
            >
              {tf}
            </button>
          ))}
        </div>

        {/* Indicator Controls */}
        <div className="flex items-center gap-3 shrink-0">
          <label className="flex items-center gap-1.5 text-[11px] cursor-pointer">
            <input
              type="checkbox"
              checked={showEMA}
              onChange={() => setShowEMA(!showEMA)}
              className="accent-accent-1 rounded border-gray-300 dark:border-gray-700"
            />
            <span>EMA (9,21)</span>
          </label>
          <span className="text-gray-300 dark:text-gray-700">|</span>
          <span className="text-[10px] bg-slate-100 dark:bg-slate-800/80 px-1.5 py-0.5 rounded flex items-center gap-1 font-medium text-gray-600 dark:text-gray-300">
            <Eye className="w-3 h-3 text-accent-1" />
            Spot Core
          </span>
        </div>
      </div>

      {/* Dynamic OHLCV stats row floating inside the graph */}
      <div className="absolute top-[85px] left-3 z-10 flex flex-wrap gap-x-3 gap-y-0.5 pointer-events-none bg-white/80 dark:bg-slate-950/80 backdrop-blur-xs p-1.5 rounded-md border border-[#e1e4e8]/60 dark:border-[#21262d]/60 font-mono text-[10px] text-gray-500 shadow-xs">
        <span className="text-gray-700 dark:text-gray-300 font-semibold">{pair.symbol} ({timeframe})</span>
        <span>O:<span className={isUpClose ? 'text-trade-green' : 'text-trade-red'}> {formatPrice(candleToDisplay.open)}</span></span>
        <span>H:<span className="text-gray-800 dark:text-gray-200"> {formatPrice(candleToDisplay.high)}</span></span>
        <span>L:<span className="text-gray-800 dark:text-gray-200"> {formatPrice(candleToDisplay.low)}</span></span>
        <span>C:<span className={isUpClose ? 'text-trade-green' : 'text-trade-red'}> {formatPrice(candleToDisplay.close)}</span></span>
        <span>V:<span className="text-accent-1"> {candleToDisplay.volume.toFixed(2)}</span></span>
        {showEMA && (
          <>
            <span className="text-[#3b82f6]">EMA(9): {formatPrice(ema9[ema9.length - 1])}</span>
            <span className="text-[#f59e0b]">EMA(21): {formatPrice(ema21[ema21.length - 1])}</span>
          </>
        )}
      </div>

      {/* Main Graph Area */}
      <div ref={containerRef} className="flex-1 min-h-[220px] bg-white dark:bg-[#070b0f] relative cursor-crosshair">
        
        {/* SVG Drawing Canvas */}
        <svg
          width={dimensions.width}
          height={dimensions.height}
          onMouseMove={handleMouseMove}
          onMouseLeave={handleMouseLeave}
          className="block overflow-hidden"
        >
          {/* Horizontal Gridlines & Price Tick Markers */}
          {priceTicks.map((p, idx) => {
            const gy = getY(p);
            return (
              <g key={idx} className="opacity-40">
                <line
                  x1={paddingLeft}
                  y1={gy}
                  x2={dimensions.width - paddingRight}
                  y2={gy}
                  stroke="#e1e4e8"
                  strokeWidth="0.5"
                  className="dark:stroke-[#21262d]"
                  strokeDasharray="2,2"
                />
                <text
                  x={dimensions.width - paddingRight + 5}
                  y={gy + 4}
                  fill="#858585"
                  fontSize="9"
                  fontFamily="Google Sans"
                  textAnchor="start"
                >
                  {p.toLocaleString(undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 })}
                </text>
              </g>
            );
          })}

          {/* Render Volume Bars (drawn at the base behind candlesticks) */}
          {candles.map((candle, i) => {
            const cx = getX(i) - candleWidth / 2;
            const barWidth = Math.max(1, candleWidth - 1);
            const vh = getVolHeight(candle.volume);
            const vy = paddingTop + plotHeight - vh;
            const candleUp = candle.close >= candle.open;

            return (
              <rect
                key={`v-${i}`}
                x={cx}
                y={vy}
                width={barWidth}
                height={vh}
                fill={candleUp ? 'var(--color-trade-green)' : 'var(--color-trade-red)'}
                opacity="0.15"
              />
            );
          })}

          {/* Render EMA9 Indicator Path */}
          {showEMA && ema9.length > 1 && (
            <path
              d={ema9.map((val, idx) => `${idx === 0 ? 'M' : 'L'} ${getX(idx)} ${getY(val)}`).join(' ')}
              fill="none"
              stroke="#3b82f6"
              strokeWidth="1"
              opacity="0.8"
            />
          )}

          {/* Render EMA21 Indicator Path */}
          {showEMA && ema21.length > 1 && (
            <path
              d={ema21.map((val, idx) => `${idx === 0 ? 'M' : 'L'} ${getX(idx)} ${getY(val)}`).join(' ')}
              fill="none"
              stroke="#f59e0b"
              strokeWidth="1"
              opacity="0.8"
            />
          )}

          {/* Render Candlesticks (Wicks and Bodies) */}
          {candles.map((candle, i) => {
            const cx = getX(i);
            const cyOpen = getY(candle.open);
            const cyClose = getY(candle.close);
            const cyHigh = getY(candle.high);
            const cyLow = getY(candle.low);
            
            const isBullish = candle.close >= candle.open;
            const color = isBullish ? 'var(--color-trade-green)' : 'var(--color-trade-red)';
            
            const boxY = Math.min(cyOpen, cyClose);
            const boxH = Math.max(1.5, Math.abs(cyOpen - cyClose));
            const barWidth = Math.max(1, candleWidth - 2);

            return (
              <g key={`candle-${i}`}>
                {/* Wick line */}
                <line
                  x1={cx}
                  y1={cyHigh}
                  x2={cx}
                  y2={cyLow}
                  stroke={color}
                  strokeWidth="1.2"
                />
                {/* Real body */}
                <rect
                  x={cx - barWidth / 2}
                  y={boxY}
                  width={barWidth}
                  height={boxH}
                  fill={color}
                  stroke={color}
                  strokeWidth="0.5"
                />
              </g>
            );
          })}

          {/* Interactive Crosshair Layers */}
          {crosshair && (
            <g>
              {/* Vertical crosshair line */}
              <line
                x1={crosshair.x}
                y1={paddingTop}
                x2={crosshair.x}
                y2={paddingTop + plotHeight}
                stroke="var(--color-accent-1)"
                strokeWidth="0.8"
                strokeDasharray="3,3"
                opacity="0.7"
              />
              {/* Horizontal crosshair line */}
              <line
                x1={paddingLeft}
                y1={crosshair.y}
                x2={dimensions.width - paddingRight}
                y2={crosshair.y}
                stroke="var(--color-accent-1)"
                strokeWidth="0.8"
                strokeDasharray="3,3"
                opacity="0.7"
              />
              
              {/* Floating crosshair details on vertical/horizontal ticks */}
              {/* Price level on Y-Scale margin block */}
              <rect
                x={dimensions.width - paddingRight}
                y={crosshair.y - 8}
                width={paddingRight}
                height={16}
                fill="var(--color-accent-1)"
                rx="2"
              />
              <text
                x={dimensions.width - paddingRight + 5}
                y={crosshair.y + 3}
                fill="#ffffff"
                fontSize="8.5"
                fontFamily="Google Sans"
                fontWeight="bold"
              >
                {crosshair.price.toFixed(2)}
              </text>

              {/* Time tick on X-Scale base block */}
              <rect
                x={crosshair.x - 35}
                y={paddingTop + plotHeight}
                width={70}
                height={15}
                fill="#343a40"
                rx="2"
              />
              <text
                x={crosshair.x}
                y={paddingTop + plotHeight + 11}
                fill="#ffffff"
                fontSize="8"
                fontFamily="Google Sans"
                textAnchor="middle"
              >
                {crosshair.time}
              </text>
            </g>
          )}

          {/* Simple Time X-Scale Ticks (3 ticks spaced across sequence) */}
          {candles.length > 2 && [0.1, 0.5, 0.9].map((pct, i) => {
            const index = Math.floor(pct * numCandles);
            const candle = candles[index];
            if (!candle) return null;
            const cx = getX(index);
            const date = new Date(candle.time * 1000);
            const label = timeframe.endsWith('d') 
              ? date.toLocaleDateString(undefined, {month: 'short', day: 'numeric'})
              : date.toLocaleTimeString(undefined, {hour: '2-digit', minute: '2-digit'});

            return (
              <text
                key={`lbl-${i}`}
                x={cx}
                y={dimensions.height - 10}
                fill="#7e8c9a"
                fontSize="8.5"
                fontFamily="Google Sans"
                textAnchor="middle"
              >
                {label}
              </text>
            );
          })}
        </svg>
      </div>

    </div>
  );
}
