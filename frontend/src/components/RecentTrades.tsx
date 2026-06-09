/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import { Flame } from 'lucide-react';
import { Trade, MarketPair } from '../types/trading';
import { formatPrice, formatQuantity } from '../utils/formatters';

interface RecentTradesProps {
  trades: Trade[];
  pair: MarketPair;
}

export default function RecentTrades({ trades, pair }: RecentTradesProps) {
  // Take the top 15-20 records
  const renderedTrades = trades.slice(0, 16);

  return (
    <div className="bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm flex flex-col h-[280px] sm:h-[350px] overflow-hidden text-gray-800 dark:text-gray-100 select-none">
      
      {/* Titlebar Header */}
      <div className="flex items-center justify-between px-3 py-2 bg-[#f6f8fa] dark:bg-[#0d1117] border-b border-[#e1e4e8] dark:border-[#21262d]">
        <span className="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 font-display flex items-center gap-1.5">
          <Flame className="w-3.5 h-3.5 text-accent-1" />
          Recent Trades
        </span>
        <span className="text-[10px] font-mono text-gray-400 bg-surface-3 px-1.5 py-0.5 rounded">
          Live Trades feed
        </span>
      </div>

      {/* Table Headers */}
      <div className="grid grid-cols-3 px-3 py-1 text-[10px] uppercase font-mono text-gray-400 border-b border-[#e1e4e8]/50 dark:border-[#21262d]/50 bg-gray-50/50 dark:bg-transparent">
        <div>Price ({pair.quoteAsset})</div>
        <div className="text-right">Size ({pair.baseAsset})</div>
        <div className="text-right">Time</div>
      </div>

      {/* Grid Content List */}
      <div className="flex-1 overflow-y-auto font-mono text-[11px] divide-y divide-gray-50 dark:divide-transparent py-1">
        {renderedTrades.length === 0 ? (
          <div className="h-full flex items-center justify-center text-gray-400 italic text-[11px]">
            Awaiting executions...
          </div>
        ) : (
          renderedTrades.map((trade) => {
            const isBuy = trade.side === 'BUY';
            const priceStr = formatPrice(trade.price, 8, 8);
            const timeStr = trade.timestamp.toLocaleTimeString(undefined, {
              hour: '2-digit',
              minute: '2-digit',
              second: '2-digit',
              hour12: false
            });

            return (
              <div
                key={trade.id}
                className="grid grid-cols-3 px-3 py-1 items-center hover:bg-slate-50 dark:hover:bg-[#161b22]/50 transition-colors"
              >
                {/* Price */}
                <div className={`font-semibold ${isBuy ? 'text-trade-green' : 'text-trade-red'}`}>
                  {priceStr}
                </div>
                
                {/* Size */}
                <div className="text-right text-gray-700 dark:text-gray-300 font-mono">
                  {formatQuantity(trade.amount, 8)}
                </div>

                {/* Timestamp */}
                <div className="text-right text-gray-400 dark:text-gray-500 text-[10px]">
                  {timeStr}
                </div>
              </div>
            );
          })
        )}
      </div>

      {/* Bottom stats aggregator */}
      <div className="p-2 border-t border-[#e1e4e8] dark:border-[#21262d] bg-[#f9fafc] dark:bg-[#070b0f] text-[9px] font-mono text-gray-400 flex justify-between">
        <span>Base ticker: {pair.symbol}</span>
        <span className="flex items-center gap-1">
          <span className="w-1.5 h-1.5 bg-trade-green rounded-full inline-block animate-pulse"></span>
          24h trades: {(pair.volume24h * 1.5).toLocaleString(undefined, { maximumFractionDigits: 0 })}
        </span>
      </div>

    </div>
  );
}
