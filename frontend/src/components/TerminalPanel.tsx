/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import { useState } from 'react';
import { Trash2, CheckCircle, Info, XOctagon, AlertTriangle, Activity, Loader2, ChevronDown, ChevronsDown, ChevronsUp } from 'lucide-react';
import { Order, Trade, SystemLog } from '../types/trading';
import { formatPrice, formatQuantity } from '../utils/formatters';
import { type AssetInfo, type AssetDeploymentInfo, type AssetPriceResponse, type DexPoolPrice } from '../services/exchangeService';
import AssetIcon from './AssetIcon';

interface TerminalPanelProps {
  openOrders: Order[];
  orderHistory: Order[];
  tradeHistory: Trade[];
  systemLogs: SystemLog[];
  selectedAssetSymbol: string;
  dexPrices: AssetPriceResponse | null;
  dexPricesLoading: boolean;
  dexPricesError: string | null;
  assetMetadata: Record<string, AssetInfo>;
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
  assetMetadata,
  onCancelOrder,
  onCancelAllOrders,
}: TerminalPanelProps) {
  const [activeTab, setActiveTab] = useState<'OPEN_ORDERS' | 'ORDER_HISTORY' | 'TRADE_HISTORY' | 'SYSTEM_LOGS' | 'DEX_PRICES'>('OPEN_ORDERS');
  const [expandedDexPoolKeys, setExpandedDexPoolKeys] = useState<Set<string>>(() => new Set());
  const dexPools = dexPrices?.prices || [];
  const dexPoolKeys = dexPools.map(dexPoolKey);
  const expandedDexPoolsCount = dexPoolKeys.filter((key) => expandedDexPoolKeys.has(key)).length;
  const areAllDexPoolsExpanded = dexPoolKeys.length > 0 && expandedDexPoolsCount === dexPoolKeys.length;

  const toggleDexPool = (key: string) => {
    setExpandedDexPoolKeys((current) => {
      const next = new Set(current);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  const expandAllDexPools = () => {
    setExpandedDexPoolKeys(new Set(dexPoolKeys));
  };

  const collapseAllDexPools = () => {
    setExpandedDexPoolKeys(new Set());
  };

  const getLogIcon = (type: string) => {
    switch (type) {
      case 'SUCCESS': return <CheckCircle className="w-3.5 h-3.5 text-trade-green shrink-0" />;
      case 'WARNING': return <AlertTriangle className="w-3.5 h-3.5 text-amber-500 shrink-0" />;
      case 'ERROR': return <XOctagon className="w-3.5 h-3.5 text-trade-red shrink-0" />;
      default: return <Info className="w-3.5 h-3.5 text-sky-400 shrink-0" />;
    }
  };

  return (
    <div className="bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm flex flex-col overflow-hidden text-gray-800 dark:text-gray-100 select-none">
      
      {/* Tabbed Navigation Bar */}
      <div className="shrink-0 flex flex-wrap items-center justify-between border-b border-[#e1e4e8] dark:border-[#21262d] bg-[#f6f8fa] dark:bg-[#0d1117] px-3">
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
                  : 'text-gray-500 hover:text-gray-800 dark:hover:text-gray-200 hover:bg-surface-3'
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

          {activeTab === 'DEX_PRICES' && dexPools.length > 0 && (
            <div className="flex items-center gap-1">
              <button
                type="button"
                onClick={expandAllDexPools}
                disabled={areAllDexPoolsExpanded}
                title="Expand all DEX details"
                className="h-6 w-6 rounded border border-[#d8dee4] dark:border-[#2f3a48] text-gray-500 hover:text-accent-1 hover:border-accent-1/45 hover:bg-surface-3 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer flex items-center justify-center transition-colors"
              >
                <ChevronsDown className="w-3.5 h-3.5" />
              </button>
              <button
                type="button"
                onClick={collapseAllDexPools}
                disabled={expandedDexPoolsCount === 0}
                title="Collapse all DEX details"
                className="h-6 w-6 rounded border border-[#d8dee4] dark:border-[#2f3a48] text-gray-500 hover:text-accent-1 hover:border-accent-1/45 hover:bg-surface-3 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer flex items-center justify-center transition-colors"
              >
                <ChevronsUp className="w-3.5 h-3.5" />
              </button>
            </div>
          )}

        </div>
      </div>

      {/* Panel Scroll Container */}
      <div className={`text-xs ${activeTab === 'DEX_PRICES' ? 'overflow-visible min-w-0' : 'overflow-auto'}`}>
        
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
                  {openOrders.map((ord, idx) => (
                    <tr key={`open-${ord.id}-${idx}`} className="hover:bg-gray-50 dark:hover:bg-[#161b22]/30 transition-colors">
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
                  {orderHistory.map((ord, idx) => (
                    <tr key={`history-${ord.id}-${idx}`} className="hover:bg-gray-50 dark:hover:bg-[#161b22]/30 transition-colors">
                      <td className="py-2 pl-2 text-gray-400 text-[10px]">
                        {ord.timestamp.toLocaleDateString()} {ord.timestamp.toLocaleTimeString()}
                      </td>
                      <td className="font-semibold text-gray-900 dark:text-gray-100">{ord.symbol}</td>
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
                      <td className="text-right font-medium text-gray-900 dark:text-gray-100">
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
          <div className="min-w-0 bg-white dark:bg-[#0b1118] flex flex-col overflow-visible">
            <div className="shrink-0 flex flex-wrap items-center gap-2 px-3 sm:px-4 py-2.5 border-b border-[#e1e4e8] dark:border-[#21262d]">
              <div className="mr-auto flex items-center gap-2 min-w-0">
                <Activity className="w-4 h-4 text-accent-1 shrink-0" />
                <AssetIcon symbol={selectedAssetSymbol} iconURL={dexPrices?.asset?.icon_url || assetMetadata[selectedAssetSymbol]?.icon_url} size="sm" />
                <div className="min-w-0">
                  <div className="text-xs font-display font-semibold uppercase text-gray-800 dark:text-gray-200">
                    {selectedAssetSymbol} Cross-DEX Price Board
                  </div>
                  <div className="text-[10px] font-mono text-gray-400">
                    Registry-backed pools only
                  </div>
                </div>
              </div>
              <div className="ml-auto flex items-center gap-2">
                {dexPricesLoading && <Loader2 className="w-3.5 h-3.5 animate-spin text-accent-1" />}
                <div className="grid grid-cols-2 gap-2 text-right font-mono">
                  <div className="rounded border border-[#e1e4e8] dark:border-[#263241] bg-[#f6f8fa] dark:bg-[#101720] px-2.5 py-1">
                    <div className="text-[9px] uppercase text-gray-400">Pools</div>
                    <div className="text-[11px] font-bold text-gray-800 dark:text-gray-100">
                      {dexPools.length}
                    </div>
                  </div>
                  <div className="rounded border border-emerald-200/70 dark:border-emerald-400/20 bg-emerald-50/70 dark:bg-emerald-400/10 px-2.5 py-1">
                    <div className="text-[9px] uppercase text-emerald-600 dark:text-emerald-300">Expanded</div>
                    <div className="text-[11px] font-bold text-emerald-700 dark:text-emerald-200">
                      {expandedDexPoolsCount}/{dexPools.length}
                    </div>
                  </div>
                </div>
              </div>
            </div>

            {dexPricesError ? (
              <div className="flex-1 flex items-center justify-center text-trade-red font-mono text-xs px-4 text-center">
                {dexPricesError}
              </div>
            ) : !dexPricesLoading && (!dexPrices || dexPrices.prices.length === 0) ? (
              <div className="flex-1 flex flex-col items-center justify-center text-gray-400 italic font-mono gap-1 px-4 text-center">
                <Activity className="w-5 h-5 text-gray-300" />
                No DEX prices found for {selectedAssetSymbol}.
              </div>
            ) : (
              <div className="overflow-visible p-2 space-y-2">
                {dexPools.map((pool) => {
                  const key = dexPoolKey(pool);
                  const isExpanded = expandedDexPoolKeys.has(key);

                  return (
                    <div
                      key={key}
                      className="min-w-0 rounded border border-[#e1e4e8] dark:border-[#263241] bg-[#fbfcfd] dark:bg-[#101720] font-mono hover:border-accent-1/45 transition-colors"
                    >
                      <button
                        type="button"
                        onClick={() => toggleDexPool(key)}
                        className="w-full min-w-0 px-3 py-2 text-left cursor-pointer"
                      >
                        <div className="grid grid-cols-1 sm:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_auto] gap-2 items-center min-w-0">
                          <div className="min-w-0 truncate font-semibold uppercase text-gray-900 dark:text-gray-100" title={formatVenue(pool.venue_key)}>
                            {formatVenue(pool.venue_key)}
                          </div>

                          <div className="flex items-center gap-2 min-w-0 text-gray-700 dark:text-gray-200">
                            <span className="flex -space-x-1 shrink-0">
                              <AssetIcon
                                symbol={pool.base_symbol}
                                iconURL={assetIconURL(pool.base_symbol, pool.base_asset, assetMetadata)}
                                size="xs"
                              />
                              <AssetIcon
                                symbol={pool.quote_symbol}
                                iconURL={assetIconURL(pool.quote_symbol, pool.quote_asset, assetMetadata)}
                                size="xs"
                              />
                            </span>
                            <span className="font-semibold truncate">{pool.base_symbol}/{pool.quote_symbol}</span>
                          </div>

                          <Metric label="Price" value={`${formatDecimal(pool.price, 12)} ${pool.quote_symbol}`} title={`${pool.price} ${pool.quote_symbol}`} />
                          <Metric label="Inverted Price" value={`${formatDecimal(pool.inverse_price, 12)} ${pool.base_symbol}`} title={`${pool.inverse_price} ${pool.base_symbol}`} />

                          <ChevronDown className={`w-4 h-4 text-gray-400 transition-transform ${isExpanded ? 'rotate-180' : ''}`} />
                        </div>
                      </button>

                      {isExpanded && (
                        <div className="border-t border-[#e1e4e8] dark:border-[#263241] px-3 py-2 bg-white/60 dark:bg-[#0b1118]/65">
	                          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-2 min-w-0">
	                            <Metric label="Chain" value={pool.chain_key} />
	                            <Metric label="Pool Type" value={pool.pool_kind} />
	                            <Metric
	                              label="Base Amount"
	                              value={`${formatPoolReserve(pool, 'base')} ${pool.base_symbol}`}
	                              title={`${formatPoolReserve(pool, 'base', 10)} ${pool.base_symbol}`}
	                              emphasis="green"
                            />
                            <Metric
                              label="Quote Amount"
                              value={`${formatPoolReserve(pool, 'quote')} ${pool.quote_symbol}`}
	                              title={`${formatPoolReserve(pool, 'quote', 10)} ${pool.quote_symbol}`}
	                              emphasis="green"
	                            />
	                            <AddressMetric label="Pool Address" value={pool.pool_id} />
	                          </div>
	                        </div>
	                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}

      </div>
    </div>
  );
}

function Metric({
  label,
  value,
  title,
  emphasis = 'default',
}: {
  label: string;
  value: string;
  title?: string;
  emphasis?: 'default' | 'green';
}) {
  return (
    <div className="min-w-0">
      <div className="text-[9px] uppercase text-gray-400">{label}</div>
      <div
        title={title || value}
        className={`truncate tabular-nums text-[11px] font-semibold ${
          emphasis === 'green'
            ? 'text-emerald-700 dark:text-emerald-300'
            : 'text-gray-800 dark:text-gray-100'
        }`}
      >
        {value}
      </div>
    </div>
  );
}

function AddressMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 sm:col-span-2 lg:col-span-4">
      <div className="text-[9px] uppercase text-gray-400">{label}</div>
      <div
        title={value || '-'}
        className="select-text break-all tabular-nums text-[11px] font-semibold text-gray-800 dark:text-gray-100"
      >
        {value || '-'}
      </div>
    </div>
  );
}

function formatVenue(value: string): string {
  return value.replace(/[_-]/g, ' ');
}

function assetIconURL(
  symbol: string,
  deployment: AssetDeploymentInfo | undefined,
  metadata: Record<string, AssetInfo>
): string | undefined {
  return deployment?.icon_url || metadata[symbol.toUpperCase()]?.icon_url;
}

function dexPoolKey(pool: DexPoolPrice): string {
  return `${pool.chain_key}:${pool.venue_key}:${pool.pool_id}`;
}

function formatDecimal(value?: string, maximumFractionDigits = 12): string {
  if (!value) return '-';
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '-';
  if (numeric !== 0 && Math.abs(numeric) < 1) {
    return trimDecimal(numeric.toFixed(maximumFractionDigits));
  }
  return numeric.toLocaleString(undefined, { maximumFractionDigits });
}

function formatPoolReserve(pool: DexPoolPrice, side: 'base' | 'quote', maximumFractionDigits = 6): string {
  const rawReserve = side === 'base' ? pool.reserve_base : pool.reserve_quote;
  const decimals = side === 'base' ? pool.base_asset?.decimals : pool.quote_asset?.decimals;
  const reserve = tokenAmount(rawReserve, decimals);
  if (reserve === null) return '-';
  return formatNumber(reserve, maximumFractionDigits);
}

function numericValue(value?: string): number | null {
  if (!value) return null;
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : null;
}

function tokenAmount(rawValue?: string, decimals?: number): number | null {
  const numeric = numericValue(rawValue);
  if (numeric === null) return null;
  if (typeof decimals !== 'number' || decimals < 0) return numeric;
  return numeric / (10 ** decimals);
}

function formatNumber(value: number, maximumFractionDigits = 6): string {
  if (!Number.isFinite(value)) return '-';
  if (value !== 0 && Math.abs(value) < 0.000001) {
    return trimDecimal(value.toFixed(12));
  }
  return value.toLocaleString(undefined, { maximumFractionDigits });
}

function trimDecimal(value: string): string {
  if (!value.includes('.')) return value;
  const trimmed = value.replace(/0+$/, '').replace(/\.$/, '');
  return trimmed === '-0' || trimmed === '' ? '0' : trimmed;
}
