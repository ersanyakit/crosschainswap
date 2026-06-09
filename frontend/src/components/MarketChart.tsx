/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import { useEffect, useMemo, useRef, useState } from 'react';
import { Activity, Eye } from 'lucide-react';
import { dispose, init, type Chart, type DataLoader, type KLineData, type Period } from 'klinecharts';
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
  const chartHostRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<Chart | null>(null);
  const [showMA, setShowMA] = useState(true);
  const timeframes: Timeframe[] = ['1m', '5m', '15m', '1h', '4h', '1d'];
  const klineData = useMemo(() => candles.map(candleToKLineData), [candles]);
  const latestCandle = candles[candles.length - 1] || null;
  const displayPrice = latestCandle?.close ?? pair.lastPrice;
  const displayHigh = latestCandle?.high ?? pair.high24h;
  const displayLow = latestCandle?.low ?? pair.low24h;
  const displayVolume = latestCandle?.volume ?? pair.volume24h;
  const candleIsUp = latestCandle ? latestCandle.close >= latestCandle.open : pair.change24h >= 0;
  const pricePrecision = pricePrecisionFor(pair, candles);

  useEffect(() => {
    if (!chartHostRef.current || chartRef.current) return;

    const chart = init(chartHostRef.current, {
      locale: 'en-US',
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      layout: {
        basicParams: {
          yAxisPosition: 'right',
          yAxisInside: false,
          barSpaceLimitMin: 3,
          barSpaceLimitMax: 18,
        },
        panes: [
          {
            type: 'candle',
            content: [],
            options: { id: 'candle_pane', minHeight: 220 },
          },
          {
            type: 'indicator',
            content: ['VOL'],
            options: { id: 'volume_pane', height: 74, minHeight: 56 },
          },
          { type: 'xAxis', options: { id: 'x_axis_pane', height: 24 } },
        ],
      },
      styles: chartStyles,
      formatter: {
        formatDate: ({ dateTimeFormat, timestamp, template }) => {
          if (template.includes('HH:mm')) {
            return dateTimeFormat.format(new Date(timestamp));
          }
          return dateTimeFormat.format(new Date(timestamp));
        },
      },
      zoomAnchor: 'last_bar',
    });

    if (!chart) return;
    chartRef.current = chart;
    chart.setBarSpace(7);
    chart.setScrollEnabled(true);
    chart.setZoomEnabled(true);

    return () => {
      dispose(chart);
      chartRef.current = null;
    };
  }, []);

  useEffect(() => {
    const chart = chartRef.current;
    if (!chart) return;

    const loader: DataLoader = {
      getBars: ({ callback }) => {
        callback(klineData, { backward: false, forward: false });
      },
    };

    chart.setDataLoader(loader);
    chart.setSymbol({
      ticker: pair.symbol,
      pricePrecision,
      volumePrecision: 6,
    });
    chart.setPeriod(periodForTimeframe(timeframe));
    chart.removeIndicator({ name: 'MA' });
    if (showMA) {
      chart.createIndicator('MA', { pane: { id: 'candle_pane' }, isStack: true });
    }
    chart.scrollToRealTime(0);
  }, [klineData, pair.symbol, pricePrecision, timeframe, showMA]);

  return (
    <div className="relative flex h-full min-h-[430px] flex-col overflow-hidden rounded-lg border border-[#e1e4e8] bg-white text-gray-800 shadow-sm dark:border-[#21262d] dark:bg-[#0c1015] dark:text-gray-100 xl:min-h-0">
      <div className="flex flex-wrap items-center justify-between gap-4 p-3 border-b border-[#e1e4e8] dark:border-[#21262d] bg-[#fafbfc] dark:bg-[#090d12] text-xs font-mono select-none">
        <div className="flex items-center gap-3">
          <div className="font-display font-medium text-sm text-gray-950 dark:text-gray-50 flex items-center gap-2">
            <span className="flex -space-x-1">
              <AssetIcon symbol={pair.baseAsset} iconURL={assetMetadata[pair.baseAsset]?.icon_url} size="sm" />
              <AssetIcon symbol={pair.quoteAsset} iconURL={assetMetadata[pair.quoteAsset]?.icon_url} size="sm" />
            </span>
            {pair.symbol}
          </div>
          <div className={`font-mono font-bold text-base ${candleIsUp ? 'text-trade-green' : 'text-trade-red'}`}>
            {formatPrice(displayPrice)}
          </div>
          <div className={`text-[11px] font-semibold flex items-center ${pair.change24h >= 0 ? 'text-trade-green' : 'text-trade-red'}`}>
            {pair.change24h >= 0 ? '+' : ''}{pair.change24h.toFixed(2)}%
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-x-5 gap-y-1 text-[11px] text-gray-500 dark:text-gray-400">
          <Metric label="High" value={formatPrice(displayHigh)} />
          <Metric label="Low" value={formatPrice(displayLow)} />
          <Metric label={`Vol (${pair.baseAsset})`} value={displayVolume.toLocaleString(undefined, { maximumFractionDigits: 2 })} />
          <Metric label="Candles" value={String(candles.length)} accent />
        </div>
      </div>

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

        <div className="flex items-center gap-3 shrink-0">
          <label className="flex items-center gap-1.5 text-[11px] cursor-pointer">
            <input
              type="checkbox"
              checked={showMA}
              onChange={() => setShowMA((value) => !value)}
              className="accent-accent-1 rounded border-gray-300 dark:border-gray-700"
            />
            <span>MA</span>
          </label>
          <span className="text-gray-300 dark:text-gray-700">|</span>
          <span className="text-[10px] bg-slate-100 dark:bg-slate-800/80 px-1.5 py-0.5 rounded flex items-center gap-1 font-medium text-gray-600 dark:text-gray-300">
            <Eye className="w-3 h-3 text-accent-1" />
            KLine
          </span>
        </div>
      </div>

      <div className="relative flex-1 min-h-[280px] bg-white dark:bg-[#070b0f] xl:min-h-0">
        <div ref={chartHostRef} className="absolute inset-0" />
        {candles.length === 0 && (
          <div className="absolute inset-x-0 top-3 flex justify-center pointer-events-none">
            <div className="flex items-center gap-2 rounded border border-[#e1e4e8] dark:border-[#263241] bg-white/90 dark:bg-[#0d1117]/90 px-2.5 py-1 text-[10px] font-mono text-gray-500 dark:text-gray-400 shadow-sm">
              <Activity className="w-3 h-3 text-accent-1" />
              No KLINE data for {pair.symbol} {timeframe}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function Metric({ label, value, accent = false }: { label: string; value: string; accent?: boolean }) {
  return (
    <div>
      <span className="block text-[9px] uppercase tracking-wider text-gray-400">{label}</span>
      <span className={`font-semibold ${accent ? 'text-accent-1' : 'text-gray-700 dark:text-gray-200'}`}>
        {value}
      </span>
    </div>
  );
}

function candleToKLineData(candle: Candle): KLineData {
  return {
    timestamp: candle.time * 1000,
    open: candle.open,
    high: candle.high,
    low: candle.low,
    close: candle.close,
    volume: candle.volume,
    turnover: candle.close * candle.volume,
  };
}

function periodForTimeframe(timeframe: Timeframe): Period {
  switch (timeframe) {
    case '1m':
      return { type: 'minute', span: 1 };
    case '5m':
      return { type: 'minute', span: 5 };
    case '15m':
      return { type: 'minute', span: 15 };
    case '1h':
      return { type: 'hour', span: 1 };
    case '4h':
      return { type: 'hour', span: 4 };
    case '1d':
      return { type: 'day', span: 1 };
  }
}

function pricePrecisionFor(pair: MarketPair, candles: Candle[]): number {
  const values = [
    pair.lastPrice,
    ...candles.flatMap((candle) => [candle.open, candle.high, candle.low, candle.close]),
  ].filter((value) => Number.isFinite(value) && value > 0);
  const min = Math.min(...values, pair.lastPrice || 1);
  if (min >= 100) return 2;
  if (min >= 1) return 4;
  if (min >= 0.01) return 6;
  if (min >= 0.0001) return 8;
  return 12;
}

const chartStyles = {
  grid: {
    horizontal: {
      show: true,
      style: 'dashed' as const,
      size: 1,
      color: '#263241',
      dashedValue: [2, 2],
    },
    vertical: {
      show: true,
      style: 'dashed' as const,
      size: 1,
      color: '#1f2937',
      dashedValue: [2, 3],
    },
  },
  candle: {
    type: 'candle_solid' as const,
    bar: {
      compareRule: 'current_open' as const,
      upColor: '#10b981',
      downColor: '#f6465d',
      noChangeColor: '#7e8c9a',
      upBorderColor: '#10b981',
      downBorderColor: '#f6465d',
      noChangeBorderColor: '#7e8c9a',
      upWickColor: '#10b981',
      downWickColor: '#f6465d',
      noChangeWickColor: '#7e8c9a',
    },
    priceMark: {
      high: { color: '#9ca3af' },
      low: { color: '#9ca3af' },
      last: {
        show: true,
        line: { show: true, style: 'dashed' as const, size: 1, dashedValue: [3, 3] },
        text: { show: true, color: '#ffffff', backgroundColor: '#1677ff' },
      },
    },
  },
  indicator: {
    bars: [
      {
        style: 'fill' as const,
        color: '#10b981',
        borderColor: '#10b981',
      },
      {
        style: 'fill' as const,
        color: '#f6465d',
        borderColor: '#f6465d',
      },
    ],
    lines: [
      { color: '#3b82f6', size: 1, style: 'solid' as const },
      { color: '#f59e0b', size: 1, style: 'solid' as const },
      { color: '#8b5cf6', size: 1, style: 'solid' as const },
    ],
  },
  xAxis: {
    axisLine: { show: true, color: '#263241' },
    tickLine: { show: false },
    tickText: { color: '#7e8c9a', size: 10 },
  },
  yAxis: {
    axisLine: { show: true, color: '#263241' },
    tickLine: { show: false },
    tickText: { color: '#7e8c9a', size: 10 },
  },
  separator: {
    size: 1,
    color: '#21262d',
    fill: true,
    activeBackgroundColor: '#1677ff',
  },
  crosshair: {
    show: true,
    horizontal: {
      line: { show: true, style: 'dashed' as const, size: 1, color: '#1677ff', dashedValue: [3, 3] },
      text: { show: true, color: '#ffffff', backgroundColor: '#1677ff' },
    },
    vertical: {
      line: { show: true, style: 'dashed' as const, size: 1, color: '#1677ff', dashedValue: [3, 3] },
      text: { show: true, color: '#ffffff', backgroundColor: '#343a40' },
    },
  },
};
