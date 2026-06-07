/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import { useState, useEffect } from 'react';
import { ArrowDownLeft, ArrowUpRight, TrendingUp } from 'lucide-react';
import { OrderBook, MarketPair, Order, OrderBookLevel } from '../types/trading';
import { formatPrice, formatQuantity } from '../utils/formatters';

interface OrderBookProps {
  orderBook: OrderBook;
  pair: MarketPair;
  onSelectPrice: (price: number) => void;
  myOpenOrders?: Order[];
  myRecentOrders?: Order[];
}

export default function OrderBookView({
  orderBook,
  pair,
  onSelectPrice,
  myOpenOrders = [],
  myRecentOrders = [],
}: OrderBookProps) {
  // Let's visual trim levels depending on screen sizes
  const bidsToRender = orderBook.bids.slice(0, 10);
  const asksToRender = orderBook.asks.slice(0, 10).reverse(); // show higher prices on top

  const [activeGrouping, setActiveGrouping] = useState<0.01 | 0.1 | 1.0>(0.01);
  const [blinkLast, setBlinkLast] = useState<'blink-up' | 'blink-down' | null>(null);
  const [prevPrice, setPrevPrice] = useState(pair.lastPrice);

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

  const visibleOrders = uniqueOrders([...myOpenOrders, ...myRecentOrders]).slice(0, 6);

  return (
    <div className="bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm flex flex-col h-[360px] sm:h-[430px] overflow-hidden text-gray-800 dark:text-gray-100 select-none">
      
      {/* Mini Titlebar */}
      <div className="flex items-center justify-between px-3 py-2 bg-[#f6f8fa] dark:bg-[#0d1117] border-b border-[#e1e4e8] dark:border-[#21262d]">
        <span className="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 font-display flex items-center gap-1.5">
          <TrendingUp className="w-3.5 h-3.5 text-accent-1" />
          Order Book
        </span>
        <div className="flex items-center gap-2">
          {/* Depth grouping selector */}
          <select
            value={activeGrouping}
            onChange={(e) => setActiveGrouping(Number(e.target.value) as any)}
            className="text-[10px] font-mono px-1.5 py-0.5 rounded border border-[#e1e4e8] dark:border-[#21262d] bg-white dark:bg-[#0d1117] text-gray-600 dark:text-gray-400 focus:outline-none focus:border-accent-1 cursor-pointer"
          >
            <option value="0.01">Group: 0.01</option>
            <option value="0.1">Group: 0.1</option>
            <option value="1.0">Group: 1.0</option>
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
        <div className="flex flex-col justify-end flex-1 min-h-[90px]">
          {asksToRender.length === 0 ? (
            <div className="flex-1 flex items-center justify-center px-3 text-center text-[10px] uppercase tracking-wide text-gray-400 dark:text-gray-500">
              No sell levels
            </div>
          ) : asksToRender.map((ask, idx) => {
            const levelPrice = ask.price;
            return (
              <div
                key={levelKey('ask', ask, idx)}
                onClick={() => onSelectPrice(levelPrice)}
                className="relative grid grid-cols-3 px-3 py-0.5 cursor-pointer hover:bg-surface-3 transition-colors group"
              >
                {/* Visual bar width representing depth */}
                <div
                  className="absolute right-0 top-0 bottom-0 bg-trade-red/5 dark:bg-trade-red-bg pointer-events-none"
                  style={{ width: `${Math.min(100, ask.depthPercent)}%` }}
                />
                
                <div className="text-trade-red font-medium z-10 group-hover:underline">
                  {formatPrice(levelPrice)}
                </div>
                <div className="text-right text-gray-700 dark:text-gray-300 z-10">
                  {formatQuantity(ask.amount)}
                </div>
                <div className="text-right text-gray-500 dark:text-gray-400 z-10">
                  {ask.total.toLocaleString(undefined, { maximumFractionDigits: 0 })}
                </div>
              </div>
            );
          })}
        </div>

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
        <div className="flex flex-col justify-start flex-1 min-h-[90px]">
          {bidsToRender.length === 0 ? (
            <div className="flex-1 flex items-center justify-center px-3 text-center text-[10px] uppercase tracking-wide text-gray-400 dark:text-gray-500">
              No buy levels
            </div>
          ) : bidsToRender.map((bid, idx) => {
            const levelPrice = bid.price;
            return (
              <div
                key={levelKey('bid', bid, idx)}
                onClick={() => onSelectPrice(levelPrice)}
                className="relative grid grid-cols-3 px-3 py-0.5 cursor-pointer hover:bg-surface-3 transition-colors group"
              >
                {/* Visual bar width representing depth */}
                <div
                  className="absolute right-0 top-0 bottom-0 bg-trade-green/5 dark:bg-trade-green-bg pointer-events-none"
                  style={{ width: `${Math.min(100, bid.depthPercent)}%` }}
                />
                
                <div className="text-trade-green font-medium z-10 group-hover:underline">
                  {formatPrice(levelPrice)}
                </div>
                <div className="text-right text-gray-700 dark:text-gray-300 z-10">
                  {formatQuantity(bid.amount)}
                </div>
                <div className="text-right text-gray-500 dark:text-gray-400 z-10">
                  {bid.total.toLocaleString(undefined, { maximumFractionDigits: 0 })}
                </div>
              </div>
            );
          })}
        </div>

      </div>

      <div className="border-t border-[#e1e4e8] dark:border-[#21262d] bg-[#f6f8fa] dark:bg-[#0d1117] px-3 py-2 min-h-[92px]">
        <div className="flex items-center justify-between mb-1.5">
          <span className="text-[10px] uppercase font-mono font-bold tracking-wide text-gray-500 dark:text-gray-400">
            My Order Flow
          </span>
          <span className="text-[9px] font-mono text-gray-400">
            {myOpenOrders.length} open / {myRecentOrders.length} recent
          </span>
        </div>

        {visibleOrders.length === 0 ? (
          <div className="h-12 flex items-center justify-center text-[10px] font-mono text-gray-400">
            Select a pair and submit limit, stop-limit, or market orders to track them here.
          </div>
        ) : (
          <div className="space-y-1 max-h-[60px] overflow-y-auto pr-1">
            {visibleOrders.map((order) => (
              <div
                key={order.id}
                className="grid grid-cols-[52px_58px_1fr_70px] items-center gap-1 text-[10px] font-mono"
              >
                <span className={`px-1.5 py-0.5 rounded text-center font-bold ${
                  order.side === 'BUY'
                    ? 'text-trade-green bg-trade-green-bg'
                    : 'text-trade-red bg-trade-red-bg'
                }`}>
                  {order.side}
                </span>
                <span className="text-gray-500 dark:text-gray-400 font-semibold">
                  {order.type === 'STOP_LIMIT' ? 'STOP' : order.type}
                </span>
                <span className="truncate text-gray-700 dark:text-gray-300">
                  {order.type === 'MARKET'
                    ? `Filled at ${formatPrice(order.price)}`
                    : order.type === 'STOP_LIMIT'
                      ? `Stop ${formatPrice(order.stopPrice || 0)} -> ${formatPrice(order.price)}`
                      : `${formatPrice(order.price)} x ${formatQuantity(Math.max(0, order.amount - order.filled))}`}
                </span>
                <span className={`text-right text-[9px] font-bold ${
                  order.status === 'FILLED' ? 'text-trade-green' : order.status === 'CANCELLED' ? 'text-gray-400' : 'text-accent-1'
                }`}>
                  {order.status}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function uniqueOrders(orders: Order[]): Order[] {
  const seen = new Set<string>();
  return orders.filter((order) => {
    if (seen.has(order.id)) return false;
    seen.add(order.id);
    return true;
  });
}

function levelKey(side: 'ask' | 'bid', level: OrderBookLevel, index: number): string {
  if (Number.isFinite(level.price) && level.price > 0) {
    return `${side}-${level.price.toFixed(12)}`;
  }
  return `${side}-${index}`;
}
