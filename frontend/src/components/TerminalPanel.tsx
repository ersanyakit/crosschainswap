/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import { useState } from 'react';
import { Trash2, CheckCircle, Info, XOctagon, AlertTriangle, Activity, Loader2 } from 'lucide-react';
import { Order, Trade, SystemLog } from '../types/trading';
import { formatPrice, formatQuantity } from '../utils/formatters';
import { type AssetPriceResponse } from '../services/exchangeService';

interface TerminalPanelProps {
  openOrders: Order[];
  orderHistory: Order[];
  tradeHistory: Trade[];
  systemLogs: SystemLog[];
  selectedAssetSymbol: string;
  dexPrices: AssetPriceResponse | null;
  dexPricesLoading: boolean;
  dexPricesError: string | null;
  onCancelOrder: (id: string) => void;
  onCancelAllOrders: () => void;
}

export default function TerminalPanel({
  openOrders,
  orderHistory,
  tradeHistory,
  systemLogs,
  selectedAssetSymbol,
  dexPrices,
  dexPricesLoading,
  dexPricesError,
  onCancelOrder,
  onCancelAllOrders,
}: TerminalPanelProps) {
  const [activeTab, setActiveTab] = useState<'OPEN_ORDERS' | 'ORDER_HISTORY' | 'TRADE_HISTORY' | 'SYSTEM_LOGS' | 'DEX_PRICES'>('OPEN_ORDERS');

  const getLogIcon = (type: string) => {
    switch (type) {
      case 'SUCCESS': return <CheckCircle className="w-3.5 h-3.5 text-trade-green shrink-0" />;
      case 'WARNING': return <AlertTriangle className="w-3.5 h-3.5 text-amber-500 shrink-0" />;
      case 'ERROR': return <XOctagon className="w-3.5 h-3.5 text-trade-red shrink-0" />;
      default: return <Info className="w-3.5 h-3.5 text-sky-400 shrink-0" />;
    }
  };

  return (
    <div className="bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm flex flex-col h-[280px] sm:h-[320px] overflow-hidden text-gray-800 dark:text-gray-150 select-none">
      
      {/* Tabbed Navigation Bar */}
      <div className="flex flex-wrap items-center justify-between border-b border-[#e1e4e8] dark:border-[#21262d] bg-[#f6f8fa] dark:bg-[#0d1117] px-3">
        <div className="flex gap-1 overflow-x-auto scrollbar-hide py-1">
          {([
            { id: 'OPEN_ORDERS', label: 'Open Orders', badge: openOrders.length },
            { id: 'ORDER_HISTORY', label: 'Order History', badge: orderHistory.length },
            { id: 'TRADE_HISTORY', label: 'Trade History', badge: undefined },
            { id: 'SYSTEM_LOGS', label: 'System Logs', badge: systemLogs.length },
            { id: 'DEX_PRICES', label: 'DEX Prices', badge: dexPrices?.prices.length },
          ] as const).map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-1.5 px-3 py-1 text-xs font-mono font-semibold rounded transition-colors whitespace-nowrap cursor-pointer ${
                activeTab === tab.id
                  ? 'bg-accent-2 border border-accent-1/20 text-accent-1 font-bold'
                  : 'text-gray-500 hover:text-gray-800 dark:hover:text-gray-250 hover:bg-surface-3'
              }`}
            >
              {tab.label}
              {tab.badge !== undefined && tab.badge > 0 && (
                <span className="px-1 text-[9px] font-bold bg-accent-1 text-white rounded-full min-w-[14px] text-center">
                  {tab.badge}
                </span>
              )}
            </button>
          ))}
        </div>

        {/* Action controllers on high-density side */}
        <div className="flex items-center gap-3 py-1">
          {activeTab === 'OPEN_ORDERS' && openOrders.length > 0 && (
            <button
              onClick={onCancelAllOrders}
              className="px-2 py-0.5 rounded text-[10px] font-mono border border-trade-red/35 text-trade-red hover:bg-trade-red/10 cursor-pointer flex items-center gap-1 transition-all"
            >
              <Trash2 className="w-3 h-3" />
              Cancel All Open Orders
            </button>
          )}

          <span className="text-[10px] text-gray-400 font-mono hidden sm:inline">
            Active Core Console v3
          </span>
        </div>
      </div>

      {/* Panel Scroll Container */}
      <div className="flex-1 overflow-auto text-xs">
        
        {/* VIEW 1: OPEN ORDERS */}
        {activeTab === 'OPEN_ORDERS' && (
          <div className="min-w-[650px] p-2">
            {openOrders.length === 0 ? (
              <div className="h-44 flex flex-col items-center justify-center text-gray-400 italic font-mono gap-1">
                <CheckCircle className="w-5 h-5 text-gray-300" />
                No active standing limit orders found.
              </div>
            ) : (
              <table className="w-full text-left font-mono">
                <thead>
                  <tr className="text-[10px] uppercase text-gray-400 border-b border-[#e1e4e8]/50 dark:border-[#21262d]/50 pb-1 text-left">
                    <th className="py-1.5 pl-2">Time</th>
                    <th>Symbol</th>
                    <th>Side</th>
                    <th>Type</th>
                    <th className="text-right">Price</th>
                    <th className="text-right">Amount</th>
                    <th className="text-right">Filled</th>
                    <th className="text-right">Total</th>
                    <th className="text-center">Action</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[#e1e4e8]/40 dark:divide-[#21262d]/40">
                  {openOrders.map((ord) => (
                    <tr key={ord.id} className="hover:bg-gray-50 dark:hover:bg-[#161b22]/30 transition-colors">
                      <td className="py-2 pl-2 text-gray-400 text-[10px]">
                        {ord.timestamp.toLocaleString(undefined, {hour: '2-digit', minute:'2-digit', second:'2-digit'})}
                      </td>
                      <td className="font-semibold text-gray-900 dark:text-gray-100">{ord.symbol}</td>
                      <td>
                        <span className={`px-1.5 py-0.5 rounded text-[9px] font-bold ${ord.side === 'BUY' ? 'text-trade-green bg-trade-green-bg' : 'text-trade-red bg-trade-red-bg'}`}>
                          {ord.side}
                        </span>
                      </td>
                      <td className="text-gray-500 font-medium">{ord.type}</td>
                      <td className="text-right font-medium">{formatPrice(ord.price)}</td>
                      <td className="text-right">{formatQuantity(ord.amount)}</td>
                      <td className="text-right text-gray-500">{formatQuantity(ord.filled)}</td>
                      <td className="text-right font-semibold text-gray-800 dark:text-gray-200">
                        {ord.total.toLocaleString(undefined, {minimumFractionDigits: 2})} {ord.symbol.split('/')[1] || 'USDC'}
                      </td>
                      <td className="text-center">
                        <button
                          onClick={() => onCancelOrder(ord.id)}
                          className="px-1.5 py-0.5 text-[9px] border border-trade-red/45 text-trade-red hover:bg-trade-red/10 rounded cursor-pointer transition-colors"
                        >
                          Cancel
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        )}

        {/* VIEW 2: ORDER HISTORY */}
        {activeTab === 'ORDER_HISTORY' && (
          <div className="min-w-[650px] p-2">
            {orderHistory.length === 0 ? (
              <div className="h-44 flex flex-col items-center justify-center text-gray-400 italic font-mono gap-1">
                No archived order logs.
              </div>
            ) : (
              <table className="w-full text-left font-mono">
                <thead>
                  <tr className="text-[10px] uppercase text-gray-400 border-b border-[#e1e4e8]/50 dark:border-[#21262d]/50 pb-1">
                    <th className="py-1.5 pl-2">Time</th>
                    <th>Symbol</th>
                    <th>Side</th>
                    <th>Type</th>
                    <th className="text-right">Price</th>
                    <th className="text-right">Amount</th>
                    <th className="text-right">Total</th>
                    <th className="text-right pr-2">Status</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[#e1e4e8]/45 dark:divide-[#21262d]/45">
                  {orderHistory.map((ord) => (
                    <tr key={ord.id} className="hover:bg-gray-50 dark:hover:bg-[#161b22]/30 transition-colors">
                      <td className="py-2 pl-2 text-gray-400 text-[10px]">
                        {ord.timestamp.toLocaleDateString()} {ord.timestamp.toLocaleTimeString()}
                      </td>
                      <td className="font-semibold text-gray-900 dark:text-gray-150">{ord.symbol}</td>
                      <td>
                        <span className={`px-1.5 py-0.5 rounded text-[9px] font-bold ${ord.side === 'BUY' ? 'text-trade-green bg-trade-green-bg' : 'text-trade-red bg-trade-red-bg'}`}>
                          {ord.side}
                        </span>
                      </td>
                      <td className="text-gray-500">{ord.type}</td>
                      <td className="text-right">{formatPrice(ord.price)}</td>
                      <td className="text-right">{formatQuantity(ord.amount)}</td>
                      <td className="text-right font-medium">{ord.total.toFixed(2)} {ord.symbol.split('/')[1] || 'USDC'}</td>
                      <td className="text-right pr-2">
                        <span className={`px-2 py-0.5 rounded-full text-[9px] font-bold ${
                          ord.status === 'FILLED' ? 'text-trade-green bg-[#10b981]/10' : 'text-gray-400 bg-gray-100 dark:bg-slate-800'
                        }`}>
                          {ord.status}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        )}

        {/* VIEW 3: TRADE HISTORY */}
        {activeTab === 'TRADE_HISTORY' && (
          <div className="min-w-[650px] p-2">
            {tradeHistory.length === 0 ? (
              <div className="h-44 flex flex-col items-center justify-center text-gray-400 italic font-mono gap-1">
                No recent matches recorded on account.
              </div>
            ) : (
              <table className="w-full text-left font-mono">
                <thead>
                  <tr className="text-[10px] uppercase text-gray-400 border-b border-[#e1e4e8]/50 dark:border-[#21262d]/50 pb-1">
                    <th className="py-1.5 pl-2">Matching ID</th>
                    <th>Time</th>
                    <th>Symbol</th>
                    <th>Side</th>
                    <th className="text-right">Price</th>
                    <th className="text-right">Executed Qty</th>
                    <th className="text-right pr-2">Notional Value</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[#e1e4e8]/45 dark:divide-[#21262d]/45">
                  {tradeHistory.map((tr) => (
                    <tr key={tr.id} className="hover:bg-gray-50 dark:hover:bg-[#161b22]/30 transition-colors">
                      <td className="py-2 pl-2 text-gray-500 text-[10px] font-mono">{tr.id}</td>
                      <td className="text-gray-400 text-[10px]">{tr.timestamp.toLocaleTimeString()}</td>
                      <td className="font-semibold text-gray-950 dark:text-gray-100">{tr.symbol}</td>
                      <td>
                        <span className={`px-1.5 py-0.5 rounded text-[9px] font-bold ${tr.side === 'BUY' ? 'text-trade-green bg-trade-green-bg' : 'text-trade-red bg-trade-red-bg'}`}>
                          {tr.side}
                        </span>
                      </td>
                      <td className="text-right font-medium text-gray-900 dark:text-gray-150">
                        {formatPrice(tr.price)}
                      </td>
                      <td className="text-right">{formatQuantity(tr.amount)}</td>
                      <td className="text-right font-bold pr-2 text-accent-1">
                        {tr.total.toLocaleString(undefined, {minimumFractionDigits: 2})} {tr.symbol.split('/')[1] || 'USDC'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        )}

        {/* VIEW 4: SYSTEM LOGS */}
        {activeTab === 'SYSTEM_LOGS' && (
          <div className="p-3 font-mono space-y-1 bg-slate-950 text-[#d1d5db] min-h-full leading-normal text-[11px] select-text selection:bg-accent-1/20 select-none">
            {systemLogs.map((log) => (
              <div key={log.id} className="flex gap-2 items-start opacity-90 hover:opacity-100 transition-opacity">
                <span className="text-gray-500 shrink-0 select-none">
                  [{log.timestamp.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })}]
                </span>
                <span className={`text-[10px] px-1 rounded font-bold shrink-0 select-none ${
                  log.source === 'STRATEGY' ? 'bg-indigo-950 text-indigo-400' :
                  log.source === 'WEBSOCKET' ? 'bg-sky-950 text-sky-400' :
                  log.source === 'ORDER' ? 'bg-emerald-950 text-emerald-400' : 'bg-slate-800 text-gray-400'
                }`}>
                  {log.source}
                </span>
                {getLogIcon(log.type)}
                <span className={`flex-1 ${
                  log.type === 'ERROR' ? 'text-rose-400' :
                  log.type === 'WARNING' ? 'text-amber-400' :
                  log.type === 'SUCCESS' ? 'text-emerald-300' : 'text-slate-300'
                }`}>
                  {log.message}
                </span>
              </div>
            ))}
          </div>
        )}

        {/* VIEW 5: CROSS-DEX PRICES */}
        {activeTab === 'DEX_PRICES' && (
          <div className="min-w-[760px] min-h-full bg-[#fdfdfd] dark:bg-[#090d12]">
            <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3 border-b border-[#e1e4e8] dark:border-[#21262d]">
              <div className="flex items-center gap-2">
                <Activity className="w-4 h-4 text-accent-1" />
                <div>
                  <div className="text-xs font-display font-semibold uppercase text-gray-800 dark:text-gray-200">
                    {selectedAssetSymbol} Cross-DEX Price Board
                  </div>
                  <div className="text-[10px] font-mono text-gray-400">
                    Registry-backed pools only
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-2 text-[10px] font-mono text-gray-400">
                {dexPricesLoading && <Loader2 className="w-3.5 h-3.5 animate-spin text-accent-1" />}
                <span>{dexPrices?.prices.length || 0} pools</span>
              </div>
            </div>

            {dexPricesError ? (
              <div className="h-44 flex items-center justify-center text-trade-red font-mono text-xs">
                {dexPricesError}
              </div>
            ) : !dexPricesLoading && (!dexPrices || dexPrices.prices.length === 0) ? (
              <div className="h-44 flex flex-col items-center justify-center text-gray-400 italic font-mono gap-1">
                <Activity className="w-5 h-5 text-gray-300" />
                No DEX prices found for {selectedAssetSymbol}.
              </div>
            ) : (
              <table className="w-full text-left font-mono">
                <thead>
                  <tr className="text-[10px] uppercase text-gray-400 border-b border-[#e1e4e8]/60 dark:border-[#21262d]/60">
                    <th className="py-2 pl-4">Chain</th>
                    <th>DEX</th>
                    <th>Pair</th>
                    <th>Kind</th>
                    <th className="text-right">Base / Quote</th>
                    <th className="text-right">Quote / Base</th>
                    <th className="text-right">Base USDC</th>
                    <th className="text-right">Quote USDC</th>
                    <th className="text-right pr-4">Pool</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[#e1e4e8]/45 dark:divide-[#21262d]/45">
                  {(dexPrices?.prices || []).map((pool) => (
                    <tr key={`${pool.chain_key}:${pool.venue_key}:${pool.pool_id}`} className="hover:bg-gray-50 dark:hover:bg-[#161b22]/40 transition-colors">
                      <td className="py-2 pl-4">
                        <span className="px-1.5 py-0.5 rounded bg-surface-3 text-gray-700 dark:text-gray-250 text-[9px] font-bold uppercase">
                          {pool.chain_key}
                        </span>
                      </td>
                      <td className="font-semibold text-gray-850 dark:text-gray-150 uppercase">{formatVenue(pool.venue_key)}</td>
                      <td className="text-gray-700 dark:text-gray-250">
                        {pool.base_symbol}/{pool.quote_symbol}
                      </td>
                      <td>
                        <span className="text-[9px] px-1.5 py-0.5 rounded bg-accent-2 text-accent-1 font-bold uppercase">
                          {pool.pool_kind}
                        </span>
                      </td>
                      <td className="text-right text-gray-900 dark:text-gray-100">
                        {formatDecimal(pool.price)} {pool.quote_symbol}
                      </td>
                      <td className="text-right text-gray-500">
                        {formatDecimal(pool.inverse_price)}
                      </td>
                      <td className="text-right font-semibold text-trade-green">
                        {formatUSD(pool.base_price_usdc || pool.price_usdc)}
                      </td>
                      <td className="text-right text-gray-700 dark:text-gray-250">
                        {formatUSD(pool.quote_price_usdc)}
                      </td>
                      <td className="text-right pr-4">
                        <span title={pool.pool_id} className="inline-flex items-center justify-end gap-1 text-gray-400 select-text">
                          {shortID(pool.pool_id)}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        )}

      </div>
    </div>
  );
}

function formatVenue(value: string): string {
  return value.replace(/[_-]/g, ' ');
}

function formatDecimal(value?: string, maximumFractionDigits = 12): string {
  if (!value) return '-';
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '-';
  if (numeric !== 0 && Math.abs(numeric) < 0.000001) return numeric.toExponential(6);
  return numeric.toLocaleString(undefined, { maximumFractionDigits });
}

function formatUSD(value?: string): string {
  const formatted = formatDecimal(value, 12);
  return formatted === '-' ? '-' : `$${formatted}`;
}

function shortID(value: string): string {
  if (!value) return '-';
  if (value.length <= 14) return value;
  return `${value.slice(0, 6)}...${value.slice(-4)}`;
}
