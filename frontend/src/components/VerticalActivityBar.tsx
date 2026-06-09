/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import { TrendingUp, Coins, Briefcase, Wallet, Terminal, Settings, RefreshCw, LogIn, LogOut, User } from 'lucide-react';
import { BRAND_INITIAL } from '../constants/brand';

interface VerticalActivityBarProps {
  activeView: string;
  setActiveView: (view: string) => void;
  openOrdersCount: number;
  connectionStatus: 'connected' | 'reconnecting' | 'disconnected';
  latency: number;
  triggerRefresh: () => void;
  isSidebarOpen: boolean;
  setIsSidebarOpen: (open: boolean) => void;
  isAuthenticated: boolean;
  authEnabled: boolean;
  authLoading: boolean;
  accountLabel?: string;
  onLogin: () => void;
  onLogout: () => void;
  onAuthRetry: () => void;
}

export default function VerticalActivityBar({
  activeView,
  setActiveView,
  openOrdersCount,
  connectionStatus,
  latency,
  triggerRefresh,
  isSidebarOpen,
  setIsSidebarOpen,
  isAuthenticated,
  authEnabled,
  authLoading,
  accountLabel,
  onLogin,
  onLogout,
  onAuthRetry,
}: VerticalActivityBarProps) {
  const items = [
    { id: 'MARKETS', label: 'Markets Explorer', icon: TrendingUp },
    { id: 'TRADE', label: 'Trading Desk', icon: Terminal },
    { id: 'PORTFOLIO', label: 'Portfolio Analytics', icon: Briefcase },
    { id: 'ORDERS', label: 'Orders Ledger', icon: Coins, badge: openOrdersCount > 0 ? openOrdersCount : undefined },
    { id: 'WALLET', label: 'Secured Wallet', icon: Wallet },
  ];

  return (
    <div className="w-14 sm:w-16 h-full flex flex-col justify-between items-center py-4 border-r border-[#e1e4e8] dark:border-[#21262d] bg-[#f6f8fa] dark:bg-[#0d1117] transition-colors duration-200 z-10 select-none">
      {/* Top Logo / Launcher */}
      <div className="flex flex-col items-center gap-6 w-full">
        <button
          onClick={() => setIsSidebarOpen(!isSidebarOpen)}
          className="relative group p-2 hover:bg-surface-3 transition-colors rounded-lg flex items-center justify-center cursor-pointer"
          title="Toggle Explorer Sidebar (Ctrl+B)"
        >
          <div className="w-7 h-7 rounded-md bg-accent-1 flex items-center justify-center text-white font-mono font-bold text-xs shadow-md ide-glow">
            {BRAND_INITIAL}
          </div>
          <span className="absolute left-16 px-2 py-1 bg-gray-900 text-white text-xs rounded opacity-0 pointer-events-none group-hover:opacity-100 transition-opacity whitespace-nowrap z-50 shadow-lg">
            Toggle Sidebar (Ctrl+B)
          </span>
        </button>

        {/* Navigation Items */}
        <div className="flex flex-col gap-3 w-full px-2">
          {items.map((item) => {
            const Icon = item.icon;
            const isActive = activeView === item.id;
            return (
              <button
                key={item.id}
                onClick={() => {
                  setActiveView(item.id);
                  if (item.id === 'TRADE') {
                    // Automatically open sidebar for markets when focusing on trade
                    setIsSidebarOpen(true);
                  }
                }}
                className={`relative group flex items-center justify-center p-3 rounded-lg transition-all duration-200 cursor-pointer ${
                  isActive
                    ? 'text-accent-1 bg-accent-2-solid dark:bg-[#1f1924]/60 border border-accent-1/25 shadow-sm'
                    : 'text-gray-500 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-surface-3'
                }`}
              >
                <Icon className={`w-5 h-5 ${isActive ? 'stroke-[2px]' : 'stroke-[1.5px]'}`} />
                {isActive && (
                  <div className="absolute left-0 w-1 h-6 bg-accent-1 rounded-r-full" />
                )}
                {item.badge !== undefined && (
                  <div className="absolute top-1 right-1 px-1.5 py-0.5 bg-accent-1 text-white text-[9px] font-mono font-semibold rounded-full min-w-4 h-4 flex items-center justify-center leading-none">
                    {item.badge}
                  </div>
                )}
                {/* Tooltip */}
                <span className="absolute left-16 px-2 py-1 bg-gray-900 text-white text-xs rounded opacity-0 pointer-events-none group-hover:opacity-100 transition-opacity whitespace-nowrap z-50 shadow-lg font-medium">
                  {item.label}
                </span>
              </button>
            );
          })}
        </div>
      </div>

      {/* Bottom Actions */}
      <div className="flex flex-col items-center gap-4 w-full px-2">
        {/* Identity launcher */}
        <button
          onClick={() => {
            if (isAuthenticated) {
              setActiveView('PORTFOLIO');
              return;
            }
            if (authEnabled) {
              onLogin();
              return;
            }
            onAuthRetry();
          }}
          disabled={authLoading}
          className={`relative group flex items-center justify-center p-3 rounded-lg transition-all duration-200 cursor-pointer disabled:cursor-not-allowed ${
            isAuthenticated
              ? 'text-accent-1 bg-accent-2-solid dark:bg-[#1f1924]/60 border border-accent-1/25 shadow-sm'
              : 'text-gray-500 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-surface-3 disabled:text-gray-400 dark:disabled:text-gray-600'
          }`}
          title={isAuthenticated ? `Profile: ${accountLabel || 'Exchange account'}` : authEnabled ? 'Login' : 'Retry Auth'}
        >
          {isAuthenticated ? (
            <User className="w-5 h-5 stroke-[1.5px]" />
          ) : (
            <LogIn className={`w-5 h-5 stroke-[1.5px] ${authLoading ? 'animate-pulse' : ''}`} />
          )}
          <span className="absolute left-16 px-2 py-1 bg-gray-900 text-white text-xs rounded opacity-0 pointer-events-none group-hover:opacity-100 transition-opacity whitespace-nowrap z-50 shadow-lg font-medium">
            {isAuthenticated ? (accountLabel || 'Profile') : authLoading ? 'Checking Auth...' : authEnabled ? 'Login' : 'Retry Auth'}
          </span>
        </button>

        {isAuthenticated && (
          <button
            onClick={onLogout}
            disabled={authLoading}
            className="relative group flex items-center justify-center p-3 rounded-lg text-rose-500 hover:text-rose-400 hover:bg-rose-500/10 border border-transparent hover:border-rose-500/20 transition-all duration-200 cursor-pointer disabled:cursor-not-allowed disabled:text-gray-500"
            title="Logout"
          >
            <LogOut className="w-5 h-5 stroke-[1.5px]" />
            <span className="absolute left-16 px-2 py-1 bg-gray-900 text-white text-xs rounded opacity-0 pointer-events-none group-hover:opacity-100 transition-opacity whitespace-nowrap z-50 shadow-lg font-medium">
              Logout
            </span>
          </button>
        )}

        {/* Network & Connection Indicators */}
        <button
          onClick={triggerRefresh}
          className="group relative p-2 text-gray-400 hover:text-accent-1 hover:bg-surface-3 rounded-lg transition-colors cursor-pointer"
          title="Refresh backend data"
        >
          <RefreshCw className="w-4 h-4 group-hover:rotate-180 transition-transform duration-500" />
          <span className="absolute left-16 px-2 py-1 bg-gray-900 text-white text-xs rounded opacity-0 pointer-events-none group-hover:opacity-100 transition-opacity whitespace-nowrap z-50 font-medium">
            Refresh Backend Data
          </span>
        </button>

        {/* Sync state details */}
        <div className="relative group flex items-center justify-center py-1">
          <div className="relative flex h-2.5 w-2.5">
            {connectionStatus === 'connected' && (
              <>
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-trade-green opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-trade-green"></span>
              </>
            )}
            {connectionStatus === 'reconnecting' && (
              <>
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-amber-400"></span>
              </>
            )}
            {connectionStatus === 'disconnected' && (
              <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-trade-red"></span>
            )}
          </div>
          {/* Tooltip */}
          <span className="absolute left-16 p-2 bg-gray-900 text-white text-xs rounded opacity-0 pointer-events-none group-hover:opacity-100 transition-opacity whitespace-nowrap z-50 font-mono text-left leading-relaxed shadow-lg">
            <span className="font-semibold block border-b border-gray-700 pb-1 mb-1">Backend status</span>
            Connection: {connectionStatus}<br/>
            Latency: {latency}ms<br/>
            Transport: REST/WS
          </span>
        </div>

        {/* Settings Launcher */}
        <button
          onClick={() => setActiveView('SETTINGS')}
          className={`relative group flex items-center justify-center p-3 rounded-lg transition-all duration-200 cursor-pointer ${
            activeView === 'SETTINGS'
              ? 'text-accent-1 bg-accent-2-solid dark:bg-[#1f1924]/60 border border-accent-1/25 shadow-sm'
              : 'text-gray-500 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-surface-3'
          }`}
        >
          <Settings className="w-5 h-5 stroke-[1.5px]" />
          <span className="absolute left-16 px-2 py-1 bg-gray-900 text-white text-xs rounded opacity-0 pointer-events-none group-hover:opacity-100 transition-opacity whitespace-nowrap z-50 shadow-lg font-medium">
            Terminal Preferences
          </span>
        </button>
      </div>
    </div>
  );
}
