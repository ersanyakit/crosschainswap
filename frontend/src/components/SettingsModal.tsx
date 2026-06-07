/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import { useState } from 'react';
import { Settings, HelpCircle, HardDrive, Volume2, ShieldCheck, RefreshCw, Layers, Sparkles, UserCheck } from 'lucide-react';
import { BRAND_NAME } from '../constants/brand';

interface SettingsModalProps {
  theme: 'light' | 'dark';
  setTheme: (theme: 'light' | 'dark') => void;
  density: 'compact' | 'comfortable';
  setDensity: (density: 'compact' | 'comfortable') => void;
  confirmOrders: boolean;
  setConfirmOrders: (confirm: boolean) => void;
  soundEnabled: boolean;
  setSoundEnabled: (enabled: boolean) => void;
  onPurgeDbs: () => void;
  userEmail: string;
}

export default function SettingsView({
  theme,
  setTheme,
  density,
  setDensity,
  confirmOrders,
  setConfirmOrders,
  soundEnabled,
  setSoundEnabled,
  onPurgeDbs,
  userEmail,
}: SettingsModalProps) {
  const [purgedMsg, setPurgedMsg] = useState(false);

  const triggerPurge = () => {
    onPurgeDbs();
    setPurgedMsg(true);
    setTimeout(() => setPurgedMsg(false), 2000);
  };

  return (
    <div className="flex-1 overflow-y-auto p-4 sm:p-5 bg-[#fafbfc] dark:bg-[#070b0f] space-y-6 select-none h-full max-w-4xl mx-auto">
      
      {/* Settings Header */}
      <div className="flex items-center gap-3 border-b border-[#e1e4e8] dark:border-[#21262d] pb-4">
        <div className="p-2.5 bg-accent-1 text-white rounded shadow-sm ide-glow">
          <Settings className="w-5 h-5 animate-spin" />
        </div>
        <div>
          <h2 className="text-sm font-bold uppercase tracking-wider text-gray-900 dark:text-gray-100 font-display">
            {BRAND_NAME} Terminal Settings & Preferences
          </h2>
          <span className="text-[10px] font-mono text-gray-400">
            Customize workstation graphics rendering, sound triggers, and spot limit configurations.
          </span>
        </div>
      </div>

      {/* Primary configuration card panels */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-5 font-mono text-xs">
        
        {/* PANEL 1: Terminal Visual Styles */}
        <div className="bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm p-5 space-y-4">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-accent-1 flex items-center gap-1.5 font-display border-b border-gray-100 dark:border-gray-800 pb-2">
            <Layers className="w-4 h-4 text-accent-1" />
            Workspace Canvas Graphics
          </h3>

          {/* Theme switcher */}
          <div className="space-y-1.5">
            <span className="block text-[10px] text-gray-400 uppercase">Interactive Color Palette</span>
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={() => setTheme('light')}
                className={`py-1.5 border font-semibold rounded cursor-pointer transition-all ${
                  theme === 'light'
                    ? 'bg-accent-1 border-accent-1 text-white shadow-sm font-bold'
                    : 'bg-[#f4f4f4] dark:bg-[#161b22] border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'
                }`}
              >
                {BRAND_NAME} Light (Default)
              </button>
              <button
                type="button"
                onClick={() => setTheme('dark')}
                className={`py-1.5 border font-semibold rounded cursor-pointer transition-all ${
                  theme === 'dark'
                    ? 'bg-accent-1 border-accent-1 text-white shadow-sm font-bold'
                    : 'bg-[#f4f4f4] dark:bg-[#161b22] border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'
                }`}
              >
                Carbon Dark
              </button>
            </div>
          </div>

          {/* Layout density switcher */}
          <div className="space-y-1.5">
            <span className="block text-[10px] text-gray-400 uppercase">Information Density scale</span>
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={() => setDensity('compact')}
                className={`py-1.5 border font-semibold rounded cursor-pointer transition-all ${
                  density === 'compact'
                    ? 'bg-accent-1 border-accent-1 text-white shadow-sm font-bold'
                    : 'bg-[#f4f4f4] dark:bg-[#161b22] border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'
                }`}
              >
                Compact (Pro Density)
              </button>
              <button
                type="button"
                onClick={() => setDensity('comfortable')}
                className={`py-1.5 border font-semibold rounded cursor-pointer transition-all ${
                  density === 'comfortable'
                    ? 'bg-accent-1 border-accent-1 text-white shadow-sm font-bold'
                    : 'bg-[#f4f4f4] dark:bg-[#161b22] border-transparent text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'
                }`}
              >
                Spacious (Legibility)
              </button>
            </div>
          </div>
        </div>

        {/* PANEL 2: Terminal Securities and Guards & system values */}
        <div className="bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm p-5 space-y-4">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-accent-1 flex items-center gap-1.5 font-display border-b border-gray-100 dark:border-gray-800 pb-2">
            <ShieldCheck className="w-4 h-4 text-accent-1" />
            Executing Guards & Alarms
          </h3>

          {/* Confirm trades toggle */}
          <div className="flex items-center justify-between py-1">
            <div className="space-y-0.5 max-w-[80%]">
              <span className="block text-gray-900 dark:text-gray-100 font-bold">Trade Confirmation Dialog</span>
              <span className="text-[10px] text-gray-400 block leading-normal">
                Prompt risk confirmation popup checklists before launching large spot orders.
              </span>
            </div>
            <label className="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                checked={confirmOrders}
                onChange={() => setConfirmOrders(!confirmOrders)}
                className="sr-only peer"
              />
              <div className="w-9 h-5 bg-gray-200 peer-focus:outline-none dark:bg-slate-800 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-accent-1"></div>
            </label>
          </div>

          {/* Sound elements */}
          <div className="flex items-center justify-between py-1">
            <div className="space-y-0.5 max-w-[80%]">
              <span className="block text-gray-900 dark:text-gray-100 font-bold">Fitted Sound Alerts</span>
              <span className="text-[10px] text-gray-400 block leading-normal">
                Triggers acoustic synthesizer signals on active standing limit executions.
              </span>
            </div>
            <label className="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                checked={soundEnabled}
                onChange={() => setSoundEnabled(!soundEnabled)}
                className="sr-only peer"
              />
              <div className="w-9 h-5 bg-gray-200 peer-focus:outline-none dark:bg-slate-800 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-accent-1"></div>
            </label>
          </div>
        </div>

      </div>

      {/* PANEL 3: Accounts details, clear databases & developer licensing */}
      <div className="bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm p-5 space-y-4">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-accent-1 flex items-center gap-1.5 font-display border-b border-gray-100 dark:border-gray-800 pb-2">
          <HardDrive className="w-4 h-4 text-accent-1" />
          Terminal Ledger Core Operations
        </h3>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-xs font-mono">
          
          {/* Identity details and parameters */}
          <div className="p-3.5 rounded bg-slate-50 dark:bg-slate-900/45 space-y-2 border border-gray-200 dark:border-gray-800/60 leading-normal">
            <div className="flex items-center gap-1 text-[11px] font-bold text-gray-800 dark:text-gray-100 font-display">
              <UserCheck className="w-4 h-4 text-accent-1" />
              Connected Developer Profile
            </div>
            
            <div className="space-y-0.5 text-[10px] text-gray-500">
              <div>Session Owner: <span className="font-bold text-gray-700 dark:text-gray-200">{userEmail || 'Guest Coder'}</span></div>
              <div>License Code: <span className="font-sans">{BRAND_NAME}-SPOT-SANDBOX-99XLR</span></div>
              <div>System Node Time: <span className="text-accent-1 font-mono">{new Date().toLocaleDateString()}</span></div>
            </div>
          </div>

          {/* Database resynchronization */}
          <div className="p-3.5 rounded bg-slate-50 dark:bg-slate-900/45 flex flex-col justify-between items-start gap-3 border border-gray-200 dark:border-gray-800/60 leading-normal">
            <div>
              <span className="font-bold block text-gray-800 dark:text-gray-100 text-[11px] font-display">Clear Handled Sessions Cache</span>
              <span className="text-[10px] text-gray-400 block mt-0.5">
                Clear local UI cache and request fresh backend data on the next refresh.
              </span>
            </div>

            <button
              onClick={triggerPurge}
              className="px-3 py-1.5 bg-rose-500 hover:bg-rose-600 text-white rounded text-[10px] font-bold cursor-pointer transition-colors shadow-sm flex items-center gap-1"
            >
              <RefreshCw className="w-3 h-3" />
              {purgedMsg ? 'Resynchronized successfully!' : 'Force System Database Purge'}
            </button>
          </div>

        </div>
      </div>

    </div>
  );
}
