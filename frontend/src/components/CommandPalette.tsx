/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import React, { useState, useEffect, useRef } from 'react';
import { Search, Command, ArrowRight, CornerDownLeft, Sliders, Sun, Moon, Wallet, Trash2, Eye } from 'lucide-react';

interface CommandAction {
  id: string;
  category: string;
  title: string;
  subtitle: string;
  shortcut?: string;
  icon: any;
  action: () => void;
}

interface CommandPaletteProps {
  isOpen: boolean;
  onClose: () => void;
  actions: CommandAction[];
}

export default function CommandPalette({ isOpen, onClose, actions }: CommandPaletteProps) {
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const paletteRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Close when clicking outside
  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (paletteRef.current && !paletteRef.current.contains(event.target as Node)) {
        onClose();
      }
    }
    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside);
      // Automatically focus search field
      setTimeout(() => inputRef.current?.focus(), 80);
    }
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [isOpen, onClose]);

  // Filter commands
  const filteredActions = actions.filter(
    (act) =>
      act.title.toLowerCase().includes(query.toLowerCase()) ||
      act.category.toLowerCase().includes(query.toLowerCase()) ||
      act.subtitle.toLowerCase().includes(query.toLowerCase())
  );

  // Reset selected cursor when search query changes
  useEffect(() => {
    setSelectedIndex(0);
  }, [query]);

  // Command palette keyboard navigation
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSelectedIndex((prev) => (prev + 1) % Math.max(1, filteredActions.length));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSelectedIndex((prev) => (prev - 1 + filteredActions.length) % Math.max(1, filteredActions.length));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (filteredActions[selectedIndex]) {
        filteredActions[selectedIndex].action();
        onClose();
        setQuery('');
      }
    } else if (e.key === 'Escape') {
      onClose();
    }
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 bg-slate-900/40 dark:bg-slate-950/70 backdrop-blur-xs z-50 flex items-start justify-center pt-20 px-4">
      
      {/* Floating container card */}
      <div
        ref={paletteRef}
        onKeyDown={handleKeyDown}
        className="w-full max-w-xl bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-2xl overflow-hidden flex flex-col max-h-[400px] ide-glow"
      >
        
        {/* Search header container */}
        <div className="flex items-center gap-2.5 px-4 py-3 border-b border-[#e1e4e8] dark:border-[#21262d] bg-[#fafbfc] dark:bg-[#0d1117]">
          <Search className="w-4 h-4 text-accent-1" />
          <input
            ref={inputRef}
            type="text"
            placeholder="Type a transaction terminal action or symbol... (e.g. Open BTC)"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="flex-1 bg-transparent border-none text-xs text-gray-900 dark:text-gray-150 focus:outline-none placeholder-gray-400 font-mono"
          />
          <kbd className="hidden sm:flex items-center gap-0.5 px-1.5 py-0.5 bg-gray-100 dark:bg-[#21262d] text-gray-400 border border-[#e1e4e8] dark:border-transparent rounded font-mono text-[9px] font-semibold select-none">
            ESC
          </kbd>
        </div>

        {/* Command list content */}
        <div className="flex-1 overflow-y-auto py-2">
          {filteredActions.length === 0 ? (
            <div className="px-4 py-8 text-center text-xs text-gray-400 font-mono italic">
              No matching terminal commands or pairs found. Try "Switch theme" or "BTC".
            </div>
          ) : (
            <div>
              {/* Grouped Header category helper */}
              {Object.entries(
                filteredActions.reduce((acc, action) => {
                  if (!acc[action.category]) acc[action.category] = [];
                  acc[action.category].push(action);
                  return acc;
                }, {} as Record<string, CommandAction[]>)
              ).map(([category, catActions]) => (
                <div key={category}>
                  <div className="px-4 py-1 text-[9px] font-mono font-bold uppercase tracking-widest text-[#7e8c9a]">
                    {category}
                  </div>
                  
                  {catActions.map((action) => {
                    const globalIndex = filteredActions.indexOf(action);
                    const isSelected = globalIndex === selectedIndex;
                    const Icon = action.icon;
                    return (
                      <div
                        key={action.id}
                        onClick={() => {
                          action.action();
                          onClose();
                          setQuery('');
                        }}
                        className={`flex items-center justify-between px-4 py-2 text-xs font-mono transition-colors cursor-pointer select-none ${
                          isSelected
                            ? 'bg-accent-2 text-accent-1 font-medium border-l-2 border-accent-1'
                            : 'text-gray-700 dark:text-gray-300 hover:bg-slate-50 dark:hover:bg-[#161b22]/50 border-l-2 border-transparent'
                        }`}
                      >
                        <div className="flex items-center gap-3">
                          <div className={`p-1 rounded ${isSelected ? 'text-accent-1' : 'text-gray-400'}`}>
                            <Icon className="w-3.5 h-3.5" />
                          </div>
                          <div>
                            <span className="block text-gray-900 dark:text-gray-100">{action.title}</span>
                            <span className="block text-[10px] text-gray-400 dark:text-gray-500">{action.subtitle}</span>
                          </div>
                        </div>

                        <div className="flex items-center gap-2">
                          {action.shortcut && (
                            <kbd className="px-1.5 py-0.5 bg-gray-100 dark:bg-[#161b22] text-gray-400 rounded text-[9px] font-bold border border-gray-200 dark:border-transparent">
                              {action.shortcut}
                            </kbd>
                          )}
                          {isSelected && (
                            <CornerDownLeft className="w-3 h-3 text-accent-1 animate-pulse" />
                          )}
                        </div>
                      </div>
                    );
                  })}
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Footer shortcuts helper info bar */}
        <div className="px-4 py-2 border-t border-[#e1e4e8] dark:border-[#21262d] bg-[#f9fafc] dark:bg-[#070b0f] flex justify-between text-[10px] text-gray-400 font-mono">
          <span className="flex items-center gap-1">
            <kbd className="px-1 bg-gray-200 dark:bg-[#161b22] text-gray-600 dark:text-gray-400 rounded font-bold">↑↓</kbd>
            <span>Navigate</span>
          </span>
          <span className="flex items-center gap-1">
            <kbd className="px-1 bg-gray-200 dark:bg-[#161b22] text-gray-600 dark:text-gray-400 rounded font-bold">Enter</kbd>
            <span>Select</span>
          </span>
          <span className="flex items-center gap-1">
            <kbd className="px-1 bg-gray-200 dark:bg-[#161b22] text-gray-600 dark:text-gray-400 rounded font-bold">Ctrl+K</kbd>
            <span>Toggle Panel</span>
          </span>
        </div>

      </div>
    </div>
  );
}
