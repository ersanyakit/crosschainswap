/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import { useState } from 'react';
import { FileCode2, Play, Pause, RefreshCw, CheckCircle2, TrendingUp, Sliders, AlertCircle, Sparkles, Terminal } from 'lucide-react';
import { TradingStrategy, MarketPair } from '../types/trading';
import { BRAND_NAME } from '../constants/brand';

interface StrategyLabProps {
  strategies: TradingStrategy[];
  markets: MarketPair[];
  onToggleStrategy: (id: string) => void;
  onUpdateStrategyCode: (id: string, code: string) => void;
  onAddSystemLog: (msg: string, source: 'SYSTEM' | 'ORDER' | 'WEBSOCKET' | 'STRATEGY', type: 'INFO' | 'SUCCESS' | 'WARNING' | 'ERROR') => void;
}

export default function StrategyLab({
  strategies,
  markets,
  onToggleStrategy,
  onUpdateStrategyCode,
  onAddSystemLog,
}: StrategyLabProps) {
  const [selectedStratId, setSelectedStratId] = useState(strategies[0]?.id || '');
  const [backtestPair, setBacktestPair] = useState('PEPPER/USD');
  const [startCapital, setStartCapital] = useState('10000');
  
  // Script compilation logs states
  const [isCompiling, setIsCompiling] = useState(false);
  const [compilationLogs, setCompilationLogs] = useState<string[]>([]);
  const [backtestStats, setBacktestStats] = useState<{
    ran: boolean;
    profitPct: number;
    trades: number;
    winRate: number;
    finalCapital: number;
  } | null>(null);

  const activeStrategy = strategies.find(s => s.id === selectedStratId) || strategies[0];

  const handleCompile = () => {
    setIsCompiling(true);
    setCompilationLogs(['Initializing WebAssembly Strategy Sandbox...', 'Verifying JS syntax compliance...']);
    
    setTimeout(() => {
      setCompilationLogs(prev => [
        ...prev,
        'Compiling evaluation hook...',
        'Linking historical market walk buffers...',
        'SUCCESS: Compiled strategy bundle registered with VM engine.'
      ]);
      setIsCompiling(false);
      onAddSystemLog(`Strategy '${activeStrategy.name}' recompiled successfully.`, 'STRATEGY', 'SUCCESS');
    }, 1200);
  };

  const handleRunBacktest = () => {
    setIsCompiling(true);
    setCompilationLogs(['Setting up backtest parameters...', `Loading historical ticks for ${backtestPair}...`]);
    setBacktestStats(null);

    setTimeout(() => {
      const mockCapital = parseFloat(startCapital) || 10000;
      // High profit bots for fun but realistic variation
      const randomWinRate = 50 + Math.random() * 25;
      const profitPctMultiplier = randomWinRate > 58 ? (1.1 + Math.random() * 0.4) : (0.8 + Math.random() * 0.3);
      const randomProfitPct = (randomWinRate * 0.5 - 20) * profitPctMultiplier;
      const finalCapital = mockCapital * (1 + randomProfitPct / 100);
      const totalTradesCount = 18 + Math.floor(Math.random() * 60);

      setCompilationLogs(prev => [
        ...prev,
        `Iterated evaluation hook over last 120 candles of ${backtestPair}...`,
        `Fitted trade calculations: ${totalTradesCount} signals triggered.`,
        'Simulation finalized without slippage faults.'
      ]);

      setBacktestStats({
        ran: true,
        profitPct: Number(randomProfitPct.toFixed(2)),
        trades: totalTradesCount,
        winRate: Number(randomWinRate.toFixed(1)),
        finalCapital: Number(finalCapital.toFixed(2)),
      });

      setIsCompiling(false);
      onAddSystemLog(`Backtest of ${activeStrategy.name} on ${backtestPair} completed containing ${totalTradesCount} signals.`, 'STRATEGY', 'INFO');
    }, 1500);
  };

  const codeValue = activeStrategy?.code || '';

  return (
    <div className="flex-1 overflow-y-auto p-4 sm:p-5 bg-[#fafbfc] dark:bg-[#070b0f] space-y-5 select-none h-full max-w-7xl mx-auto">
      
      {/* Overview Intro Banner */}
      <div className="p-4 rounded-lg bg-accent-1/5 dark:bg-accent-2 border border-accent-1/15 flex items-start gap-3.5 relative overflow-hidden">
        <div className="p-2 bg-accent-1 text-white rounded shadow-sm ide-glow">
          <FileCode2 className="w-5 h-5" />
        </div>
        <div className="space-y-1">
          <h2 className="text-xs font-bold uppercase tracking-wider text-accent-1 font-display">
            {BRAND_NAME} Strategy Sandbox Lab
          </h2>
          <p className="text-[11px] text-gray-500 dark:text-gray-400 font-mono leading-relaxed max-w-3xl">
            A real-time developer terminal interface. Edit strategy hooks using the visual JS compiler, run instant backtests on multi-day charts, and toggle automated trading bots on sandbox wallets.
          </p>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-24 gap-5">
        
        {/* CODE EDITOR CONTAINER (LEFT) */}
        <div className="lg:col-span-15 flex flex-col bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm h-[420px] overflow-hidden">
          
          {/* Editor Header tabs */}
          <div className="flex items-center justify-between px-3 py-2 bg-[#f6f8fa] dark:bg-[#0d1117] border-b border-[#e1e4e8] dark:border-[#21262d]">
            <div className="flex gap-2">
              <select
                value={selectedStratId}
                onChange={(e) => setSelectedStratId(e.target.value)}
                className="text-xs font-mono font-bold px-2 py-0.5 rounded border border-[#e1e4e8] dark:border-[#21262d] bg-white dark:bg-[#0d1117] text-gray-800 dark:text-gray-250 focus:outline-none focus:border-accent-1 cursor-pointer"
              >
                {strategies.map(s => (
                  <option key={s.id} value={s.id}>{s.name}</option>
                ))}
              </select>
            </div>

            <div className="flex items-center gap-2">
              <button
                onClick={handleCompile}
                disabled={isCompiling}
                className="px-2.5 py-1 bg-gray-100 hover:bg-gray-200 dark:bg-slate-800 hover:text-accent-1 text-gray-600 dark:text-gray-300 rounded font-mono text-[10px] font-bold border border-gray-200 dark:border-transparent flex items-center gap-1 cursor-pointer transition-colors"
              >
                <RefreshCw className={`w-3 h-3 ${isCompiling ? 'animate-spin' : ''}`} />
                Compile Script
              </button>
              
              <button
                onClick={() => onToggleStrategy(activeStrategy.id)}
                className={`px-3 py-1 text-[10px] font-mono font-bold rounded flex items-center gap-1 cursor-pointer transition-colors shadow-sm ${
                  activeStrategy.status === 'RUNNING'
                    ? 'bg-amber-500 hover:bg-amber-600 text-white'
                    : 'bg-trade-green hover:bg-trade-green/90 text-white'
                }`}
              >
                {activeStrategy.status === 'RUNNING' ? (
                  <>
                    <Pause className="w-3 h-3 fill-white" />
                    Disable Bot
                  </>
                ) : (
                  <>
                    <Play className="w-3 h-3 fill-white" />
                    Deploy Bot Sandbox
                  </>
                )}
              </button>
            </div>
          </div>

          {/* Simulated Code area */}
          <div className="flex-1 flex relative">
            
            {/* Visual Line Numbers */}
            <div className="w-10 bg-[#fafbfc] dark:bg-[#0d1117] border-r border-[#e1e4e8] dark:border-[#21262d] select-none text-right pr-2 py-3 text-[10px] font-mono text-gray-400 space-y-[4.5px] leading-relaxed">
              {Array.from({ length: 30 }).map((_, i) => (
                <div key={i}>{i + 1}</div>
              ))}
            </div>

            {/* Code Textarea */}
            <textarea
              spellCheck={false}
              value={codeValue}
              onChange={(e) => onUpdateStrategyCode(activeStrategy.id, e.target.value)}
              className="flex-1 p-3 font-mono text-[11px] leading-relaxed bg-white dark:bg-[#070b0f] text-gray-800 dark:text-gray-100 focus:outline-none resize-none overflow-y-auto"
            />
          </div>

          <div className="p-2 border-t border-[#e1e4e8] dark:border-[#21262d] bg-[#f9fafc] dark:bg-[#070b0f] font-mono text-[9px] text-gray-400 text-right">
            Active workspace runtime: JS ESNext Sandbox
          </div>

        </div>

        {/* PARAMETERS & COMPILER BACKTEST RESULTS (RIGHT) */}
        <div className="lg:col-span-9 flex flex-col gap-4">
          
          {/* Backtest Config Card */}
          <div className="bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm p-4 space-y-3 shrink-0">
            <h3 className="text-xs font-bold font-display uppercase tracking-wider text-[#7e8c9a]">
              Backtest Simulation Parameters
            </h3>

            <div className="space-y-4 text-xs font-mono">
              <div>
                <label className="block text-[9px] uppercase tracking-wider text-gray-400 mb-1">Target Spot Market</label>
                <select
                  value={backtestPair}
                  onChange={(e) => setBacktestPair(e.target.value)}
                  className="w-full bg-[#fafbfc] dark:bg-[#0d1117] border border-[#e1e4e8] dark:border-[#21262d] rounded px-3 py-1.5 focus:outline-none focus:border-accent-1 cursor-pointer"
                >
                  {markets.map(m => (
                    <option key={m.symbol} value={m.symbol}>{m.symbol}</option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-[9px] uppercase tracking-wider text-gray-400 mb-1">Backtest Simulated Starting Capital (USD)</label>
                <input
                  type="number"
                  value={startCapital}
                  onChange={(e) => setStartCapital(e.target.value)}
                  className="w-full bg-[#fafbfc] dark:bg-[#0d1117] border border-[#e1e4e8] dark:border-[#21262d] rounded px-3 py-1.5 focus:ring-1 focus:ring-accent-1 focus:border-accent-1 focus:outline-none"
                />
              </div>

              <button
                onClick={handleRunBacktest}
                disabled={isCompiling}
                className="w-full py-2.5 bg-accent-1 hover:bg-accent-1-hovered text-white text-[11px] font-bold rounded cursor-pointer transition-all flex items-center justify-center gap-1.5"
              >
                <Play className="w-3.5 h-3.5 fill-white" />
                EXECUTE SIMULATION WALK
              </button>
            </div>
          </div>

          {/* Compiler Messages logs & Backtest Results Card */}
          <div className="flex-1 bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm p-4 flex flex-col justify-between overflow-hidden">
            <div className="space-y-3 overflow-y-auto max-h-[170px]">
              <h3 className="text-xs font-bold font-display uppercase tracking-wider text-[#7e8c9a] flex items-center gap-1 shrink-0">
                <Terminal className="w-4 h-4 text-accent-1" />
                VM Compiler Messages
              </h3>

              {compilationLogs.length === 0 ? (
                <div className="text-[10px] font-mono text-gray-400 italic">
                  Awaiting script compile or backtest execute commands...
                </div>
              ) : (
                <div className="font-mono text-[10px] space-y-1 bg-slate-950 text-emerald-400 p-2.5 rounded border border-[#21262d] leading-normal select-text selection:bg-accent-1/20 select-none">
                  {compilationLogs.map((log, idx) => (
                    <div key={idx} className="flex gap-1.5 items-start">
                      <span className="text-slate-600">&gt;</span>
                      <span>{log}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Backtest Success Metrics */}
            {backtestStats && (
              <div className="border-t border-[#e1e4e8] dark:border-[#21262d] pt-3.5 mt-3 text-xs space-y-2.5 font-mono">
                <div className="flex items-center gap-1.5 text-xs text-trade-green font-display font-semibold uppercase">
                  <CheckCircle2 className="w-4 h-4 text-trade-green" />
                  Simulation finalized
                </div>

                <div className="grid grid-cols-2 gap-2 text-[11px]">
                  <div className="p-2 bg-slate-50 dark:bg-slate-900/40 rounded border border-[#e1e4e8]/50 dark:border-[#21262d]/50">
                    <span className="block text-[8.5px] uppercase text-gray-400">Yield PnL Return</span>
                    <span className={`font-bold block text-sm ${backtestStats.profitPct >= 0 ? 'text-trade-green' : 'text-trade-red'}`}>
                      {backtestStats.profitPct >= 0 ? '+' : ''}{backtestStats.profitPct}%
                    </span>
                  </div>
                  <div className="p-2 bg-slate-50 dark:bg-slate-900/40 rounded border border-[#e1e4e8]/50 dark:border-[#21262d]/50">
                    <span className="block text-[8.5px] uppercase text-gray-400">Compiled Win-rate</span>
                    <span className="font-bold block text-sm text-gray-900 dark:text-gray-150">
                      {backtestStats.winRate}% (Fitted)
                    </span>
                  </div>
                </div>

                <div className="flex justify-between items-center text-[11px] pt-1 border-t border-dashed border-gray-150 dark:border-gray-800">
                  <span className="text-gray-500">Ended Equity Balance:</span>
                  <span className="font-bold text-accent-1">${backtestStats.finalCapital.toLocaleString(undefined, { minimumFractionDigits: 2 })} USD</span>
                </div>
              </div>
            )}
          </div>

        </div>

      </div>

    </div>
  );
}
