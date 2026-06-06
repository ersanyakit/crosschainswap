/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import { useState } from 'react';
import { Search, Star, Folder, FolderOpen, Heart, TrendingUp, Sparkles, SlidersHorizontal, ChevronRight } from 'lucide-react';
import { MarketPair } from '../types/trading';
import { formatPrice } from '../utils/formatters';

interface CollapsibleSidebarProps {
  markets: MarketPair[];
  selectedPair: string;
  onSelectPair: (symbol: string) => void;
  onToggleFavorite: (symbol: string) => void;
  isSidebarOpen: boolean;
}

export default function CollapsibleSidebar({
  markets,
  selectedPair,
  onSelectPair,
  onToggleFavorite,
  isSidebarOpen,
}: CollapsibleSidebarProps) {
  const [searchQuery, setSearchQuery] = useState('');
  const [activeCategory, setActiveCategory] = useState<'ALL' | 'FAVORITES' | 'TRENDING'>('ALL');
  
  // File explorer tree states
  const [foldersOpen, setFoldersOpen] = useState({
    watchlist: true,
    spotMarkets: true,
    trending: false,
  });

  if (!isSidebarOpen) return null;

  // Filter products
  const filteredMarkets = markets.filter((m) => {
    const matchesSearch = m.symbol.toLowerCase().includes(searchQuery.toLowerCase()) || 
                          m.baseAsset.toLowerCase().includes(searchQuery.toLowerCase());
    
    if (activeCategory === 'FAVORITES') return matchesSearch && m.isFavorite;
    if (activeCategory === 'TRENDING') return matchesSearch && Math.abs(m.change24h) > 2; // high volatility
    return matchesSearch;
  });

  const favorites = markets.filter(m => m.isFavorite);
  const trending = [...markets].sort((a,b) => Math.abs(b.change24h) - Math.abs(a.change24h)).slice(0, 3);

  const toggleFolder = (folderKey: 'watchlist' | 'spotMarkets' | 'trending') => {
    setFoldersOpen(prev => ({ ...prev, [folderKey]: !prev[folderKey] }));
  };

  const renderMarketItem = (market: MarketPair) => {
    const isSelected = selectedPair === market.symbol;
    const isUp = market.change24h >= 0;

    return (
      <div
        key={market.symbol}
        onClick={() => onSelectPair(market.symbol)}
        className={`group flex items-center justify-between pl-8 pr-3 py-1.5 text-xs font-mono transition-all duration-150 cursor-pointer border-l-2 select-none ${
          isSelected
            ? 'bg-accent-2/60 text-accent-1 border-accent-1 font-medium'
            : 'text-gray-600 dark:text-gray-400 hover:bg-surface-3 border-transparent hover:text-gray-900 dark:hover:text-gray-100'
        }`}
      >
        <div className="flex items-center gap-1.5 min-w-0">
          <button
            onClick={(e) => {
              e.stopPropagation();
              onToggleFavorite(market.symbol);
            }}
            className="text-gray-300 hover:text-amber-400 dark:text-gray-600 dark:hover:text-amber-400 transition-colors cursor-pointer"
          >
            <Star
              className={`w-3.5 h-3.5 ${
                market.isFavorite ? 'fill-amber-400 text-amber-500' : ''
              }`}
            />
          </button>
          <span className="truncate">{market.symbol}</span>
        </div>

        <div className="flex items-center gap-2 text-right">
          <span className="font-semibold text-gray-800 dark:text-gray-200">
            {formatPrice(market.lastPrice)}
          </span>
          <span
            className={`text-[10px] w-12 px-1 text-center rounded font-medium ${
              isUp
                ? 'text-trade-green bg-trade-green-bg'
                : 'text-trade-red bg-trade-red-bg'
            }`}
          >
            {isUp ? '+' : ''}
            {market.change24h.toFixed(2)}%
          </span>
        </div>
      </div>
    );
  };

  return (
    <div className="w-64 max-w-full h-full flex flex-col border-r border-[#e1e4e8] dark:border-[#21262d] bg-[#fdfdfd] dark:bg-[#090d12] transition-colors duration-200 shrink-0">
      
      {/* Search Header */}
      <div className="p-3 border-b border-[#e1e4e8] dark:border-[#21262d]">
        <div className="flex items-center justify-between mb-2">
          <h2 className="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 font-display flex items-center gap-1.5">
            <SlidersHorizontal className="w-3.5 h-3.5 text-accent-1" />
            Spot Explorer
          </h2>
          <span className="text-[10px] font-mono text-gray-400 dark:text-gray-500">
            {filteredMarkets.length}/{markets.length} pairs
          </span>
        </div>

        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-gray-400" />
          <input
            type="text"
            placeholder="Search assets... (e.g. BTC)"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full pl-8 pr-3 py-1.5 text-xs font-mono rounded border border-[#e1e4e8] dark:border-[#21262d] bg-gray-50 dark:bg-[#0d1117] text-gray-900 dark:text-gray-100 placeholder-gray-400 hover:border-gray-300 dark:hover:border-gray-700 focus:outline-none focus:border-accent-1 focus:ring-1 focus:ring-accent-1/20 transition-all"
          />
        </div>

        {/* Quick Tabs inside Sidebar */}
        <div className="flex gap-1 mt-2.5">
          {(['ALL', 'FAVORITES', 'TRENDING'] as const).map((cat) => (
            <button
              key={cat}
              onClick={() => setActiveCategory(cat)}
              className={`flex-1 py-1 text-[9px] font-mono rounded font-medium border cursor-pointer transition-colors ${
                activeCategory === cat
                  ? 'bg-accent-1 text-white border-accent-1 shadow-sm'
                  : 'bg-[#f4f4f4] dark:bg-[#161b22] text-gray-500 dark:text-gray-400 border-transparent hover:text-gray-800 dark:hover:text-gray-200'
              }`}
            >
              {cat}
            </button>
          ))}
        </div>
      </div>

      {/* Directory-style List View */}
      <div className="flex-1 overflow-y-auto py-2">
        
        {/* Watchlist Folder */}
        {activeCategory === 'ALL' && (
          <div className="mb-2">
            <div
              onClick={() => toggleFolder('watchlist')}
              className="group flex items-center gap-1.5 px-3 py-1 text-xs font-mono font-medium text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-surface-3 cursor-pointer"
            >
              <div className="w-4 h-4 flex items-center justify-center">
                <ChevronRight className={`w-3.5 h-3.5 text-gray-400 transition-transform ${foldersOpen.watchlist ? 'rotate-90' : ''}`} />
              </div>
              {foldersOpen.watchlist ? <FolderOpen className="w-3.5 h-3.5 text-amber-400 shrink-0" /> : <Folder className="w-3.5 h-3.5 text-amber-400 shrink-0" />}
              <span className="truncate">★ Watchlist ({favorites.length})</span>
            </div>
            {foldersOpen.watchlist && (
              <div className="mt-0.5">
                {favorites.length === 0 ? (
                  <div className="pl-12 pr-3 py-2 text-[10px] text-gray-400 italic">
                    Add pairs to Watchlist
                  </div>
                ) : (
                  favorites.map(renderMarketItem)
                )}
              </div>
            )}
          </div>
        )}

        {/* Main spotMarkets Folder */}
        <div className="mb-2">
          <div
            onClick={() => toggleFolder('spotMarkets')}
            className="group flex items-center gap-1.5 px-3 py-1 text-xs font-mono font-medium text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-surface-3 cursor-pointer"
          >
            <div className="w-4 h-4 flex items-center justify-center">
              <ChevronRight className={`w-3.5 h-3.5 text-gray-400 transition-transform ${foldersOpen.spotMarkets ? 'rotate-90' : ''}`} />
            </div>
            {foldersOpen.spotMarkets ? <FolderOpen className="w-3.5 h-3.5 text-sky-400 shrink-0" /> : <Folder className="w-3.5 h-3.5 text-sky-400 shrink-0" />}
            <span className="truncate">📁 Spot Markets ({filteredMarkets.length})</span>
          </div>
          {foldersOpen.spotMarkets && (
            <div className="mt-0.5">
              {filteredMarkets.length === 0 ? (
                <div className="pl-12 pr-3 py-2 text-[10px] text-gray-400 italic">
                  No matching assets
                </div>
              ) : (
                filteredMarkets.map(renderMarketItem)
              )}
            </div>
          )}
        </div>

        {/* Hot / Trending Folder */}
        {activeCategory === 'ALL' && (
          <div className="mb-2">
            <div
              onClick={() => toggleFolder('trending')}
              className="group flex items-center gap-1.5 px-3 py-1 text-xs font-mono font-medium text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-surface-3 cursor-pointer"
            >
              <div className="w-4 h-4 flex items-center justify-center">
                <ChevronRight className={`w-3.5 h-3.5 text-gray-400 transition-transform ${foldersOpen.trending ? 'rotate-90' : ''}`} />
              </div>
              {foldersOpen.trending ? <FolderOpen className="w-3.5 h-3.5 text-rose-400 shrink-0" /> : <Folder className="w-3.5 h-3.5 text-rose-400 shrink-0" />}
              <span className="truncate">🔥 Volatile Movers</span>
            </div>
            {foldersOpen.trending && (
              <div className="mt-0.5">
                {trending.map(renderMarketItem)}
              </div>
            )}
          </div>
        )}

      </div>

      {/* Workspace Shortcut footer */}
      <div className="p-3 border-t border-[#e1e4e8] dark:border-[#21262d] bg-[#f9fafc] dark:bg-[#070b0f] font-mono text-[10px] text-gray-400 space-y-1">
        <div className="flex justify-between">
          <span>Command Palette:</span>
          <kbd className="px-1 bg-gray-200 dark:bg-[#21262d] text-gray-700 dark:text-gray-300 rounded font-semibold text-[9px]">Ctrl+K</kbd>
        </div>
        <div className="flex justify-between">
          <span>Toggle Sidebar:</span>
          <kbd className="px-1 bg-gray-200 dark:bg-[#21262d] text-gray-700 dark:text-gray-300 rounded font-semibold text-[9px]">Ctrl+B</kbd>
        </div>
      </div>
    </div>
  );
}
