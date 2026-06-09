/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import React, { useState, useEffect } from 'react';
import { ShieldAlert, AlertTriangle, AlertCircle, Sliders } from 'lucide-react';
import { MarketPair, OrderType, OrderSide } from '../types/trading';

const BASE_AMOUNT_DECIMALS = 8;
const ORDER_INPUT_DECIMALS = 8;
const BALANCE_EPSILON = 0.00000001;
const DISPLAY_ROUNDING_TOLERANCE = 0.0001;

interface OrderFormProps {
  pair: MarketPair;
  availableUsdt: number;
  availableBase: number;
  onSubmitOrder: (order: {
    side: OrderSide;
    type: OrderType;
    price: number;
    amount: number;
    stopPrice?: number;
  }) => void;
  selectedPrice: number | null;
  selectedAmount: number | null;
  clearSelectedPrice: () => void;
  submitError?: string | null;
  docked?: boolean;
}

export default function OrderForm({
  pair,
  availableUsdt,
  availableBase,
  onSubmitOrder,
  selectedPrice,
  selectedAmount,
  clearSelectedPrice,
  submitError,
  docked = false,
}: OrderFormProps) {
  const [side, setSide] = useState<OrderSide>('BUY');
  const [type, setType] = useState<OrderType>('LIMIT');
  const [priceInput, setPriceInput] = useState('');
  const [amountInput, setAmountInput] = useState('');
  const [stopPriceInput, setStopPriceInput] = useState('');
  
  // Track which input is active for the numeric terminal keyboard focus mapping
  const [activeInput, setActiveInput] = useState<'price' | 'amount' | 'stopPrice'>('amount');
  const [showKeypad, setShowKeypad] = useState(false);

  // Confirmation Modal
  const [showConfirm, setShowConfirm] = useState(false);

  useEffect(() => {
    setPriceInput(formatOrderNumberInput(pair.lastPrice));
    setAmountInput('');
    setStopPriceInput('');
    setShowConfirm(false);
    setActiveInput('amount');
  }, [pair.symbol]);

  // Keypad processing engine
  const handleKeypadPress = (val: string) => {
    let currentVal = '';
    let setVal: React.Dispatch<React.SetStateAction<string>>;

    if (activeInput === 'price') {
      currentVal = priceInput;
      setVal = setPriceInput;
    } else if (activeInput === 'stopPrice') {
      currentVal = stopPriceInput;
      setVal = setStopPriceInput;
    } else {
      currentVal = amountInput;
      setVal = setAmountInput;
    }

    if (val === '⌫') {
      setVal(currentVal.slice(0, -1));
    } else if (val === 'C') {
      setVal('');
    } else if (val === '.') {
      if (!currentVal.includes('.')) {
        setVal(currentVal ? currentVal + '.' : '0.');
      }
    } else if (val.startsWith('+')) {
      const inc = parseFloat(val);
      const currentNum = parseFloat(currentVal) || 0;
      const nextNum = Math.max(0, currentNum + inc);
      if (activeInput === 'amount') {
        setVal(formatOrderBaseAmountInput(nextNum));
      } else {
        setVal(formatOrderNumberInput(nextNum));
      }
    } else {
      if (currentVal === '0' && val === '0') return;
      if (currentVal === '0' && val !== '0') {
        setVal(sanitizeDecimalInput(val));
      } else {
        setVal(sanitizeDecimalInput(currentVal + val));
      }
    }
  };

  // Sync selected price from order book click
  useEffect(() => {
    if (selectedPrice !== null || selectedAmount !== null) {
      if (selectedPrice !== null) {
        setType('LIMIT');
        setPriceInput(formatOrderNumberInput(selectedPrice || 0));
      }
      const selectedPrimaryInput = selectedAmount !== null && side === 'SELL'
        ? Math.min(selectedAmount, availableBase)
        : selectedAmount;
      const formattedPrimaryInput = selectedPrimaryInput !== null
        ? formatOrderBaseAmountInput(selectedPrimaryInput)
        : '';
      if (selectedPrimaryInput !== null) {
        setAmountInput(formattedPrimaryInput);
        setActiveInput('amount');
      }
      clearSelectedPrice();
    }
  }, [selectedPrice, selectedAmount, side, availableBase, clearSelectedPrice]);

  // Set default price inputs
  useEffect(() => {
    if (type === 'LIMIT' && !priceInput) {
      setPriceInput(formatOrderNumberInput(pair.lastPrice));
    }
  }, [pair.lastPrice, type]);

  // Derived properties
  const price = type === 'MARKET' ? pair.lastPrice : parseFloat(priceInput) || 0;
  const amount = parseFloat(amountInput) || 0;
  const stopPrice = parseFloat(stopPriceInput) || 0;
  const total = price * amount;

  const currentBalance = side === 'BUY' ? availableUsdt : availableBase;
  const balanceLabel = side === 'BUY' ? pair.quoteAsset : pair.baseAsset;
  const primaryInputLabel = `Amount (${pair.baseAsset})`;
  const primaryInputSuffix = pair.baseAsset;

  // Percentage Calculations
  const handlePercentClick = (percent: number) => {
    if (side === 'BUY') {
      const targetSpendUsd = availableUsdt * (percent / 100);
      if (isFinite(targetSpendUsd) && targetSpendUsd > 0 && price > 0) {
        const calculatedAmount = targetSpendUsd / price;
        setAmountInput(formatOrderBaseAmountInput(calculatedAmount));
      } else {
        setAmountInput('');
      }
    } else {
      const calculatedAmount = availableBase * (percent / 100);
      setAmountInput(formatOrderBaseAmountInput(calculatedAmount));
    }
  };

  // Safe checks
  const normalizedSubmissionAmount = normalizeSubmissionAmount(amount, side, availableBase);
  const isBalanceExceeded = side === 'BUY'
    ? total > availableUsdt + BALANCE_EPSILON
    : normalizedSubmissionAmount > floorToDecimals(availableBase, BASE_AMOUNT_DECIMALS) + BALANCE_EPSILON;
  const isAmountZero = amount <= 0;
  const isPriceZero = type !== 'MARKET' && price <= 0;
  const isStopPriceNeeded = type === 'STOP_LIMIT' && stopPrice <= 0;
  
  const isOrderInvalid = isAmountZero || isPriceZero || isStopPriceNeeded || isBalanceExceeded;

  // Risks analysis indicators
  const isSlippageRisk = total > 15000; // Big order impact
  const isLiquidityWarning = amount > (pair.volume24h * 0.02); // Order occupies > 2% of 24h volume
  const isPriceDeviationWarning = type === 'LIMIT' && price > 0 && Math.abs((price - pair.lastPrice) / pair.lastPrice) > 0.1; // 10% extreme limit deviation

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (isOrderInvalid) return;

    // Trigger confirmation warnings for large or deviant trades
    if (isSlippageRisk || isLiquidityWarning || isPriceDeviationWarning) {
      setShowConfirm(true);
    } else {
      executeOrderPlacement();
    }
  };

  const executeOrderPlacement = () => {
    onSubmitOrder({
      side,
      type,
      price,
      amount: normalizedSubmissionAmount,
      stopPrice: type === 'STOP_LIMIT' ? stopPrice : undefined,
    });
    setAmountInput('');
    setShowConfirm(false);
  };

  return (
    <div className={`relative flex h-full min-h-[430px] flex-col overflow-y-auto bg-white p-3 text-gray-800 select-none dark:bg-[#0c1015] dark:text-gray-100 xl:min-h-0 ${
      docked
        ? 'border-0 shadow-none'
        : 'rounded-lg border border-[#e1e4e8] shadow-sm dark:border-[#21262d]'
    }`}>
      {/* Main Order Input Fields */}
      <form onSubmit={handleSubmit} className="flex-1 flex flex-col justify-between gap-2.5 text-xs">
        
        {/* PHYSICAL HARDWARE CONSOLE SHELL (Types, Balances, Inputs & keypad integrated together) */}
        <div className="relative flex flex-col space-y-2.5 rounded-xl border-2 border-[#e1e4e8] bg-[#fafbfc] p-2.5 shadow-xs transition-all focus-within:border-accent-1/60 dark:border-[#21262d] dark:bg-[#0a0c10]">
          <div className="rounded-lg border border-[#d8dee6] bg-white p-1.5 shadow-2xs dark:border-[#263241] dark:bg-[#10151d]">
            <div className="grid grid-cols-2 gap-1.5">
              <button
                type="button"
                onClick={() => {
                  setSide('BUY');
                }}
                className={`flex h-9 items-center justify-center gap-1.5 rounded-md border text-[11px] font-black uppercase tracking-wide transition-all active:scale-[0.98] ${
                  side === 'BUY'
                    ? 'border-trade-green bg-trade-green text-white shadow-[0_4px_12px_rgba(4,151,105,0.22)]'
                    : 'border-[#e1e4e8] bg-[#f6f8fa] text-gray-500 hover:border-trade-green/60 hover:text-trade-green dark:border-[#263241] dark:bg-[#121820] dark:text-gray-400 dark:hover:text-trade-green'
                }`}
              >
                <span>Buy</span>
                <span className={side === 'BUY' ? 'text-white/80' : 'text-gray-400 dark:text-gray-500'}>{pair.baseAsset}</span>
              </button>
              <button
                type="button"
                onClick={() => {
                  setSide('SELL');
                }}
                className={`flex h-9 items-center justify-center gap-1.5 rounded-md border text-[11px] font-black uppercase tracking-wide transition-all active:scale-[0.98] ${
                  side === 'SELL'
                    ? 'border-trade-red bg-trade-red text-white shadow-[0_4px_12px_rgba(220,41,121,0.22)]'
                    : 'border-[#e1e4e8] bg-[#f6f8fa] text-gray-500 hover:border-trade-red/60 hover:text-trade-red dark:border-[#263241] dark:bg-[#121820] dark:text-gray-400 dark:hover:text-trade-red'
                }`}
              >
                <span>Sell</span>
                <span className={side === 'SELL' ? 'text-white/80' : 'text-gray-400 dark:text-gray-500'}>{pair.baseAsset}</span>
              </button>
            </div>
          </div>

          {/* Core Controls: Order Execution Type (LIMIT, MARKET, STOP_LIMIT) */}
          <div className="bg-white dark:bg-[#12161f] rounded-lg border border-[#e1e4e8]/60 dark:border-[#21262d]/80 p-2 space-y-1.5 shadow-2xs">
            <div className="flex justify-between items-center text-[8.5px] font-mono font-bold text-gray-400 dark:text-gray-500 uppercase tracking-wider select-none">
              <span>Order Type</span>
              <span className="text-[#ff37c7] font-extrabold bg-[#ff37c7]/10 px-1 rounded-xs">{type === 'STOP_LIMIT' ? 'STOP' : type}</span>
            </div>
            <div className="grid grid-cols-3 gap-1">
              {(['LIMIT', 'MARKET', 'STOP_LIMIT'] as OrderType[]).map((t) => (
                <button
                  key={t}
                  type="button"
                  onClick={() => setType(t)}
                  className={`py-1.5 text-[9.5px] font-mono font-extrabold rounded-md text-center cursor-pointer transition-all border ${
                    type === t
                      ? 'bg-accent-1 text-white border-accent-1 shadow-xs'
                      : 'border-[#e1e4e8] dark:border-[#30363d] bg-gray-50/50 dark:bg-[#161b22]/50 text-gray-400 dark:text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'
                  }`}
                >
                  {t === 'STOP_LIMIT' ? 'STOP' : t}
                </button>
              ))}
            </div>
          </div>

          {/* Capital Allocation & Margin Balance display */}
          <div className="bg-white dark:bg-[#12161f] rounded-lg border border-[#e1e4e8]/60 dark:border-[#21262d]/80 p-2.5 flex justify-between items-center font-mono text-[10px]/none shadow-2xs">
            <div className="flex flex-col gap-1.5">
              <span className="text-[8.5px] text-gray-400 dark:text-gray-500 uppercase font-black tracking-wider select-none">Available Balance</span>
              <span className="text-gray-400 text-[9.5px]">{side === 'BUY' ? 'Quote asset' : 'Base asset'}</span>
            </div>
            <div className="text-right flex flex-col items-end gap-1.5">
              <span className="font-extrabold text-accent-1 text-xs select-all">
                {formatFixedInputDisplay(currentBalance, 8)} {balanceLabel}
              </span>
            </div>
          </div>

          {/* 1. INPUT: Stop-Limit activation price */}
          {type === 'STOP_LIMIT' && (
            <div className="space-y-1">
              <div className="flex justify-between items-center">
                <label className="block text-[10.5px] font-mono text-gray-400 uppercase select-none">Stop Price ({pair.quoteAsset})</label>
                {activeInput === 'stopPrice' && <span className="text-[8px] text-[#ff37c7] font-bold font-mono uppercase select-none">Active</span>}
              </div>
              <div className="relative text-gray-800 dark:text-gray-100">
                <input
                  type="text"
                  inputMode="decimal"
                  pattern="[0-9]*[.]?[0-9]*"
                  placeholder="0.00"
                  value={stopPriceInput}
                  onChange={(e) => setStopPriceInput(sanitizeDecimalInput(e.target.value))}
                  onFocus={() => setActiveInput('stopPrice')}
                  className={`w-full bg-white dark:bg-[#12161f] border rounded px-3 py-1.5 font-mono text-xs focus:ring-1 focus:ring-accent-1 focus:border-accent-1 transition-all focus:outline-none ${
                    activeInput === 'stopPrice'
                      ? 'border-[#ff37c7] ring-1 ring-[#ff37c7] bg-white dark:bg-[#161d2b] shadow-xs'
                      : 'border-[#e1e4e8] dark:border-[#30363d]'
                  }`}
                  required
                />
                <span className={`absolute right-3 top-1/2 -translate-y-1/2 font-mono text-[9px] uppercase transition-colors ${
                  activeInput === 'stopPrice' ? 'text-accent-1 font-bold animate-pulse' : 'text-gray-400'
                }`}>Stop</span>
              </div>
            </div>
          )}

          {/* 2. INPUT: Limit Price */}
          <div className="space-y-1">
            <div className="flex justify-between items-center">
              <label className="block text-[10.5px] font-mono text-gray-400 uppercase select-none">
                {type === 'MARKET' ? 'Price (Market)' : 'Limit Price (' + pair.quoteAsset + ')'}
              </label>
              {activeInput === 'price' && type !== 'MARKET' && <span className="text-[8px] text-[#ff37c7] font-bold font-mono uppercase select-none">Active</span>}
            </div>
            <div className="relative text-gray-800 dark:text-gray-100">
              <input
                type="text"
                inputMode="decimal"
                pattern="[0-9]*[.]?[0-9]*"
                disabled={type === 'MARKET'}
                placeholder={type === 'MARKET' ? 'MARKET ORDER ACTIVE' : '0.00'}
                value={type === 'MARKET' ? '' : priceInput}
                onChange={(e) => setPriceInput(sanitizeDecimalInput(e.target.value))}
                onFocus={() => setActiveInput('price')}
                className={`w-full bg-white dark:bg-[#12161f] border rounded px-3 py-1.5 font-mono text-xs focus:ring-1 focus:ring-accent-1 focus:border-accent-1 transition-all focus:outline-none ${
                  type === 'MARKET' ? 'opacity-55 bg-gray-50/50 dark:bg-[#161b22]/30 text-gray-400 cursor-not-allowed border-[#e1e4e8]/50 dark:border-[#30363d]/50' : ''
                } ${
                  activeInput === 'price' && type !== 'MARKET'
                    ? 'border-[#ff37c7] ring-1 ring-[#ff37c7] bg-white dark:bg-[#161d2b] shadow-xs'
                    : 'border-[#e1e4e8] dark:border-[#30363d]'
                }`}
                required={type !== 'MARKET'}
              />
              <span className={`absolute right-3 top-1/2 -translate-y-1/2 font-mono text-[9px] uppercase transition-colors ${
                activeInput === 'price' && type !== 'MARKET' ? 'text-accent-1 font-bold animate-pulse' : 'text-gray-400'
              }`}>
                {type === 'MARKET' ? 'Market' : pair.quoteAsset}
              </span>
            </div>
          </div>

          {/* 3. INPUT: Base asset amount */}
          <div className="space-y-1">
            <div className="flex justify-between items-center">
              <label className="block text-[10.5px] font-mono text-gray-400 uppercase select-none">{primaryInputLabel}</label>
              {activeInput === 'amount' && <span className="text-[8px] text-[#ff37c7] font-bold font-mono uppercase select-none">Active</span>}
            </div>
            <div className="relative text-gray-800 dark:text-gray-100">
              <input
                type="text"
                inputMode="decimal"
                pattern="[0-9]*[.]?[0-9]*"
                placeholder="0.00000000"
                value={amountInput}
                onChange={(e) => setAmountInput(sanitizeDecimalInput(e.target.value))}
                onFocus={() => setActiveInput('amount')}
                className={`w-full bg-white dark:bg-[#12161f] border rounded px-3 py-1.5 font-mono text-xs focus:ring-1 focus:ring-accent-1 focus:border-accent-1 transition-all focus:outline-none ${
                  activeInput === 'amount'
                    ? 'border-[#ff37c7] ring-1 ring-[#ff37c7] bg-white dark:bg-[#161d2b] shadow-xs'
                    : 'border-[#e1e4e8] dark:border-[#30363d]'
                }`}
                required
              />
              <span className={`absolute right-3 top-1/2 -translate-y-1/2 font-mono text-[9px] uppercase transition-colors ${
                activeInput === 'amount' ? 'text-accent-1 font-bold animate-pulse' : 'text-gray-400'
              }`}>{primaryInputSuffix}</span>
            </div>
          </div>

          {/* 4. Compact keypad */}
          <div className="border-t border-[#e1e4e8]/65 pt-3 dark:border-[#21262d]/65">
            <div className="flex items-center justify-between text-[10px] font-mono">
              <button
                type="button"
                onClick={() => setShowKeypad(!showKeypad)}
                className="flex items-center gap-1.5 font-bold uppercase text-[#ff37c7] hover:text-[#ff1cf4] transition-colors cursor-pointer select-none"
              >
                <Sliders className="w-3.5 h-3.5" />
                <span>Keypad</span>
                <span className="text-[8.5px] bg-[#ff37c7]/10 text-[#ff37c7] px-1 rounded-sm font-bold">
                  {activeInput === 'amount' ? 'Amount' : activeInput === 'price' ? 'Limit' : 'Stop'}
                </span>
              </button>
              <button
                type="button"
                onClick={() => setShowKeypad(!showKeypad)}
                className="text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 transition-colors cursor-pointer font-bold select-none text-[9px] bg-gray-100 dark:bg-[#18202a] px-1.5 py-0.5 rounded"
              >
                {showKeypad ? 'Hide keys' : 'Show keys'}
              </button>
            </div>

            <div className="mt-2 rounded-lg border border-[#d8dee6] bg-[#f6f8fa] p-2.5 shadow-inner select-none font-mono text-gray-800 dark:border-[#263241] dark:bg-[#080d14] dark:text-gray-100">
              <div className="grid grid-cols-4 gap-1.5">
                {[25, 50, 75, 100].map((pct) => (
                  <button
                    key={pct}
                    type="button"
                    onClick={() => handlePercentClick(pct)}
                    className="h-8 text-[9.5px] font-mono font-black border border-[#d8dee6] dark:border-[#263241] bg-white dark:bg-[#121820] hover:bg-[#ff37c7]/10 hover:border-[#ff37c7] rounded-md hover:text-[#ff37c7] cursor-pointer transition-colors text-center shadow-2xs active:scale-95"
                  >
                    {pct}%
                  </button>
                ))}
              </div>

              {showKeypad && (
                <div className="mt-2.5 border-t border-[#e1e4e8]/70 pt-2.5 dark:border-[#263241]/80 animate-fade-in">
                  <div className="grid grid-cols-4 gap-1.5">
                  {/* Digits and decimal */}
                  <div className="col-span-3 grid grid-cols-3 gap-1">
                    {['1', '2', '3', '4', '5', '6', '7', '8', '9', '.', '0'].map((digit) => (
                      <button
                        key={digit}
                        type="button"
                        onClick={() => handleKeypadPress(digit)}
                        className="h-8 text-xs font-semibold rounded-md bg-white dark:bg-[#121820] border border-[#d8dee6] dark:border-[#263241] hover:border-[#ff37c7] hover:text-[#ff37c7] dark:hover:border-[#ff37c7] dark:hover:text-[#ff37c7] cursor-pointer transition-all active:scale-95 flex items-center justify-center shadow-xs"
                      >
                        {digit}
                      </button>
                    ))}
                    <button
                      type="button"
                      onClick={() => handleKeypadPress('C')}
                      className="h-8 text-[10px] font-bold rounded-md bg-rose-50 dark:bg-rose-950/30 border border-rose-200 dark:border-rose-900 text-rose-500 hover:bg-rose-100 dark:hover:bg-rose-900/40 hover:text-rose-600 cursor-pointer transition-all active:scale-95 flex items-center justify-center shadow-xs"
                      title="Clear Field"
                    >
                      CLEAR
                    </button>
                  </div>

                  {/* Backspace and Increments column */}
                  <div className="col-span-1 flex flex-col gap-1">
                    <button
                      type="button"
                      onClick={() => handleKeypadPress('⌫')}
                      className="h-8 text-[11px] font-bold rounded-md bg-amber-50 dark:bg-amber-100/10 border border-amber-200 dark:border-amber-800 text-amber-600 dark:text-amber-400 hover:bg-amber-100 dark:hover:bg-amber-950/30 cursor-pointer transition-all active:scale-95 flex items-center justify-center shadow-xs"
                      title="Backspace"
                    >
                      ⌫
                    </button>
                    {activeInput === 'amount' ? (
                      <>
                        {['+0.01', '+0.1', '+1'].map((inc) => (
                          <button
                            key={inc}
                            type="button"
                            onClick={() => handleKeypadPress(inc)}
                            className="h-[21px] text-[8.5px] font-bold rounded-md bg-white dark:bg-[#121820] border border-[#d8dee6] dark:border-[#263241] text-gray-500 dark:text-gray-400 hover:border-[#ff37c7] hover:text-[#ff37c7] cursor-pointer transition-all active:scale-95"
                          >
                            {inc}
                          </button>
                        ))}
                      </>
                    ) : (
                      <>
                        {['+1', '+10', '+100'].map((inc) => (
                          <button
                            key={inc}
                            type="button"
                            onClick={() => handleKeypadPress(inc)}
                            className="h-[21px] text-[8.5px] font-bold rounded-md bg-white dark:bg-[#121820] border border-[#d8dee6] dark:border-[#263241] text-gray-500 dark:text-gray-400 hover:border-[#ff37c7] hover:text-[#ff37c7] cursor-pointer transition-all active:scale-95"
                          >
                            {inc}
                          </button>
                        ))}
                      </>
                    )}
                    </div>
                  </div>
                </div>
              )}
            </div>
          </div>

        </div>

        {/* OUTPUT: Order pricing metrics estimation */}
        <div className="space-y-1 border-t border-[#e1e4e8]/60 py-0.5 text-[10px] font-mono text-gray-500 dark:border-[#21262d]/60">
          <div className="flex justify-between">
            <span>Sub-Total:</span>
            <span className="font-semibold text-gray-700 dark:text-gray-300">{formatFixedInputDisplay(total, 8)} {pair.quoteAsset}</span>
          </div>
          <div className="flex justify-between border-t border-dashed border-[#e1e4e8] dark:border-[#21262d] pt-1 mt-1 text-xs">
            <span className="text-gray-800 dark:text-gray-200">Total Outflow:</span>
            <span className="font-bold text-accent-1">{formatFixedInputDisplay(total, 8)} {pair.quoteAsset}</span>
          </div>
        </div>

        {/* Embedded Risk Warning Board */}
        {(isSlippageRisk || isLiquidityWarning || isPriceDeviationWarning || isBalanceExceeded) && (
          <div className="p-2.5 rounded bg-amber-50 dark:bg-amber-950/20 border border-amber-200/50 dark:border-amber-900/30 font-mono text-[9px] text-amber-700 dark:text-amber-400 leading-normal flex gap-2">
            <ShieldAlert className="w-4 h-4 shrink-0 text-amber-500" />
            <div className="space-y-0.5">
              <span className="font-bold block uppercase">Risk Evaluation Notice:</span>
              {isBalanceExceeded && <span>• Insufficient wallet account funds to cover this margin order.</span>}
              {isSlippageRisk && <span>• Order block exceeds slippage hazard limits (${total.toLocaleString()}).</span>}
              {isLiquidityWarning && <span>• Size occupies high percentage of 24h liquidity pools. High price impact.</span>}
              {isPriceDeviationWarning && <span>• Entry deviates significantly (&gt;10%) from active market index price.</span>}
            </div>
          </div>
        )}

        {submitError && (
          <div className="p-2.5 rounded bg-rose-50 dark:bg-rose-950/20 border border-rose-200/60 dark:border-rose-900/35 font-mono text-[9px] text-rose-600 dark:text-rose-400 leading-normal flex gap-2">
            <AlertCircle className="w-4 h-4 shrink-0" />
            <div>
              <span className="font-bold block uppercase">Backend Order Rejected:</span>
              <span>{submitError}</span>
            </div>
          </div>
        )}

        {/* ORDER TRIGGER BUTTON */}
        <button
          type="submit"
          disabled={isOrderInvalid}
          className={`relative flex w-full items-center justify-center gap-1.5 overflow-hidden rounded py-2 text-xs font-bold uppercase tracking-wider transition-all duration-300 ${
            isOrderInvalid
              ? 'bg-gray-100 dark:bg-[#161b22] text-gray-400 border border-[#e1e4e8] dark:border-[#21262d] cursor-not-allowed'
              : side === 'BUY'
              ? 'bg-trade-green text-white cursor-pointer hover:bg-trade-green/95 shadow-md hover:shadow-trade-green/20'
              : 'bg-trade-red text-white cursor-pointer hover:bg-trade-red/95 shadow-md hover:shadow-trade-red/20'
          }`}
        >
          {side === 'BUY' ? `EXECUTE BUY ${type}` : `EXECUTE SELL ${type}`}
        </button>

      </form>

      {/* CONFIRMATION SAFETY MODAL DIALOG */}
      {showConfirm && (
        <div className="absolute inset-0 bg-white/95 dark:bg-[#0c1015]/95 backdrop-blur-xs flex flex-col justify-between p-4 z-50 rounded-lg animate-fade-in border border-[#e1e4e8] dark:border-[#21262d]">
          <div className="space-y-2">
            <div className="flex items-center gap-2 text-amber-500 font-display font-semibold text-xs border-b border-gray-200 dark:border-gray-800 pb-1.5">
              <AlertTriangle className="w-4 h-4 text-amber-500 animate-bounce" />
              Slippage & Impact Guard
            </div>
            
            <p className="text-[10px] text-gray-500 leading-normal">
              You are about to launch a high-impact spot trade that triggers our risk management protocols.
            </p>

            <div className="bg-slate-50 dark:bg-slate-900/50 p-2 rounded text-[10px] font-mono space-y-1 block border border-[#e1e4e8]/50 dark:border-[#21262d]/50">
              <div className="flex justify-between">
                <span>Asset Pair:</span>
                <span className="font-semibold">{pair.symbol}</span>
              </div>
              <div className="flex justify-between">
                <span>Order Side / Type:</span>
                <span className={`font-semibold ${side === 'BUY' ? 'text-trade-green' : 'text-trade-red'}`}>{side} {type}</span>
              </div>
              <div className="flex justify-between">
                <span>Quantity Requested:</span>
                <span className="font-semibold">{formatFixedInputDisplay(amount, 8)} {pair.baseAsset}</span>
              </div>
              <div className="flex justify-between">
                <span>Notional Amount:</span>
                <span className="font-bold text-accent-1">{formatFixedInputDisplay(total, 8)} {pair.quoteAsset}</span>
              </div>
            </div>

            <p className="text-[9px] text-rose-500 font-medium">
              *Confirming submits the order with the displayed price, amount, and notional.
            </p>
          </div>

          <div className="flex gap-2">
            <button
              onClick={() => setShowConfirm(false)}
              className="flex-1 py-1.5 border border-[#e1e4e8] dark:border-[#21262d] bg-gray-50/50 hover:bg-surface-3 text-[10px] font-mono rounded cursor-pointer text-gray-600 dark:text-gray-400"
            >
              Abandone Trade
            </button>
            <button
              onClick={executeOrderPlacement}
              className={`flex-1 py-1.5 text-white text-[10px] font-mono font-bold rounded cursor-pointer ${
                side === 'BUY' ? 'bg-trade-green hover:bg-trade-green/90' : 'bg-trade-red hover:bg-trade-red/90'
              }`}
            >
              Verify & Force Order
            </button>
          </div>
        </div>
      )}

    </div>
  );
}

function formatOrderNumberInput(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '';
  return trimTrailingDecimalZeros(truncateDecimalString(expandDecimalNumber(value), ORDER_INPUT_DECIMALS));
}

function formatOrderBaseAmountInput(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '';
  return trimTrailingDecimalZeros(truncateDecimalString(expandDecimalNumber(value), BASE_AMOUNT_DECIMALS));
}

function normalizeSubmissionAmount(value: number, side: OrderSide, availableBase: number): number {
  if (!Number.isFinite(value) || value <= 0) return 0;
  const normalized = floorToDecimals(value, BASE_AMOUNT_DECIMALS);
  if (side === 'SELL' && normalized > availableBase) {
    const available = floorToDecimals(availableBase, BASE_AMOUNT_DECIMALS);
    if (normalized <= available || normalized - availableBase <= DISPLAY_ROUNDING_TOLERANCE) {
      return available;
    }
  }
  return normalized;
}

function floorToDecimals(value: number, decimals: number): number {
  const truncated = truncateDecimalString(expandDecimalNumber(value), decimals);
  const numeric = Number(truncated);
  return Number.isFinite(numeric) ? numeric : 0;
}

function formatFixedInputDisplay(value: number, decimals: number): string {
  return padDecimalString(truncateDecimalString(expandDecimalNumber(value), decimals), decimals);
}

function sanitizeDecimalInput(value: string, maxDecimals = ORDER_INPUT_DECIMALS): string {
  let out = '';
  let hasDecimal = false;
  let decimalCount = 0;

  for (const char of value.replace(',', '.')) {
    if (char >= '0' && char <= '9') {
      if (hasDecimal) {
        if (decimalCount >= maxDecimals) continue;
        decimalCount++;
      }
      out += char;
      continue;
    }
    if (char === '.' && !hasDecimal) {
      out += char;
      hasDecimal = true;
    }
  }

  return out;
}

function expandDecimalNumber(value: number): string {
  if (!Number.isFinite(value)) return '0';
  const raw = value.toString();
  if (!/[eE]/.test(raw)) return raw;

  const [mantissa, exponentPart] = raw.toLowerCase().split('e');
  const exponent = Number(exponentPart);
  if (!Number.isFinite(exponent)) return '0';

  const sign = mantissa.startsWith('-') ? '-' : '';
  const unsignedMantissa = mantissa.replace('-', '');
  const [integerPart, fractionalPart = ''] = unsignedMantissa.split('.');
  const digits = `${integerPart}${fractionalPart}`;
  const decimalIndex = integerPart.length + exponent;

  if (decimalIndex <= 0) {
    return `${sign}0.${'0'.repeat(Math.abs(decimalIndex))}${digits}`;
  }
  if (decimalIndex >= digits.length) {
    return `${sign}${digits}${'0'.repeat(decimalIndex - digits.length)}`;
  }
  return `${sign}${digits.slice(0, decimalIndex)}.${digits.slice(decimalIndex)}`;
}

function truncateDecimalString(value: string, decimals: number): string {
  const normalized = value.replace(',', '.').trim();
  if (!normalized) return '0';
  const sign = normalized.startsWith('-') ? '-' : '';
  const unsigned = normalized.replace(/^[+-]/, '');
  const [integerPartRaw, fractionalPartRaw = ''] = unsigned.split('.');
  const integerPart = integerPartRaw.replace(/^0+(?=\d)/, '') || '0';
  if (decimals <= 0) return `${sign}${integerPart}`;
  const fractionalPart = fractionalPartRaw.slice(0, decimals);
  return fractionalPart.length > 0 ? `${sign}${integerPart}.${fractionalPart}` : `${sign}${integerPart}`;
}

function trimTrailingDecimalZeros(value: string): string {
  if (!value.includes('.')) return value;
  return value.replace(/0+$/, '').replace(/\.$/, '');
}

function padDecimalString(value: string, decimals: number): string {
  const [integerPart, fractionalPart = ''] = truncateDecimalString(value, decimals).split('.');
  if (decimals <= 0) return integerPart;
  return `${integerPart}.${fractionalPart.padEnd(decimals, '0').slice(0, decimals)}`;
}
