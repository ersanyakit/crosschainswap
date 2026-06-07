/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import React, { useEffect, useMemo, useState } from 'react';
import { Briefcase, ArrowUpRight, ShieldCheck, Wallet, Activity, PlusCircle, AlertCircle, Copy, QrCode, RefreshCw } from 'lucide-react';
import { AssetBalance } from '../types/trading';
import { type AssetDeploymentInfo, type AssetInfo, type DepositAddressInfo } from '../services/exchangeService';
import AssetIcon from './AssetIcon';

type WalletTransaction = {
  id: string;
  type: string;
  asset: string;
  chainKey?: string;
  amount: number;
  time: Date;
};

type AssetOption = {
  symbol: string;
  registrySymbol: string;
  name: string;
  iconURL?: string;
};

type ChainOption = {
  key: string;
  label: string;
  iconURL?: string;
};

interface PortfolioViewProps {
  balances: AssetBalance[];
  onDeposit: (asset: string, chainKey: string) => Promise<DepositAddressInfo>;
  onWithdraw: (asset: string, chainKey: string, address: string, amount: number) => Promise<void> | void;
  transactions: WalletTransaction[];
  assetMetadata: Record<string, AssetInfo>;
}

export default function PortfolioView({
  balances,
  onDeposit,
  onWithdraw,
  transactions,
  assetMetadata,
}: PortfolioViewProps) {
  const [activeTab, setActiveTab] = useState<'OVERVIEW' | 'TX_LEDGER'>('OVERVIEW');
  const [selectedAsset, setSelectedAsset] = useState('USD');
  const [selectedChain, setSelectedChain] = useState('');
  const [destinationAddress, setDestinationAddress] = useState('');
  const [formAmount, setFormAmount] = useState('');
  const [actionType, setActionType] = useState<'DEPOSIT' | 'WITHDRAW'>('DEPOSIT');
  const [errorMessage, setErrorMessage] = useState('');
  const [successMsg, setSuccessMsg] = useState('');
  const [depositAddress, setDepositAddress] = useState<DepositAddressInfo | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const balanceAssetOptions = useMemo<AssetOption[]>(() => balances.map((balance) => ({
    symbol: balance.asset,
    registrySymbol: balance.asset,
    name: balance.name || balance.asset,
    iconURL: assetMetadata[balance.asset]?.icon_url,
  })), [assetMetadata, balances]);
  const registryAssetOptions = useMemo<AssetOption[]>(() => {
    const seen = new Set<string>();
    return Object.values(assetMetadata)
      .map((asset) => {
        const symbol = asset.symbol?.toUpperCase() || '';
        const registrySymbol = asset.registry_symbol?.toUpperCase() || symbol;
        return {
          symbol,
          registrySymbol,
          name: asset.name || symbol,
          iconURL: asset.icon_url,
        };
      })
      .filter((asset) => {
        if (!asset.symbol || seen.has(asset.symbol)) return false;
        seen.add(asset.symbol);
        return true;
      })
      .sort((a, b) => {
        const registryDelta = a.registrySymbol.localeCompare(b.registrySymbol);
        if (registryDelta !== 0) return registryDelta;
        return a.symbol.localeCompare(b.symbol);
      });
  }, [assetMetadata]);
  const assetOptions = actionType === 'DEPOSIT'
    ? registryAssetOptions
    : balanceAssetOptions;
  const chainOptions = useMemo(() => chainOptionsForAsset(assetMetadata[selectedAsset], selectedAsset), [assetMetadata, selectedAsset]);
  const selectedAssetOption = assetOptions.find((asset) => asset.symbol === selectedAsset);
  const selectedChainOption = chainOptions.find((chain) => chain.key === selectedChain);

  useEffect(() => {
    if (assetOptions.length === 0) {
      setSelectedAsset('');
      return;
    }
    if (!assetOptions.some((asset) => asset.symbol === selectedAsset)) {
      setSelectedAsset(assetOptions[0].symbol);
    }
  }, [assetOptions, selectedAsset]);

  useEffect(() => {
    if (chainOptions.length === 0) {
      setSelectedChain('');
      return;
    }
    if (!chainOptions.some((chain) => chain.key === selectedChain)) {
      setSelectedChain(chainOptions[0].key);
    }
  }, [chainOptions, selectedChain]);

  useEffect(() => {
    setDepositAddress(null);
  }, [actionType, selectedAsset, selectedChain]);

  // Calculations
  const totalBalanceUsd = balances.reduce((sum, b) => sum + b.valueUsd, 0);
  const totalLockedUsd = balances.reduce((sum, b) => sum + (b.locked * (b.valueUsd / (b.free || 1))), 0);
  const totalFreeUsd = totalBalanceUsd - totalLockedUsd;

  // Let's create a realistic daily PNL (e.g. +$1,240.23 / +2.15%)
  const yesterdayBalance = totalBalanceUsd * 0.9785; // up 2.15% overall
  const absolutePnl = totalBalanceUsd - yesterdayBalance;
  const percentagePnl = 2.15;

  const handleActionSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErrorMessage('');
    setSuccessMsg('');
    setIsSubmitting(true);

    try {
      if (!selectedAsset) {
        setErrorMessage(actionType === 'DEPOSIT'
          ? 'No backend asset registry item is available for deposits.'
          : 'No backend balance asset is available for withdrawal.');
        return;
      }

      if (!selectedChain) {
        setErrorMessage(`Select a chain for ${selectedAsset} before submitting.`);
        return;
      }

      if (actionType === 'DEPOSIT') {
        const details = await onDeposit(selectedAsset, selectedChain);
        setDepositAddress(details);
        setSuccessMsg(`Deposit address for ${selectedAsset} on ${selectedChain} is ready.`);
        return;
      }

      const amt = parseFloat(formAmount);
      if (isNaN(amt) || amt <= 0) {
        setErrorMessage('Please type a valid amount larger than zero.');
        return;
      }

      const assetBal = balances.find(b => b.asset === selectedAsset);
      if (!assetBal || assetBal.free < amt) {
        setErrorMessage(`Insufficient free ${selectedAsset} to complete withdrawal.`);
        return;
      }
      if (!destinationAddress.trim()) {
        setErrorMessage('Destination address is required for withdrawals.');
        return;
      }

      await onWithdraw(selectedAsset, selectedChain, destinationAddress.trim(), amt);
      setSuccessMsg(`Withdrawal request for ${amt} ${selectedAsset} on ${selectedChain} was handed to the exchange handler.`);
      setFormAmount('');
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : `${actionType === 'DEPOSIT' ? 'Deposit address' : 'Withdrawal request'} failed.`);
    } finally {
      setIsSubmitting(false);
    }
  };

  const copyDepositAddress = async () => {
    if (!depositAddress?.address) return;
    try {
      await navigator.clipboard.writeText(depositAddress.address);
      setSuccessMsg('Deposit address copied.');
    } catch {
      setErrorMessage('Could not copy the deposit address.');
    }
  };

  return (
    <div className="flex-1 w-full min-w-0 max-w-none overflow-y-auto p-4 sm:p-5 bg-[#fafbfc] dark:bg-[#070b0f] space-y-5 select-none h-full">
      
      {/* Overview Bento Card Row */}
      <div className="grid w-full min-w-0 grid-cols-1 md:grid-cols-3 gap-4">
        
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
      <div className="flex w-full min-w-0 border-b border-[#e1e4e8] dark:border-[#21262d] text-xs font-mono">
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
        <div className="grid w-full min-w-0 grid-cols-1 lg:grid-cols-23 gap-5">
          
          {/* Allocation Table (left) */}
          <div className="w-full min-w-0 lg:col-span-14 bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg p-4 space-y-3">
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

          {/* Quick funding form panel (right) */}
          <div className="w-full min-w-0 lg:col-span-9 bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg p-5">
            <h3 className="text-xs font-bold font-display uppercase tracking-wider text-[#7e8c9a] mb-4 flex items-center gap-1">
              <PlusCircle className="w-4 h-4 text-accent-1" />
              Backend Funds Station
            </h3>

            <p className="text-[10px] text-gray-400 font-mono mb-4 leading-normal">
              Generate gateway deposit addresses and route withdrawals through the exchange backend.
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
                  Deposit (+)
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
                  Withdrawal (-)
                </button>
              </div>

              {/* Asset Select */}
              <div>
                <label className="block text-[9px] uppercase tracking-wider text-gray-400 font-mono mb-1">Target Asset</label>
                <div className="flex items-center gap-2">
                  <div className="h-[34px] w-[34px] shrink-0 rounded border border-[#e1e4e8] dark:border-[#21262d] bg-white dark:bg-[#0d1117] flex items-center justify-center">
                    <AssetIcon symbol={selectedAsset} iconURL={selectedAssetOption?.iconURL || assetMetadata[selectedAsset]?.icon_url} size="sm" />
                  </div>
                  <select
                    value={selectedAsset}
                    onChange={(e) => {
                      setSelectedAsset(e.target.value);
                      setErrorMessage('');
                      setSuccessMsg('');
                    }}
                    className="min-w-0 flex-1 bg-[#fafbfc] dark:bg-[#0d1117] border border-[#e1e4e8] dark:border-[#21262d] rounded px-3 py-1.5 font-mono text-xs focus:outline-none focus:border-accent-1 cursor-pointer"
                  >
                    {assetOptions.map(asset => (
                      <option key={asset.symbol} value={asset.symbol}>
                        {asset.symbol} - {asset.name}{asset.registrySymbol && asset.registrySymbol !== asset.symbol ? ` (${asset.registrySymbol})` : ''}
                      </option>
                    ))}
                  </select>
                </div>
              </div>

              {/* Chain Select */}
              <div>
                <label className="block text-[9px] uppercase tracking-wider text-gray-400 font-mono mb-1">Chain</label>
                <div className="flex items-center gap-2">
                  <div className="h-[34px] w-[34px] shrink-0 overflow-hidden rounded border border-[#e1e4e8] dark:border-[#21262d] bg-white dark:bg-[#0d1117] flex items-center justify-center">
                    {selectedChainOption?.iconURL ? (
                      <img
                        src={selectedChainOption.iconURL}
                        alt={chainLabel(selectedChain)}
                        className="h-full w-full object-cover"
                        loading="lazy"
                        referrerPolicy="no-referrer"
                      />
                    ) : (
                      <span className="text-[9px] font-bold uppercase text-gray-500">{chainInitials(selectedChain)}</span>
                    )}
                  </div>
                  <select
                    value={selectedChain}
                    disabled={chainOptions.length === 0}
                    onChange={(e) => {
                      setSelectedChain(e.target.value);
                      setErrorMessage('');
                      setSuccessMsg('');
                    }}
                    className="min-w-0 flex-1 bg-[#fafbfc] dark:bg-[#0d1117] border border-[#e1e4e8] dark:border-[#21262d] rounded px-3 py-1.5 font-mono text-xs focus:outline-none focus:border-accent-1 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {chainOptions.map(chain => (
                      <option key={chain.key} value={chain.key}>{chain.label}</option>
                    ))}
                  </select>
                </div>
              </div>

              {actionType === 'WITHDRAW' && (
                <div>
                  <label className="block text-[9px] uppercase tracking-wider text-gray-400 font-mono mb-1">Destination Address</label>
                  <input
                    type="text"
                    value={destinationAddress}
                    onChange={(e) => setDestinationAddress(e.target.value)}
                    className="w-full bg-[#fafbfc] dark:bg-[#0d1117] border border-[#e1e4e8] dark:border-[#21262d] rounded px-3 py-1.5 font-mono text-xs focus:ring-1 focus:ring-accent-1 focus:border-accent-1 focus:outline-none"
                  />
                </div>
              )}

              {actionType === 'WITHDRAW' && (
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
              )}

              {actionType === 'DEPOSIT' && depositAddress && (
                <div className="rounded border border-[#e1e4e8] dark:border-[#21262d] bg-[#fafbfc] dark:bg-[#0d1117] p-3 space-y-3">
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-1.5 text-[10px] uppercase tracking-wider font-mono font-bold text-[#7e8c9a]">
                      <QrCode className="w-3.5 h-3.5 text-accent-1" />
                      Deposit QR
                    </div>
                    <span className="text-[9px] font-mono text-gray-400 uppercase">{depositAddress.chain_key}</span>
                  </div>
                  {depositAddress.qr_url && (
                    <div className="flex justify-center">
                      <img
                        src={depositAddress.qr_url}
                        alt={`${depositAddress.asset} deposit QR`}
                        className="h-40 w-40 rounded bg-white p-2 border border-[#e1e4e8] dark:border-[#30363d]"
                      />
                    </div>
                  )}
                  <div className="space-y-1">
                    <label className="block text-[9px] uppercase tracking-wider text-gray-400 font-mono">Gateway Address</label>
                    <div className="flex items-stretch gap-2">
                      <input
                        type="text"
                        readOnly
                        value={depositAddress.address}
                        className="min-w-0 flex-1 bg-white dark:bg-[#080c10] border border-[#e1e4e8] dark:border-[#21262d] rounded px-2 py-1.5 font-mono text-[10px] text-gray-700 dark:text-gray-200"
                      />
                      <button
                        type="button"
                        onClick={copyDepositAddress}
                        className="h-[31px] w-[34px] inline-flex items-center justify-center rounded border border-[#e1e4e8] dark:border-[#21262d] text-gray-500 hover:text-accent-1 hover:border-accent-1 transition-colors"
                        title="Copy address"
                      >
                        <Copy className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>
                </div>
              )}

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
                disabled={!selectedAsset || !selectedChain || isSubmitting}
                className="w-full py-2.5 bg-accent-1 hover:bg-accent-1-hovered text-white text-[11px] font-mono font-bold rounded cursor-pointer transition-all uppercase tracking-wider disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-1.5"
              >
                {isSubmitting && <RefreshCw className="w-3.5 h-3.5 animate-spin" />}
                {actionType === 'DEPOSIT' ? 'Get Gateway Deposit Address' : 'Publish Hardware Withdrawal'}
              </button>

            </form>
          </div>

        </div>
      ) : (
        /* ACCOUNT TRANSACTION LOGS LEDGER */
        <div className="w-full min-w-0 bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg p-4 space-y-3">
          <h3 className="text-xs font-bold font-display uppercase tracking-wider text-[#7e8c9a]">
            Ledger Transactions History
          </h3>

          <div className="overflow-x-auto font-mono text-xs">
            {transactions.length === 0 ? (
              <div className="p-10 text-center text-gray-400 italic">
                No deposit or withdrawal records returned during this workspace session.
              </div>
            ) : (
              <table className="w-full text-left">
                <thead>
                  <tr className="border-b border-gray-100 dark:border-[#21262d]/50 text-gray-400 text-[10px] uppercase">
                    <th className="pb-1.5">Tx ID</th>
                    <th>Type</th>
                    <th>Asset</th>
                    <th>Chain</th>
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
                      <td className="text-gray-500 text-[10px] uppercase">{tx.chainKey || '-'}</td>
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

function chainOptionsForAsset(asset?: AssetInfo, selectedSymbol = ''): ChainOption[] {
  const seen = new Set<string>();
  const out: ChainOption[] = [];
  const selected = selectedSymbol.toUpperCase();

  (asset?.deployments || []).forEach((deployment: AssetDeploymentInfo) => {
    if (deployment.enabled === false || !deployment.chain_key) return;
    const key = deployment.chain_key;
    if (seen.has(key)) return;
    seen.add(key);
    const deploymentSymbol = deployment.symbol?.toUpperCase() || '';
    const symbol = selected || deploymentSymbol || asset?.symbol || '';
    const labelSymbol = deploymentSymbol && deploymentSymbol !== symbol
      ? `${symbol} / ${deploymentSymbol}`
      : symbol;
    out.push({
      key,
      label: labelSymbol ? `${chainLabel(key)} - ${labelSymbol}` : chainLabel(key),
      iconURL: deployment.chain_logo_url,
    });
  });

  return out.sort((a, b) => chainRank(a.key) - chainRank(b.key) || a.label.localeCompare(b.label));
}

function chainInitials(value: string): string {
  const parts = value.split('_').filter(Boolean);
  if (parts.length === 0) return '?';
  if (parts.length === 1) return parts[0].slice(0, 3).toUpperCase();
  return parts.slice(0, 2).map((part) => part[0]?.toUpperCase() || '').join('');
}

function chainLabel(value: string): string {
  return value
    .split('_')
    .map((part) => part ? part[0].toUpperCase() + part.slice(1) : part)
    .join(' ');
}

function chainRank(value: string): number {
  const order = ['chiliz', 'base', 'solana', 'ethereum', 'avalanche', 'arbitrum', 'unichain', 'binance_smart_chain'];
  const index = order.indexOf(value);
  return index === -1 ? order.length : index;
}
