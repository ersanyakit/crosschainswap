/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import { useState, useEffect } from 'react';
import { ArrowDownLeft, ArrowUpRight, TrendingUp } from 'lucide-react';
import { OrderBook, MarketPair, OrderBookLevel } from '../types/trading';
import { formatPrice, formatQuantity } from '../utils/formatters';

type OrderBookDisplayMode = 'both' | 'bids' | 'asks';
type OrderBookGroupingDecimals = 8 | 6 | 4 | 2 | 0;
export type OrderBookSelection = {
  price: number;
  amount: number;
  total: number;
  bookSide: 'ASK' | 'BID';
};

interface OrderBookProps {
  orderBook: OrderBook;
  pair: MarketPair;
  onSelectPrice: (selection: OrderBookSelection) => void;
}

export default function OrderBookView({
  orderBook,
  pair,
  onSelectPrice,
}: OrderBookProps) {
  const [displayMode, setDisplayMode] = useState<OrderBookDisplayMode>('both');
  const [activeGrouping, setActiveGrouping] = useState<OrderBookGroupingDecimals>(8);
  const groupedBids = groupOrderBookLevels(orderBook.bids, 'bid', activeGrouping);
  const groupedAsks = groupOrderBookLevels(orderBook.asks, 'ask', activeGrouping);
  const bidsToRender = groupedBids;
  const asksToRender = [...groupedAsks].reverse(); // show higher prices on top

  const [blinkLast, setBlinkLast] = useState<'blink-up' | 'blink-down' | null>(null);
  const [prevPrice, setPrevPrice] = useState(pair.lastPrice);
  const showAsks = displayMode === 'both' || displayMode === 'asks';
  const showBids = displayMode === 'both' || displayMode === 'bids';

  useEffect(() => {
    if (pair.lastPrice > prevPrice) {
      setBlinkLast('blink-up');
      const timer = setTimeout(() => setBlinkLast(null), 500);
      setPrevPrice(pair.lastPrice);
      return () => clearTimeout(timer);
    } else if (pair.lastPrice < prevPrice) {
      setBlinkLast('blink-down');
      const timer = setTimeout(() => setBlinkLast(null), 500);
      setPrevPrice(pair.lastPrice);
      return () => clearTimeout(timer);
    }
  }, [pair.lastPrice, prevPrice]);

  return (
    <div className="flex h-[360px] w-full flex-col overflow-hidden rounded-lg border border-[#e1e4e8] bg-white text-gray-800 shadow-sm select-none dark:border-[#21262d] dark:bg-[#0c1015] dark:text-gray-100 sm:h-[430px] xl:h-full">
      
      {/* Mini Titlebar */}
      <div className="flex items-center justify-between px-3 py-2 bg-[#f6f8fa] dark:bg-[#0d1117] border-b border-[#e1e4e8] dark:border-[#21262d]">
        <span className="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 font-display flex items-center gap-1.5">
          <TrendingUp className="w-3.5 h-3.5 text-accent-1" />
          Order Book
        </span>
        <div className="flex items-center gap-2">
          <div className="flex h-6 items-center rounded-md border border-[#e1e4e8] bg-white p-0.5 dark:border-[#21262d] dark:bg-[#0d1117]">
            <OrderBookModeButton
              mode="both"
              activeMode={displayMode}
              label="Show buy and sell orders"
              onClick={setDisplayMode}
            />
            <OrderBookModeButton
              mode="bids"
              activeMode={displayMode}
              label="Show buy orders only"
              onClick={setDisplayMode}
            />
            <OrderBookModeButton
              mode="asks"
              activeMode={displayMode}
              label="Show sell orders only"
              onClick={setDisplayMode}
            />
          </div>

          {/* Depth grouping selector */}
          <select
            value={activeGrouping}
            onChange={(e) => setActiveGrouping(Number(e.target.value) as OrderBookGroupingDecimals)}
            className="text-[10px] font-mono px-1.5 py-0.5 rounded border border-[#e1e4e8] dark:border-[#21262d] bg-white dark:bg-[#0d1117] text-gray-600 dark:text-gray-400 focus:outline-none focus:border-accent-1 cursor-pointer"
          >
            <option value="8">Decimals: 8</option>
            <option value="6">Decimals: 6</option>
            <option value="4">Decimals: 4</option>
            <option value="2">Decimals: 2</option>
            <option value="0">Decimals: 0</option>
          </select>
        </div>
      </div>

      {/* Grid Header */}
      <div className="grid grid-cols-3 px-3 py-1 text-[10px] uppercase font-mono text-gray-400 border-b border-[#e1e4e8]/50 dark:border-[#21262d]/50 bg-gray-50/50 dark:bg-transparent">
        <div>Price ({pair.quoteAsset})</div>
        <div className="text-right">Size ({pair.baseAsset})</div>
        <div className="text-right">Total ({pair.quoteAsset})</div>
      </div>

      {/* Layout Containers */}
      <div className="flex-1 flex flex-col justify-between overflow-y-auto font-mono text-[11px] leading-relaxed">
        {/* ASKS (SELL ORDERS) - Placed top */}
        {showAsks && (
          <div className={`flex flex-col justify-end flex-1 ${displayMode === 'asks' ? 'min-h-0' : 'min-h-[90px]'}`}>
            {asksToRender.length === 0 ? (
              <div className="flex-1 flex items-center justify-center px-3 text-center text-[10px] uppercase tracking-wide text-gray-400 dark:text-gray-500">
                No sell levels
              </div>
            ) : asksToRender.map((ask, idx) => {
              const levelPrice = ask.price;
              return (
                <div
                  key={levelKey('ask', ask, idx)}
                  onClick={() => onSelectPrice({
                    price: levelPrice,
                    amount: ask.cumulativeAmount,
                    total: ask.cumulativeTotal,
                    bookSide: 'ASK',
                  })}
                  className="relative grid grid-cols-3 px-3 py-0.5 cursor-pointer hover:bg-surface-3 transition-colors group"
                >
                  {/* Visual bar width representing depth */}
                  <div
                    className="absolute right-0 top-0 bottom-0 bg-trade-red/5 dark:bg-trade-red-bg pointer-events-none"
                    style={{ width: `${Math.min(100, ask.depthPercent)}%` }}
                  />

                  <div className="text-trade-red font-medium z-10 group-hover:underline">
                    {formatPrice(levelPrice, activeGrouping, activeGrouping)}
                  </div>
                  <div className="text-right text-gray-700 dark:text-gray-300 z-10">
                    {formatQuantity(ask.amount, 8)}
                  </div>
                  <div className="text-right text-gray-500 dark:text-gray-400 z-10">
                    {formatQuantity(ask.cumulativeTotal, 8)}
                  </div>
                </div>
              );
            })}
          </div>
        )}

        {/* SPREAD WIDGET (CENTER BLOCK) */}
        <div className={`py-1.5 px-3 border-y border-[#e1e4e8] dark:border-[#21262d] bg-[#f9fafc] dark:bg-[#070b0f] flex items-center justify-between text-xs transition-colors duration-300 ${
          blinkLast === 'blink-up' ? 'animate-blink-up' : blinkLast === 'blink-down' ? 'animate-blink-down' : ''
        }`}>
          <div className="flex items-center gap-1.5">
            <span className={`text-sm font-bold flex items-center ${pair.change24h >= 0 ? 'text-trade-green' : 'text-trade-red'}`}>
              {pair.change24h >= 0 ? (
                <ArrowUpRight className="w-4 h-4 text-trade-green mr-0.5 shrink-0" />
              ) : (
                <ArrowDownLeft className="w-4 h-4 text-trade-red mr-0.5 shrink-0" />
              )}
              {formatPrice(pair.lastPrice)}
            </span>
          </div>

          <div className="text-[10px] text-right font-mono text-gray-500">
            <span className="block">Spread: {orderBook.spread.toFixed(2)} ({orderBook.spreadPercent.toFixed(3)}%)</span>
          </div>
        </div>

        {/* BIDS (BUY ORDERS) - Placed bottom */}
        {showBids && (
          <div className={`flex flex-col justify-start flex-1 ${displayMode === 'bids' ? 'min-h-0' : 'min-h-[90px]'}`}>
            {bidsToRender.length === 0 ? (
              <div className="flex-1 flex items-center justify-center px-3 text-center text-[10px] uppercase tracking-wide text-gray-400 dark:text-gray-500">
                No buy levels
              </div>
            ) : bidsToRender.map((bid, idx) => {
              const levelPrice = bid.price;
              return (
                <div
                  key={levelKey('bid', bid, idx)}
                  onClick={() => onSelectPrice({
                    price: levelPrice,
                    amount: bid.cumulativeAmount,
                    total: bid.cumulativeTotal,
                    bookSide: 'BID',
                  })}
                  className="relative grid grid-cols-3 px-3 py-0.5 cursor-pointer hover:bg-surface-3 transition-colors group"
                >
                  {/* Visual bar width representing depth */}
                  <div
                    className="absolute right-0 top-0 bottom-0 bg-trade-green/5 dark:bg-trade-green-bg pointer-events-none"
                    style={{ width: `${Math.min(100, bid.depthPercent)}%` }}
                  />

                  <div className="text-trade-green font-medium z-10 group-hover:underline">
                    {formatPrice(levelPrice, activeGrouping, activeGrouping)}
                  </div>
                  <div className="text-right text-gray-700 dark:text-gray-300 z-10">
                    {formatQuantity(bid.amount, 8)}
                  </div>
                  <div className="text-right text-gray-500 dark:text-gray-400 z-10">
                    {formatQuantity(bid.cumulativeTotal, 8)}
                  </div>
                </div>
              );
            })}
          </div>
        )}

      </div>

    </div>
  );
}

function OrderBookModeButton({
  mode,
  activeMode,
  label,
  onClick,
}: {
  mode: OrderBookDisplayMode;
  activeMode: OrderBookDisplayMode;
  label: string;
  onClick: (mode: OrderBookDisplayMode) => void;
}) {
  const isActive = mode === activeMode;
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      aria-pressed={isActive}
      onClick={() => onClick(mode)}
      className={`flex h-5 w-5 items-center justify-center rounded transition-colors ${
        isActive
          ? 'bg-gray-100 text-gray-900 shadow-sm ring-1 ring-[#d2d6dc] dark:bg-[#18202a] dark:text-gray-100 dark:ring-[#303946]'
          : 'text-gray-400 hover:bg-gray-50 hover:text-gray-700 dark:hover:bg-[#151b23] dark:hover:text-gray-200'
      }`}
    >
      <OrderBookModeIcon mode={mode} />
    </button>
  );
}

function OrderBookModeIcon({ mode }: { mode: OrderBookDisplayMode }) {
  if (mode === 'bids') {
    return (
      <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
        <path d="M2.66663 2.66699L7.33329 2.66699L7.33329 13.3337L2.66663 13.3337L2.66663 2.66699Z" fill="#049769" />
        <path fillRule="evenodd" clipRule="evenodd" d="M8.66663 2.66699L13.3333 2.66699L13.3333 5.33366L8.66663 5.33366L8.66663 2.66699ZM8.66663 6.66699L13.3333 6.66699L13.3333 9.33366L8.66663 9.33366L8.66663 6.66699ZM13.3333 10.667L8.66663 10.667L8.66663 13.3337L13.3333 13.3337L13.3333 10.667Z" fill="#d2d6dc" />
      </svg>
    );
  }

  if (mode === 'asks') {
    return (
      <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
        <path d="M2.66663 2.66699L7.33329 2.66699L7.33329 13.3337L2.66663 13.3337L2.66663 2.66699Z" fill="#dc2979" />
        <path fillRule="evenodd" clipRule="evenodd" d="M8.66663 2.66699L13.3333 2.66699L13.3333 5.33366L8.66663 5.33366L8.66663 2.66699ZM8.66663 6.66699L13.3333 6.66699L13.3333 9.33366L8.66663 9.33366L8.66663 6.66699ZM13.3333 10.667L8.66663 10.667L8.66663 13.3337L13.3333 13.3337L13.3333 10.667Z" fill="#d2d6dc" />
      </svg>
    );
  }

  return (
    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
      <path d="M2.66663 2.66699L7.33329 2.66699L7.33329 7.33366L2.66663 7.33366L2.66663 2.66699Z" fill="#dc2979" />
      <path d="M2.66663 8.66699L7.33329 8.66699L7.33329 13.3337L2.66663 13.3337L2.66663 8.66699Z" fill="#049769" />
      <path fillRule="evenodd" clipRule="evenodd" d="M8.66663 2.66699L13.3333 2.66699L13.3333 5.33366L8.66663 5.33366L8.66663 2.66699ZM8.66663 6.66699L13.3333 6.66699L13.3333 9.33366L8.66663 9.33366L8.66663 6.66699ZM13.3333 10.667L8.66663 10.667L8.66663 13.3337L13.3333 13.3337L13.3333 10.667Z" fill="#d2d6dc" />
    </svg>
  );
}

function groupOrderBookLevels(
  levels: OrderBookLevel[],
  side: 'bid' | 'ask',
  decimals: OrderBookGroupingDecimals,
): OrderBookLevel[] {
  const factor = 10 ** decimals;
  const grouped = new Map<number, OrderBookLevel>();

  levels.forEach((level) => {
    if (!Number.isFinite(level.amount) || level.amount < 0.000000005) return;
    const price = groupPrice(level.price, side, factor);
    if (price <= 0) return;
    const existing = grouped.get(price);
    const amount = level.amount;

    if (existing) {
      existing.amount += amount;
      existing.total += price * amount;
      return;
    }

    grouped.set(price, {
      price,
      amount,
      total: price * amount,
      cumulativeAmount: 0,
      cumulativeTotal: 0,
      depthPercent: 0,
    });
  });

  const sorted = Array.from(grouped.values()).sort((left, right) => (
    side === 'bid' ? right.price - left.price : left.price - right.price
  ));

  let cumulativeAmount = 0;
  let cumulativeTotal = 0;
  sorted.forEach((level) => {
    cumulativeAmount += level.amount;
    cumulativeTotal += level.total;
    level.cumulativeAmount = cumulativeAmount;
    level.cumulativeTotal = cumulativeTotal;
  });

  const maxTotal = sorted[sorted.length - 1]?.cumulativeTotal || 1;
  sorted.forEach((level) => {
    level.depthPercent = (level.cumulativeTotal / maxTotal) * 100;
  });

  return sorted;
}

function groupPrice(price: number, side: 'bid' | 'ask', factor: number): number {
  if (!Number.isFinite(price) || price <= 0) return 0;
  const scaled = price * factor;
  const grouped = side === 'bid'
    ? Math.floor(scaled + Number.EPSILON)
    : Math.ceil(scaled - Number.EPSILON);
  return grouped / factor;
}

function levelKey(side: 'ask' | 'bid', level: OrderBookLevel, index: number): string {
  if (Number.isFinite(level.price) && level.price > 0) {
    return `${side}-${level.price.toFixed(12)}`;
  }
  return `${side}-${index}`;
}
