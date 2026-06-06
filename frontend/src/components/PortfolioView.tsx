/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import React, { useState } from 'react';
import { Briefcase, ArrowUpRight, ArrowDownLeft, ShieldCheck, Wallet, ChevronRight, Activity, PlusCircle, MinusCircle, AlertCircle } from 'lucide-react';
import { AssetBalance } from '../types/trading';

interface PortfolioViewProps {
  balances: AssetBalance[];
  onDeposit: (asset: string, amount: number) => void;
  onWithdraw: (asset: string, amount: number) => void;
  transactions: Array<{ id: string; type: string; asset: string; amount: number; time: Date }>;
}

export default function PortfolioView({
  balances,
  onDeposit,
  onWithdraw,
  transactions,
}: PortfolioViewProps) {
  const [activeTab, setActiveTab] = useState<'OVERVIEW' | 'TX_LEDGER'>('OVERVIEW');
  const [selectedAsset, setSelectedAsset] = useState('USD');
  const [formAmount, setFormAmount] = useState('');
  const [actionType, setActionType] = useState<'DEPOSIT' | 'WITHDRAW'>('DEPOSIT');
  const [errorMessage, setErrorMessage] = useState('');
  const [successMsg, setSuccessMsg] = useState('');

  // Calculations
  const totalBalanceUsd = balances.reduce((sum, b) => sum + b.valueUsd, 0);
  const totalLockedUsd = balances.reduce((sum, b) => sum + (b.locked * (b.valueUsd / (b.free || 1))), 0);
  const totalFreeUsd = totalBalanceUsd - totalLockedUsd;

  // Let's create a realistic daily PNL (e.g. +$1,240.23 / +2.15%)
  const yesterdayBalance = totalBalanceUsd * 0.9785; // up 2.15% overall
  const absolutePnl = totalBalanceUsd - yesterdayBalance;
  const percentagePnl = 2.15;

  const handleActionSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setErrorMessage('');
    setSuccessMsg('');
    const amt = parseFloat(formAmount);
    
    if (isNaN(amt) || amt <= 0) {
      setErrorMessage('Please type a valid amount larger than zero.');
      return;
    }

    if (actionType === 'WITHDRAW') {
      const assetBal = balances.find(b => b.asset === selectedAsset);
      if (!assetBal || assetBal.free < amt) {
        setErrorMessage(`Insufficient free ${selectedAsset} to complete withdrawal.`);
        return;
      }
      onWithdraw(selectedAsset, amt);
      setSuccessMsg(`Simulated withdrawal of ${amt} ${selectedAsset} registered successfully.`);
    } else {
      onDeposit(selectedAsset, amt);
      setSuccessMsg(`Simulated deposit of ${amt} ${selectedAsset} settled successfully.`);
    }

    setFormAmount('');
  };

  return (
    <div className="flex-1 overflow-y-auto p-4 sm:p-5 bg-[#fafbfc] dark:bg-[#070b0f] space-y-5 select-none h-full max-w-7xl mx-auto">
      
      {/* Overview Bento Card Row */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        
        {/* Card 1: Combined Net Asset Equity */}
        <div className="p-5 bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm flex flex-col justify-between ide-glow relative overflow-hidden">
          <div className="absolute right-3 top-3 p-1.5 rounded-full bg-accent-2 text-accent-1">
            <Briefcase className="w-5 h-5" />
          </div>
          <div>
            <span className="block text-[10px] uppercase font-mono tracking-widest text-[#7e8c9a] font-bold mb-1">
              Estimated Balance (USD)
            </span>
            <span className="text-2xl sm:text-3xl font-display font-black text-gray-950 dark:text-gray-50">
              ${totalBalanceUsd.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
            </span>
          </div>

          <div className="mt-4 flex items-center gap-2 text-xs font-mono">
            <div className="flex items-center text-trade-green rounded-full bg-trade-green-bg px-2 py-0.5 font-bold">
              <ArrowUpRight className="w-3.5 h-3.5 mr-0.5" />
              +{percentagePnl}%
            </div>
            <span className="text-gray-500">
              +${absolutePnl.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })} (Today)
            </span>
          </div>
        </div>

        {/* Card 2: Collateral margins / locked assets */}
        <div className="p-5 bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm flex flex-col justify-between">
          <div>
            <span className="block text-[10px] uppercase font-mono tracking-widest text-[#7e8c9a] font-bold mb-1">
              Available Cash vs Standing Orders
            </span>
            <div className="space-y-1 mt-2">
              <div className="flex justify-between items-center text-xs">
                <span className="text-gray-500 font-mono">Liquid Free Assets:</span>
                <span className="font-semibold text-gray-800 dark:text-gray-200 font-mono">
                  ${totalFreeUsd.toLocaleString(undefined, { minimumFractionDigits: 2 })}
                </span>
              </div>
              <div className="flex justify-between items-center text-xs">
                <span className="text-gray-500 font-mono">Locked in Orders:</span>
                <span className="font-semibold text-amber-500 font-mono">
                  ${totalLockedUsd.toLocaleString(undefined, { minimumFractionDigits: 2 })}
                </span>
              </div>
            </div>
          </div>

          <div className="mt-3 text-[10px] font-mono text-gray-400 border-t border-gray-100 dark:border-gray-800/60 pt-2 flex items-center gap-1.5">
            <ShieldCheck className="w-3.5 h-3.5 text-trade-green shrink-0" />
            Locked blocks secure backing on pending limits.
          </div>
        </div>

        {/* Card 3: Live Account Settings mode */}
        <div className="p-5 bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm flex flex-col justify-between">
          <div>
            <span className="block text-[10px] uppercase font-mono tracking-widest text-[#7e8c9a] font-bold mb-1">
              Clearing Mode status
            </span>
            <div className="flex items-center gap-2 mt-1">
              <span className="w-2.5 h-2.5 bg-trade-green rounded-full inline-block animate-pulse"></span>
              <span className="text-sm font-semibold font-display">Fast Margin-Free Spot Node</span>
            </div>
          </div>

          <div className="space-y-1 font-mono text-[10px] text-gray-500">
            <div className="flex justify-between">
              <span>Maker / Taker fee rate:</span>
              <span className="font-bold text-accent-1">0.08% / 0.10%</span>
            </div>
            <div className="flex justify-between">
              <span>Withdrawal Limit:</span>
              <span className="font-bold text-gray-700 dark:text-gray-200">50.0 BTC / Daily</span>
            </div>
          </div>
        </div>

      </div>

      {/* Primary Panels tabs */}
      <div className="flex border-b border-[#e1e4e8] dark:border-[#21262d] text-xs font-mono">
        <button
          onClick={() => setActiveTab('OVERVIEW')}
          className={`pb-2 px-4 border-b-2 font-bold cursor-pointer flex items-center gap-1.5 transition-all ${
            activeTab === 'OVERVIEW'
              ? 'border-accent-1 text-accent-1'
              : 'border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'
          }`}
        >
          <Wallet className="w-3.5 h-3.5" />
          Holding Asset Allocations
        </button>
        <button
          onClick={() => setActiveTab('TX_LEDGER')}
          className={`pb-2 px-4 border-b-2 font-bold cursor-pointer flex items-center gap-1.5 transition-all ${
            activeTab === 'TX_LEDGER'
              ? 'border-accent-1 text-accent-1'
              : 'border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'
          }`}
        >
          <Activity className="w-3.5 h-3.5" />
          Deposit & Withdrawal Ledgers
        </button>
      </div>

      {/* TAB content */}
      {activeTab === 'OVERVIEW' ? (
        <div className="grid grid-cols-1 lg:grid-cols-23 gap-5">
          
          {/* Allocation Table (left) */}
          <div className="lg:col-span-14 bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg p-4 space-y-3">
            <h3 className="text-xs font-bold font-display uppercase tracking-wider text-[#7e8c9a]">
              Spot Wallet Assets ({balances.length})
            </h3>
            
            <div className="overflow-x-auto">
              <table className="w-full text-left font-mono text-xs">
                <thead>
                  <tr className="border-b border-[#e1e4e8]/60 dark:border-[#21262d]/60 text-gray-400 text-[10px] uppercase">
                    <th className="pb-2">Asset</th>
                    <th className="pb-2">Name</th>
                    <th className="pb-2 text-right">Available Cash</th>
                    <th className="pb-2 text-right">In Orders</th>
                    <th className="pb-2 text-right">Estimated Value (USD)</th>
                    <th className="pb-2 text-right">Holdings Allocation %</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-50 dark:divide-[#21262d]/30">
                  {balances.map((item) => {
                    const totalAssetAmount = item.free + item.locked;
                    const allocationPct = totalBalanceUsd > 0 ? (item.valueUsd / totalBalanceUsd) * 100 : 0;
                    return (
                      <tr key={item.asset} className="hover:bg-gray-50 dark:hover:bg-[#161b22]/30 transition-colors">
                        <td className="py-2.5 font-bold text-gray-950 dark:text-gray-50">{item.asset}</td>
                        <td className="py-2.5 text-[#7e8c9a]">{item.name}</td>
                        <td className="py-2.5 text-right font-medium">{item.free.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 6 })}</td>
                        <td className="py-2.5 text-right text-amber-500 font-medium">
                          {item.locked > 0 ? item.locked.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 6 }) : '0.00'}
                        </td>
                        <td className="py-2.5 text-right font-bold text-gray-900 dark:text-gray-100">
                          ${item.valueUsd.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                        </td>
                        <td className="py-2.5 text-right font-semibold">
                          <div className="flex items-center justify-end gap-1.5">
                            <span className="w-8">{allocationPct.toFixed(1)}%</span>
                            <div className="w-12 h-1.5 bg-gray-100 dark:bg-slate-800 rounded-full overflow-hidden">
                              <div
                                className="h-full bg-accent-1"
                                style={{ width: `${allocationPct}%` }}
                              />
                            </div>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>

          {/* Quick Simulated Deposits Form Panel (right) */}
          <div className="lg:col-span-9 bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg p-5">
            <h3 className="text-xs font-bold font-display uppercase tracking-wider text-[#7e8c9a] mb-4 flex items-center gap-1">
              <PlusCircle className="w-4 h-4 text-accent-1" />
              Simulated Funds Station
            </h3>

            <p className="text-[10px] text-gray-400 font-mono mb-4 leading-normal">
              Need more buying power to run strategy simulations? Deposit fake USD or base crypto assets directly, or pull mock coins into external secured hardware ledgers.
            </p>

            <form onSubmit={handleActionSubmit} className="space-y-4">
              
              {/* Form type switcher */}
              <div className="grid grid-cols-2 gap-2 p-1 bg-slate-50 dark:bg-[#0d1117] rounded border border-[#e1e4e8] dark:border-[#21262d]">
                <button
                  type="button"
                  onClick={() => setActionType('DEPOSIT')}
                  className={`py-1 text-[10px] font-mono rounded cursor-pointer transition-all ${
                    actionType === 'DEPOSIT'
                      ? 'bg-accent-1 text-white shadow-sm font-semibold'
                      : 'text-gray-400'
                  }`}
                >
                  Simulate Deposit (+)
                </button>
                <button
                  type="button"
                  onClick={() => setActionType('WITHDRAW')}
                  className={`py-1 text-[10px] font-mono rounded cursor-pointer transition-all ${
                    actionType === 'WITHDRAW'
                      ? 'bg-accent-1 text-white shadow-sm font-semibold'
                      : 'text-gray-400'
                  }`}
                >
                  Simulate Withdrawal (-)
                </button>
              </div>

              {/* Asset Select */}
              <div>
                <label className="block text-[9px] uppercase tracking-wider text-gray-400 font-mono mb-1">Target Asset Asset</label>
                <select
                  value={selectedAsset}
                  onChange={(e) => setSelectedAsset(e.target.value)}
                  className="w-full bg-[#fafbfc] dark:bg-[#0d1117] border border-[#e1e4e8] dark:border-[#21262d] rounded px-3 py-1.5 font-mono text-xs focus:outline-none focus:border-accent-1 cursor-pointer"
                >
                  {balances.map(b => (
                    <option key={b.asset} value={b.asset}>{b.asset} - {b.name}</option>
                  ))}
                </select>
              </div>

              {/* Amount Input */}
              <div>
                <label className="block text-[9px] uppercase tracking-wider text-gray-400 font-mono mb-1">Quantity</label>
                <input
                  type="number"
                  step="any"
                  placeholder="0.00"
                  required
                  value={formAmount}
                  onChange={(e) => setFormAmount(e.target.value)}
                  className="w-full bg-[#fafbfc] dark:bg-[#0d1117] border border-[#e1e4e8] dark:border-[#21262d] rounded px-3 py-1.5 font-mono text-xs focus:ring-1 focus:ring-accent-1 focus:border-accent-1 focus:outline-none"
                />
              </div>

              {/* Notification warnings */}
              {errorMessage && (
                <div className="p-2 bg-rose-50 dark:bg-rose-950/20 border border-rose-100 dark:border-rose-900/30 font-mono text-[9px] text-trade-red rounded flex gap-1.5">
                  <AlertCircle className="w-3.5 h-3.5 shrink-0" />
                  <span>{errorMessage}</span>
                </div>
              )}

              {successMsg && (
                <div className="p-2 bg-emerald-50 dark:bg-emerald-950/20 border border-emerald-100 dark:border-emerald-900/30 font-mono text-[9px] text-trade-green rounded flex gap-1.5">
                  <ShieldCheck className="w-3.5 h-3.5 shrink-0" />
                  <span>{successMsg}</span>
                </div>
              )}

              <button
                type="submit"
                className="w-full py-2.5 bg-accent-1 hover:bg-accent-1-hovered text-white text-[11px] font-mono font-bold rounded cursor-pointer transition-all uppercase tracking-wider"
              >
                {actionType === 'DEPOSIT' ? 'Settle Inward Transfer' : 'Publish Hardware Withdrawal'}
              </button>

            </form>
          </div>

        </div>
      ) : (
        /* ACCOUNT TRANSACTION LOGS LEDGER */
        <div className="bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg p-4 space-y-3">
          <h3 className="text-xs font-bold font-display uppercase tracking-wider text-[#7e8c9a]">
            Simulated Ledger Transactions History
          </h3>

          <div className="overflow-x-auto font-mono text-xs">
            {transactions.length === 0 ? (
              <div className="p-10 text-center text-gray-400 italic">
                No simulated deposit or withdrawals recorded during this workspace session.
              </div>
            ) : (
              <table className="w-full text-left">
                <thead>
                  <tr className="border-b border-gray-100 dark:border-[#21262d]/50 text-gray-400 text-[10px] uppercase">
                    <th className="pb-1.5">Tx ID</th>
                    <th>Type</th>
                    <th>Asset</th>
                    <th className="text-right">Transaction Amount</th>
                    <th className="text-right">Settled System Timestamp</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-50 dark:divide-gray-800/40">
                  {transactions.map((tx) => (
                    <tr key={tx.id} className="hover:bg-slate-50/50 dark:hover:bg-slate-900/10 transition-colors">
                      <td className="py-2.5 text-gray-400 font-mono text-[10px]">{tx.id}</td>
                      <td>
                        <span className={`px-1.5 py-0.5 rounded text-[9px] font-bold ${
                          tx.type === 'DEPOSIT' ? 'text-trade-green bg-trade-green-bg' : 'text-trade-red bg-trade-red-bg'
                        }`}>
                          {tx.type}
                        </span>
                      </td>
                      <td className="font-bold">{tx.asset}</td>
                      <td className={`text-right font-semibold ${tx.type === 'DEPOSIT' ? 'text-trade-green' : 'text-trade-red'}`}>
                        {tx.type === 'DEPOSIT' ? '+' : '-'}
                        {tx.amount.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 6 })} {tx.asset}
                      </td>
                      <td className="text-right text-gray-400 text-[10px]">{tx.time.toLocaleDateString()} {tx.time.toLocaleTimeString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      )}

    </div>
  );
}
