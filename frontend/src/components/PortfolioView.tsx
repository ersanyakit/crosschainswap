/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import React, { useEffect, useMemo, useState } from 'react';
import { Briefcase, ShieldCheck, Wallet, Activity, PlusCircle, AlertCircle, Copy, QrCode, RefreshCw, ChevronDown, Check } from 'lucide-react';
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
  native?: boolean;
};

interface PortfolioViewProps {
  balances: AssetBalance[];
  balancesError: string | null;
  onDeposit: (asset: string, chainKey: string) => Promise<DepositAddressInfo>;
  onWithdraw: (asset: string, chainKey: string, address: string, amount: number) => Promise<void> | void;
  transactions: WalletTransaction[];
  assetMetadata: Record<string, AssetInfo>;
}

export default function PortfolioView({
  balances,
  balancesError,
  onDeposit,
  onWithdraw,
  transactions,
  assetMetadata,
}: PortfolioViewProps) {
  const [activeTab, setActiveTab] = useState<'HOLDINGS' | 'FUNDS' | 'LEDGER'>('HOLDINGS');
  const [selectedAsset, setSelectedAsset] = useState('USD');
  const [selectedChain, setSelectedChain] = useState('');
  const [destinationAddress, setDestinationAddress] = useState('');
  const [formAmount, setFormAmount] = useState('');
  const [actionType, setActionType] = useState<'DEPOSIT' | 'WITHDRAW'>('DEPOSIT');
  const [errorMessage, setErrorMessage] = useState('');
  const [successMsg, setSuccessMsg] = useState('');
  const [depositAddress, setDepositAddress] = useState<DepositAddressInfo | null>(null);
  const [qrImageError, setQrImageError] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const registryAssetOptions = useMemo<AssetOption[]>(() => {
    const byRegistry = new Map<string, AssetOption>();
    Object.values(assetMetadata).forEach((asset) => {
      const registrySymbol = asset.registry_symbol?.toUpperCase() || asset.symbol?.toUpperCase() || '';
      if (!registrySymbol || byRegistry.has(registrySymbol)) return;
      const registryAsset = assetMetadata[registrySymbol] || asset;
      byRegistry.set(registrySymbol, {
        symbol: registrySymbol,
        registrySymbol,
        name: registryAsset.name || asset.name || registrySymbol,
        iconURL: registryAsset.icon_url || asset.icon_url,
      });
    });
    return Array.from(byRegistry.values()).sort((a, b) => a.symbol.localeCompare(b.symbol));
  }, [assetMetadata]);
  const balanceRows = useMemo(() => aggregateBalancesByAsset(balances, assetMetadata), [assetMetadata, balances]);
  const balanceAssetOptions = useMemo<AssetOption[]>(() => balanceRows.map((balance) => ({
    symbol: balance.asset,
    registrySymbol: balance.asset,
    name: balance.name || balance.asset,
    iconURL: assetMetadata[balance.asset]?.icon_url,
  })), [assetMetadata, balanceRows]);
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
      const nextAsset = assetOptions[0].symbol;
      setSelectedAsset(nextAsset);
      setSelectedChain(firstChainKeyForAsset(assetMetadata, nextAsset));
    }
  }, [assetMetadata, assetOptions, selectedAsset]);

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
  const totalBalanceUsd = balanceRows.reduce((sum, b) => sum + b.valueUsd, 0);
  const totalLockedUsd = balanceRows.reduce((sum, b) => sum + balanceAmountValueUsd(b, balanceLockedAmount(b)), 0);
  const totalAvailableUsd = balanceRows.reduce((sum, b) => sum + balanceAmountValueUsd(b, balanceAvailableAmount(b)), 0);
  const depositQRURL = depositAddress?.qr_url || '';

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
        setQrImageError(false);
        setSuccessMsg(`Deposit address for ${selectedAsset} on ${selectedChain} is ready.`);
        return;
      }

      const amt = parseFloat(formAmount);
      if (isNaN(amt) || amt <= 0) {
        setErrorMessage('Please type a valid amount larger than zero.');
        return;
      }

      const assetBal = balanceRows.find(b => b.asset === selectedAsset);
      if (!assetBal || balanceAvailableAmount(assetBal) < amt) {
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

          <div className="mt-4 text-[10px] font-mono text-gray-400">
            {balanceRows.length} asset{balanceRows.length === 1 ? '' : 's'}
          </div>
        </div>

        {/* Card 2: Available */}
        <div className="p-5 bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm flex flex-col justify-between">
          <div>
            <span className="block text-[10px] uppercase font-mono tracking-widest text-[#7e8c9a] font-bold mb-1">
              Available Balance (USD)
            </span>
            <span className="text-2xl sm:text-3xl font-display font-black text-gray-950 dark:text-gray-50">
              ${totalAvailableUsd.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
            </span>
          </div>
          <div className="mt-3 text-[10px] font-mono text-gray-400 border-t border-gray-100 dark:border-gray-800/60 pt-2 flex items-center gap-1.5">
            <ShieldCheck className="w-3.5 h-3.5 text-trade-green shrink-0" />
            Withdrawable balance across listed assets.
          </div>
        </div>

        {/* Card 3: Locked */}
        <div className="p-5 bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm flex flex-col justify-between">
          <div>
            <span className="block text-[10px] uppercase font-mono tracking-widest text-[#7e8c9a] font-bold mb-1">
              Locked Balance (USD)
            </span>
            <span className="text-2xl sm:text-3xl font-display font-black text-amber-500">
              ${totalLockedUsd.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
            </span>
          </div>
          <div className="mt-3 text-[10px] font-mono text-gray-400 border-t border-gray-100 dark:border-gray-800/60 pt-2">
            Reserved by open orders.
          </div>
        </div>

      </div>

      {/* Primary Panels tabs */}
      <div className="flex w-full min-w-0 border-b border-[#e1e4e8] dark:border-[#21262d] text-xs font-mono">
        <button
          onClick={() => setActiveTab('HOLDINGS')}
          className={`pb-2 px-4 border-b-2 font-bold cursor-pointer flex items-center gap-1.5 transition-all ${
            activeTab === 'HOLDINGS'
              ? 'border-accent-1 text-accent-1'
              : 'border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'
          }`}
        >
          <Wallet className="w-3.5 h-3.5" />
          Holding Asset Allocations
        </button>
        <button
          onClick={() => setActiveTab('FUNDS')}
          className={`pb-2 px-4 border-b-2 font-bold cursor-pointer flex items-center gap-1.5 transition-all ${
            activeTab === 'FUNDS'
              ? 'border-accent-1 text-accent-1'
              : 'border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'
          }`}
        >
          <PlusCircle className="w-3.5 h-3.5" />
          Deposit / Withdrawal
        </button>
        <button
          onClick={() => setActiveTab('LEDGER')}
          className={`pb-2 px-4 border-b-2 font-bold cursor-pointer flex items-center gap-1.5 transition-all ${
            activeTab === 'LEDGER'
              ? 'border-accent-1 text-accent-1'
              : 'border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'
          }`}
        >
          <Activity className="w-3.5 h-3.5" />
          Ledger
        </button>
      </div>

      {/* TAB content */}
      {activeTab === 'HOLDINGS' ? (
        <div className="grid w-full min-w-0 grid-cols-1 gap-5">
          {/* Allocation Table (left) */}
          <div className="w-full min-w-0 bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg p-4 space-y-3">
            <h3 className="text-xs font-bold font-display uppercase tracking-wider text-[#7e8c9a]">
              Holding Asset Allocations ({balanceRows.length})
            </h3>
            
            <div className="overflow-x-auto">
              <table className="w-full text-left font-mono text-xs">
                <thead>
                  <tr className="border-b border-[#e1e4e8]/60 dark:border-[#21262d]/60 text-gray-400 text-[10px] uppercase">
                    <th className="pb-2">Asset</th>
                    <th className="pb-2 text-right">Locked</th>
                    <th className="pb-2 text-right">Available</th>
                    <th className="pb-2 text-right">Total</th>
                    <th className="pb-2 text-right">Value (USD)</th>
                    <th className="pb-2 text-right">Allocation</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-50 dark:divide-[#21262d]/30">
                  {balanceRows.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="py-8 text-center text-[10px] text-gray-400">
                        {balancesError || 'No exchange balances registered for this account.'}
                      </td>
                    </tr>
                  ) : balanceRows.map((item) => {
                    const allocationPct = totalBalanceUsd > 0 ? (item.valueUsd / totalBalanceUsd) * 100 : 0;
                    const lockedAmount = balanceLockedAmount(item);
                    const availableAmount = balanceAvailableAmount(item);
                    const totalAmount = balanceTotalAmount(item);
                    return (
                      <tr key={item.asset} className="hover:bg-gray-50 dark:hover:bg-[#161b22]/30 transition-colors">
                        <td className="py-2.5 pr-3">
                          <div className="flex min-w-[140px] items-center gap-2">
                            <AssetIcon symbol={item.asset} iconURL={assetMetadata[item.asset]?.icon_url} size="xs" />
                            <div className="min-w-0">
                              <div className="font-bold text-gray-950 dark:text-gray-50">{item.asset}</div>
                              <div className="truncate text-[9px] text-[#7e8c9a]">{item.name}</div>
                            </div>
                          </div>
                        </td>
                        <td className="py-2.5 text-right text-amber-500 font-medium">
                          {lockedAmount.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 6 })}
                        </td>
                        <td className="py-2.5 text-right font-medium">
                          {availableAmount.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 6 })}
                        </td>
                        <td className="py-2.5 text-right font-bold text-gray-950 dark:text-gray-50">
                          {totalAmount.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 6 })}
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
        </div>
      ) : activeTab === 'FUNDS' ? (
        <div className="grid w-full min-w-0 grid-cols-1 xl:grid-cols-[minmax(0,1fr)_minmax(360px,460px)] gap-5">
          <form
            onSubmit={handleActionSubmit}
            className={`w-full min-w-0 bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg p-5 space-y-5 ${actionType === 'WITHDRAW' || !depositAddress ? 'xl:col-span-2' : ''}`}
          >
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <h3 className="text-xs font-bold font-display uppercase tracking-wider text-[#7e8c9a] flex items-center gap-1">
                <PlusCircle className="w-4 h-4 text-accent-1" />
                Deposit / Withdrawal
              </h3>

              <div className="grid w-full grid-cols-2 gap-2 p-1 bg-slate-50 dark:bg-[#0d1117] rounded border border-[#e1e4e8] dark:border-[#21262d] sm:w-[260px]">
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
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
              <div>
                <label className="block text-[9px] uppercase tracking-wider text-gray-400 font-mono mb-1">Target Asset</label>
                <AssetDropdown
                  options={assetOptions}
                  value={selectedAsset}
                  selectedOption={selectedAssetOption}
                  fallbackIconURL={assetMetadata[selectedAsset]?.icon_url}
                  onChange={(symbol) => {
                    setSelectedAsset(symbol);
                    setSelectedChain(firstChainKeyForAsset(assetMetadata, symbol));
                    setErrorMessage('');
                    setSuccessMsg('');
                  }}
                />
              </div>

              <div>
                <label className="block text-[9px] uppercase tracking-wider text-gray-400 font-mono mb-1">Chain</label>
                <ChainDropdown
                  options={chainOptions}
                  value={selectedChain}
                  selectedOption={selectedChainOption}
                  onChange={(chainKey) => {
                    setSelectedChain(chainKey);
                    setErrorMessage('');
                    setSuccessMsg('');
                  }}
                />
              </div>
            </div>

              {actionType === 'WITHDRAW' && (
                <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_220px] gap-4">
                  <div>
                    <label className="block text-[9px] uppercase tracking-wider text-gray-400 font-mono mb-1">Destination Address</label>
                    <input
                      type="text"
                      value={destinationAddress}
                      onChange={(e) => setDestinationAddress(e.target.value)}
                      className="w-full bg-[#fafbfc] dark:bg-[#0d1117] border border-[#e1e4e8] dark:border-[#21262d] rounded px-3 py-2 font-mono text-xs focus:ring-1 focus:ring-accent-1 focus:border-accent-1 focus:outline-none"
                    />
                  </div>
                  <div>
                    <label className="block text-[9px] uppercase tracking-wider text-gray-400 font-mono mb-1">Quantity</label>
                    <input
                      type="number"
                      step="any"
                      placeholder="0.00"
                      required
                      value={formAmount}
                      onChange={(e) => setFormAmount(e.target.value)}
                      className="w-full bg-[#fafbfc] dark:bg-[#0d1117] border border-[#e1e4e8] dark:border-[#21262d] rounded px-3 py-2 font-mono text-xs focus:ring-1 focus:ring-accent-1 focus:border-accent-1 focus:outline-none"
                    />
                  </div>
                </div>
              )}

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
                {actionType === 'DEPOSIT' ? 'Get Deposit Address' : 'Submit Withdrawal'}
              </button>
          </form>

          {actionType === 'DEPOSIT' && depositAddress && (
            <div className="w-full min-w-0 bg-white dark:bg-[#0c1015] border border-[#d8dee4] dark:border-[#263241] rounded-lg p-5 space-y-4 shadow-sm">
              <div className="flex items-center justify-between gap-2">
                <h3 className="text-xs font-bold font-display uppercase tracking-wider text-[#7e8c9a] flex items-center gap-1.5">
                  <QrCode className="w-4 h-4 text-accent-1" />
                  Gateway QR
                </h3>
                <span className="text-[9px] font-mono text-gray-400 uppercase">
                  {depositAddress.asset} / {depositAddress.chain_key}
                </span>
              </div>

              <div className="grid grid-cols-1 gap-4 lg:grid-cols-[220px_minmax(0,1fr)] xl:grid-cols-1 2xl:grid-cols-[220px_minmax(0,1fr)]">
                <div className="flex items-center justify-center rounded-lg border border-[#e1e4e8] bg-[#f6f8fa] p-3 dark:border-[#263241] dark:bg-[#0d1117]">
                  {depositQRURL && !qrImageError ? (
                    <img
                      src={depositQRURL}
                      alt={`${depositAddress.asset} deposit QR`}
                      className="h-48 w-48 rounded-md border border-[#e1e4e8] bg-white p-2 dark:border-[#30363d]"
                      onError={() => setQrImageError(true)}
                    />
                  ) : (
                    <div className="flex h-48 w-48 flex-col items-center justify-center rounded-md border border-dashed border-[#d8dee4] bg-white px-4 text-center font-mono text-[10px] text-gray-400 dark:border-[#30363d] dark:bg-[#101720]">
                      <QrCode className="mb-2 h-6 w-6 text-gray-400" />
                      QR could not be loaded from gateway.
                    </div>
                  )}
                </div>

                <div className="min-w-0 space-y-3">
                  <div className="rounded border border-[#e1e4e8] bg-[#fafbfc] px-3 py-2 dark:border-[#263241] dark:bg-[#0d1117]">
                    <div className="mb-1 flex items-center justify-between gap-2">
                      <label className="block text-[9px] uppercase tracking-wider text-gray-400 font-mono">
                        Wallet Address
                      </label>
                      <span className="text-[9px] font-mono uppercase text-gray-400">{depositAddress.chain_key}</span>
                    </div>
                    <div className="flex items-stretch gap-2">
                      <input
                        type="text"
                        readOnly
                        value={depositAddress.address}
                        className="min-w-0 flex-1 bg-white dark:bg-[#101720] border border-[#e1e4e8] dark:border-[#30363d] rounded px-3 py-2 font-mono text-[10px] text-gray-700 dark:text-gray-200"
                      />
                      <button
                        type="button"
                        onClick={copyDepositAddress}
                        className="h-[34px] w-[38px] inline-flex items-center justify-center rounded border border-[#d8dee4] dark:border-[#30363d] text-gray-500 hover:text-accent-1 hover:border-accent-1 transition-colors"
                        title="Copy address"
                      >
                        <Copy className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>

                  <div className="rounded border border-emerald-200/70 bg-emerald-50/70 px-3 py-2 font-mono text-[10px] text-emerald-700 dark:border-emerald-400/20 dark:bg-emerald-400/10 dark:text-emerald-300">
                    Gateway static address is ready for this wallet.
                  </div>
                </div>
              </div>
            </div>
          )}
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

function AssetDropdown({
  options,
  value,
  selectedOption,
  fallbackIconURL,
  onChange,
}: {
  options: AssetOption[];
  value: string;
  selectedOption?: AssetOption;
  fallbackIconURL?: string;
  onChange: (symbol: string) => void;
}) {
  const [isOpen, setIsOpen] = useState(false);
  const disabled = options.length === 0;

  return (
    <div
      className="relative"
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
          setIsOpen(false);
        }
      }}
    >
      <button
        type="button"
        disabled={disabled}
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        onClick={() => setIsOpen((open) => !open)}
        className={`group flex h-10 w-full min-w-0 items-center gap-2 rounded-md border bg-white px-2.5 text-left shadow-xs transition-all hover:border-[#9aa4b2]/70 hover:bg-[#f8fafc] focus:outline-none focus:ring-2 focus:ring-slate-400/10 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-[#0d1117] dark:hover:bg-[#101720] ${
          isOpen
            ? 'border-[#8b949e]/70 dark:border-[#3b4654]'
            : 'border-[#d8dee4] dark:border-[#263241]'
        }`}
      >
        <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-slate-100 ring-1 ring-slate-200/80 dark:bg-[#151b23] dark:ring-[#2f3a48]">
          <AssetIcon symbol={value} iconURL={selectedOption?.iconURL || fallbackIconURL} size="sm" className="shadow-none" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate font-mono text-xs font-bold text-gray-950 dark:text-gray-50">
            {selectedOption?.symbol || value || 'No asset'}
          </span>
          <span className="block truncate font-mono text-[9px] text-gray-400">
            {selectedOption?.name || 'Asset registry unavailable'}
          </span>
        </span>
        <ChevronDown className={`h-3.5 w-3.5 shrink-0 text-gray-400 transition-transform ${isOpen ? 'rotate-180 text-accent-1' : ''}`} />
      </button>

      {isOpen && !disabled && (
        <div
          role="listbox"
          className="absolute left-0 right-0 top-[calc(100%+4px)] z-40 max-h-64 overflow-y-auto rounded-md border border-[#d8dee4] bg-white p-1 shadow-xl shadow-slate-900/10 dark:border-[#263241] dark:bg-[#0b1118] dark:shadow-black/35"
        >
          {options.map((asset) => {
            const isSelected = asset.symbol === value;
            return (
              <button
                key={asset.symbol}
                type="button"
                role="option"
                aria-selected={isSelected}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => {
                  onChange(asset.symbol);
                  setIsOpen(false);
                }}
                className={`flex w-full items-center gap-2 rounded px-2.5 py-1.5 text-left transition-colors ${
                  isSelected
                    ? 'bg-[#f6f8fa] text-gray-950 dark:bg-[#111820] dark:text-gray-50'
                    : 'text-gray-600 hover:bg-[#f6f8fa] hover:text-gray-950 dark:text-gray-300 dark:hover:bg-[#111820] dark:hover:text-gray-50'
                }`}
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-mono text-[11px] font-bold">{asset.symbol}</span>
                  <span className="block truncate font-mono text-[9px] text-gray-400">{asset.name}</span>
                </span>
                {isSelected && <Check className="h-3.5 w-3.5 shrink-0 text-accent-1" />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

function ChainDropdown({
  options,
  value,
  selectedOption,
  onChange,
}: {
  options: ChainOption[];
  value: string;
  selectedOption?: ChainOption;
  onChange: (chainKey: string) => void;
}) {
  const [isOpen, setIsOpen] = useState(false);
  const disabled = options.length === 0;

  return (
    <div
      className="relative"
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
          setIsOpen(false);
        }
      }}
    >
      <button
        type="button"
        disabled={disabled}
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        onClick={() => setIsOpen((open) => !open)}
        className={`group flex h-10 w-full min-w-0 items-center gap-2 rounded-md border bg-white px-2.5 text-left shadow-xs transition-all hover:border-[#9aa4b2]/70 hover:bg-[#f8fafc] focus:outline-none focus:ring-2 focus:ring-slate-400/10 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-[#0d1117] dark:hover:bg-[#101720] ${
          isOpen
            ? 'border-[#8b949e]/70 dark:border-[#3b4654]'
            : 'border-[#d8dee4] dark:border-[#263241]'
        }`}
      >
        <ChainMark option={selectedOption} value={value} />
        <span className="min-w-0 flex-1">
          <span className="block truncate font-mono text-xs font-bold text-gray-950 dark:text-gray-50">
            {selectedOption ? chainLabel(selectedOption.key) : 'No chain'}
          </span>
          <span className="block truncate font-mono text-[9px] text-gray-400">
            {selectedOption?.label || 'No enabled deployment'}
          </span>
        </span>
        <ChevronDown className={`h-3.5 w-3.5 shrink-0 text-gray-400 transition-transform ${isOpen ? 'rotate-180 text-accent-1' : ''}`} />
      </button>

      {isOpen && !disabled && (
        <div
          role="listbox"
          className="absolute left-0 right-0 top-[calc(100%+4px)] z-40 max-h-64 overflow-y-auto rounded-md border border-[#d8dee4] bg-white p-1 shadow-xl shadow-slate-900/10 dark:border-[#263241] dark:bg-[#0b1118] dark:shadow-black/35"
        >
          {options.map((chain) => {
            const isSelected = chain.key === value;
            return (
              <button
                key={chain.key}
                type="button"
                role="option"
                aria-selected={isSelected}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => {
                  onChange(chain.key);
                  setIsOpen(false);
                }}
                className={`flex w-full items-center gap-2 rounded px-2.5 py-1.5 text-left transition-colors ${
                  isSelected
                    ? 'bg-[#f6f8fa] text-gray-950 dark:bg-[#111820] dark:text-gray-50'
                    : 'text-gray-600 hover:bg-[#f6f8fa] hover:text-gray-950 dark:text-gray-300 dark:hover:bg-[#111820] dark:hover:text-gray-50'
                }`}
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-mono text-[11px] font-bold">{chainLabel(chain.key)}</span>
                  <span className="block truncate font-mono text-[9px] text-gray-400">{chain.label}</span>
                </span>
                {isSelected && <Check className="h-3.5 w-3.5 shrink-0 text-accent-1" />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

function ChainMark({ option, value }: { option?: ChainOption; value: string }) {
  return (
    <span className="flex h-7 w-7 shrink-0 items-center justify-center overflow-hidden rounded-full bg-slate-100 ring-1 ring-slate-200/80 dark:bg-[#151b23] dark:ring-[#2f3a48]">
      {option?.iconURL ? (
        <img
          src={option.iconURL}
          alt={chainLabel(option.key)}
          className="h-5 w-5 object-contain"
          loading="lazy"
          referrerPolicy="no-referrer"
        />
      ) : (
        <span className="text-[8px] font-bold uppercase text-gray-500">{chainInitials(value)}</span>
      )}
    </span>
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
      native: deployment.native === true,
    });
  });

  return out.sort((a, b) => {
    if (a.native !== b.native) return a.native ? -1 : 1;
    return chainRank(a.key) - chainRank(b.key) || a.label.localeCompare(b.label);
  });
}

function firstChainKeyForAsset(assetMetadata: Record<string, AssetInfo>, symbol: string): string {
  return chainOptionsForAsset(assetMetadata[symbol.toUpperCase()], symbol)[0]?.key || '';
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
  const order = ['bitcoin', 'ethereum', 'base', 'avalanche', 'binance_smart_chain', 'bnbchain', 'arbitrum', 'unichain', 'tron', 'solana', 'chiliz', 'chiliz_spicy'];
  const index = order.indexOf(value);
  return index === -1 ? order.length : index;
}

function aggregateBalancesByAsset(balances: AssetBalance[], assetMetadata: Record<string, AssetInfo>): AssetBalance[] {
  const rows = new Map<string, AssetBalance>();

  Object.values(assetMetadata).forEach((asset) => {
    const registrySymbol = asset.registry_symbol?.toUpperCase() || asset.symbol?.toUpperCase() || '';
    if (!registrySymbol || rows.has(registrySymbol)) return;
    const registryMetadata = assetMetadata[registrySymbol] || asset;
    rows.set(registrySymbol, {
      asset: registrySymbol,
      name: registryMetadata.name || asset.name || registrySymbol,
      free: 0,
      locked: 0,
      frozen: 0,
      valueUsd: 0,
      change24h: 0,
    });
  });

  balances.forEach((balance) => {
    const balanceSymbol = balance.asset?.toUpperCase() || '';
    if (!balanceSymbol) return;
    const metadata = assetMetadata[balanceSymbol];
    const registrySymbol = metadata?.registry_symbol?.toUpperCase() || balanceSymbol;
    const registryMetadata = assetMetadata[registrySymbol] || metadata;
    const current = rows.get(registrySymbol);
    const nextName = registryMetadata?.name || metadata?.name || balance.name || registrySymbol;

    if (!current) {
      rows.set(registrySymbol, {
        ...balance,
        asset: registrySymbol,
        name: nextName,
        free: finiteNumber(balance.free),
        locked: finiteNumber(balance.locked),
        frozen: finiteNumber(balance.frozen),
        valueUsd: finiteNumber(balance.valueUsd),
        change24h: finiteNumber(balance.change24h),
      });
      return;
    }

    current.free += finiteNumber(balance.free);
    current.locked += finiteNumber(balance.locked);
    current.frozen += finiteNumber(balance.frozen);
    current.valueUsd += finiteNumber(balance.valueUsd);
    if (!current.name || current.name === current.asset) {
      current.name = nextName;
    }
  });

  return Array.from(rows.values()).sort((a, b) => b.valueUsd - a.valueUsd || a.asset.localeCompare(b.asset));
}

function balanceAvailableAmount(balance: AssetBalance): number {
  return finiteNumber(balance.free);
}

function balanceLockedAmount(balance: AssetBalance): number {
  return finiteNumber(balance.locked) + finiteNumber(balance.frozen);
}

function balanceTotalAmount(balance: AssetBalance): number {
  return balanceAvailableAmount(balance) + balanceLockedAmount(balance);
}

function balanceAmountValueUsd(balance: AssetBalance, amount: number): number {
  const total = balanceTotalAmount(balance);
  return total > 0 ? (finiteNumber(balance.valueUsd) / total) * amount : 0;
}

function finiteNumber(value: number): number {
  return Number.isFinite(value) ? value : 0;
}
