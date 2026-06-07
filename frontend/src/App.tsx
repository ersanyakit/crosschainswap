/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import React, { useState, useEffect, useTransition, useCallback, useRef } from 'react';
import { Sparkles, Cpu, Coins, ShieldCheck, Heart, User, Sun, Moon, CheckSquare, Layers, Code, Play, Wallet, Trash2, Briefcase, X } from 'lucide-react';
import VerticalActivityBar from './components/VerticalActivityBar';
import CollapsibleSidebar from './components/CollapsibleSidebar';
import MarketChart from './components/MarketChart';
import OrderBookView from './components/OrderBook';
import RecentTrades from './components/RecentTrades';
import OrderForm from './components/OrderForm';
import TerminalPanel from './components/TerminalPanel';
import CommandPalette from './components/CommandPalette';
import PortfolioView from './components/PortfolioView';
import StrategyLab from './components/StrategyLab';
import SettingsView from './components/SettingsModal';
import LoginScreen from './components/LoginScreen';
import AssetIcon from './components/AssetIcon';
import { BRAND_DOCUMENT_TITLE, BRAND_NAME } from './constants/brand';

import {
  INITIAL_MARKETS,
  INITIAL_BALANCES,
  INITIAL_ORDERS,
  INITIAL_LOGS,
  INITIAL_STRATEGIES,
  generateCandles,
  generateRecentTrades
} from './data/mockData';

import { MarketPair, Candle, Timeframe, Order, Trade, SystemLog, AssetBalance, TradingStrategy, OrderType, OrderSide, OrderBook } from './types/trading';
import {
  type AssetInfo,
  type AssetPriceResponse,
  cancelOrder as cancelExchangeOrder,
  exchangeConfig,
  fetchAssets,
  fetchAssetPrices,
  fetchBalances,
  fetchCandles,
  fetchMarketTrades,
  fetchOrderBook,
  fetchUserOrders,
  fetchUserTrades,
  healthCheck,
  listMarkets,
  openExchangeSocket,
  openPriceSocket,
  placeOrder as placeExchangeOrder,
  settleDeposit as settleExchangeDeposit,
} from './services/exchangeService';
import {
  AuthUser,
  fetchAuthSession,
  fetchAuthStatus,
  logout as logoutOIDC,
  oidcLoginURL,
} from './services/authService';
import { formatPrice } from './utils/formatters';

interface Tab {
  id: string;
  title: string;
  type: 'MARKET' | 'PORTFOLIO' | 'STRATEGY_LAB' | 'CUSTOM_PAIR';
  symbol?: string;
}

type ExchangeMode = 'connecting' | 'live' | 'fallback';
type ThemeMode = 'light' | 'dark';

const THEME_STORAGE_KEY = 'kewl.theme';

function initialTheme(): ThemeMode {
  if (typeof window === 'undefined') return 'dark';
  const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
  if (stored === 'light' || stored === 'dark') return stored;
  return 'dark';
}

export default function App() {
  const [isPending, startTransition] = useTransition();
  const exchangeModeRef = useRef<ExchangeMode>('connecting');

  // Primary Workspace views: MARKETS, TRADE (docked terminal), PORTFOLIO, ORDERS, WALLET, STRATEGY_LAB, SETTINGS
  const [activeView, setActiveView] = useState<string>('TRADE');
  const [isSidebarOpen, setIsSidebarOpen] = useState(true);
  const [selectedPairSymbol, setSelectedPairSymbol] = useState('PEPPER/USD');

  // Command palette toggle state
  const [isCommandPaletteOpen, setIsCommandPaletteOpen] = useState(false);

  // Layout preferences states
  const [theme, setTheme] = useState<ThemeMode>(() => initialTheme());
  const [density, setDensity] = useState<'compact' | 'comfortable'>('compact');
  const [confirmOrders, setConfirmOrders] = useState(true);
  const [soundEnabled, setSoundEnabled] = useState(true);

  // Core exchange data structures states
  const [markets, setMarkets] = useState<MarketPair[]>(INITIAL_MARKETS);
  const [balances, setBalances] = useState<AssetBalance[]>(INITIAL_BALANCES);
  const [openOrders, setOpenOrders] = useState<Order[]>(INITIAL_ORDERS.filter(o => o.status === 'PENDING'));
  const [orderHistory, setOrderHistory] = useState<Order[]>(INITIAL_ORDERS.filter(o => o.status !== 'PENDING'));
  
  // Historical executions
  const selectedMarketObj = markets.find(m => m.symbol === selectedPairSymbol) || markets[0];
  const [tradeHistory, setTradeHistory] = useState<Trade[]>(() => generateRecentTrades(selectedMarketObj.lastPrice));
  const [activeOrderBook, setActiveOrderBook] = useState<OrderBook>(() => emptyOrderBook());
  const [orderSubmitError, setOrderSubmitError] = useState<string | null>(null);
  const [exchangeMode, setExchangeMode] = useState<ExchangeMode>('connecting');
  const [exchangeMessage, setExchangeMessage] = useState('Probing exchange API');
  const [protocolRevision, setProtocolRevision] = useState(0);
  const [authLoading, setAuthLoading] = useState(true);
  const [authEnabled, setAuthEnabled] = useState(false);
  const [authProvider, setAuthProvider] = useState('RESEARCHCAVE');
  const [authUser, setAuthUser] = useState<AuthUser | null>(null);
  const [authError, setAuthError] = useState('');
  const activeUserID = authUser?.sub || exchangeConfig.userID;
  const selectedAssetSymbol = selectedMarketObj.baseAsset || selectedPairSymbol.split('/')[0] || 'PEPPER';
  const [dexPrices, setDexPrices] = useState<AssetPriceResponse | null>(null);
  const [dexPricesLoading, setDexPricesLoading] = useState(false);
  const [dexPricesError, setDexPricesError] = useState<string | null>(null);
  const [assetMetadata, setAssetMetadata] = useState<Record<string, AssetInfo>>({});

  // Visual terminal logs and strategies
  const [systemLogs, setSystemLogs] = useState<SystemLog[]>(INITIAL_LOGS);
  const [strategies, setStrategies] = useState<TradingStrategy[]>(INITIAL_STRATEGIES);

  // Wallet transaction ledger (Deposits/Withdrawals)
  const [walletTransactions, setWalletTransactions] = useState<Array<{ id: string; type: string; asset: string; amount: number; time: Date }>>([
    { id: 'TX-98210', type: 'DEPOSIT', asset: 'USD', amount: 15000, time: new Date(Date.now() - 5 * 24 * 60 * 60 * 1000) }
  ]);

  // Map of active candlesticks series for the loaded asset pair
  const [timeframe, setTimeframe] = useState<Timeframe>('15m');
  const [candles, setCandles] = useState<Candle[]>([]);

  // Selected pricing feedback from Order book click (to flow into Order Form)
  const [orderBookSelectedPrice, setOrderBookSelectedPrice] = useState<number | null>(null);

  // Connection parameters
  const [connectionStatus, setConnectionStatus] = useState<'connected' | 'reconnecting' | 'disconnected'>('connected');
  const [latency, setLatency] = useState(16);

  // Active Workspace Open editor tabs
  const [openTabs, setOpenTabs] = useState<Tab[]>([
    { id: 'PEPPER/USD', title: 'PEPPER/USD', type: 'MARKET', symbol: 'PEPPER/USD' },
    { id: 'CHZ/USD', title: 'CHZ/USD', type: 'MARKET', symbol: 'CHZ/USD' },
    { id: 'SOL/USD', title: 'SOL/USD', type: 'MARKET', symbol: 'SOL/USD' },
    { id: 'PORTFOLIO', title: 'Portfolio.json', type: 'PORTFOLIO' },
    { id: 'STRATEGY_LAB', title: 'strategy.ts', type: 'STRATEGY_LAB' }
  ]);
  const [activeTabId, setActiveTabId] = useState<string>('PEPPER/USD');

  // Log appender helper
  const appendLog = useCallback((message: string, source: 'SYSTEM' | 'ORDER' | 'WEBSOCKET' | 'STRATEGY', type: 'INFO' | 'SUCCESS' | 'WARNING' | 'ERROR') => {
    const newLog: SystemLog = {
      id: `LOG-${Math.random().toString(36).substring(2, 9).toUpperCase()}`,
      timestamp: new Date(),
      type,
      source,
      message,
    };
    setSystemLogs(prev => [newLog, ...prev.slice(0, 70)]);
  }, []);

  const refreshAuth = useCallback(async () => {
    setAuthLoading(true);
    setAuthError('');
    try {
      const status = await fetchAuthStatus();
      setAuthEnabled(status.enabled);
      setAuthProvider(status.provider || 'OIDC');
      if (!status.enabled) {
        setAuthUser(null);
        return;
      }

      const session = await fetchAuthSession();
      setAuthUser(session.authenticated ? session.user || null : null);
    } catch (err) {
      setAuthError(err instanceof Error ? err.message : 'Auth service unavailable');
      setAuthUser(null);
      setAuthEnabled(false);
    } finally {
      setAuthLoading(false);
    }
  }, []);

  useEffect(() => {
    refreshAuth();
  }, [refreshAuth]);

  // Load custom CSS theme modifier block on mounting change
  useEffect(() => {
    const root = document.documentElement;
    if (theme === 'dark') {
      root.classList.add('theme-dark', 'dark');
      root.classList.remove('theme-light');
      root.style.colorScheme = 'dark';
    } else {
      root.classList.add('theme-light');
      root.classList.remove('theme-dark', 'dark');
      root.style.colorScheme = 'light';
    }
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
  }, [theme]);

  // Global key listening (Ctrl+K for command palette, Ctrl+B for Sidebar toggle)
  useEffect(() => {
    document.title = BRAND_DOCUMENT_TITLE;
  }, []);

  useEffect(() => {
    let cancelled = false;

    fetchAssets()
      .then((assets) => {
        if (!cancelled) {
          setAssetMetadata(assetMetadataBySymbol(assets));
        }
      })
      .catch((err) => {
        appendLog(`Asset registry metadata failed: ${err instanceof Error ? err.message : 'unknown asset metadata error'}`, 'SYSTEM', 'WARNING');
      });

    return () => {
      cancelled = true;
    };
  }, [appendLog]);

  useEffect(() => {
    const handleGlobalKeys = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setIsCommandPaletteOpen(prev => !prev);
      }
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'b') {
        e.preventDefault();
        setIsSidebarOpen(prev => !prev);
      }
    };
    window.addEventListener('keydown', handleGlobalKeys);
    return () => window.removeEventListener('keydown', handleGlobalKeys);
  }, []);

  // System heartbeat walk timer (runs every 1.8s)
  useEffect(() => {
    let wsHeartbeat = setInterval(() => {
      // 1. Simulates connection fluctuations slightly
      setLatency(10 + Math.floor(Math.random() * 15));

      // 2. Selectively walks quotes of All Spot pairs slightly to simulate live feed action
      if (exchangeMode === 'live') return;
      setMarkets((prevMarkets) => {
        return prevMarkets.map((m) => {
          const isSelected = m.symbol === selectedPairSymbol;
          const walkFactor = isSelected ? 0.0004 : 0.0006;
          const fluctuation = m.lastPrice * walkFactor * (Math.random() > 0.48 ? 1 : -1);
          const nextPrice = Number((m.lastPrice + fluctuation).toFixed(2));
          const high24h = Math.max(m.high24h, nextPrice);
          const low24h = Math.min(m.low24h, nextPrice);

          return {
            ...m,
            lastPrice: nextPrice,
            high24h,
            low24h,
          };
        });
      });
    }, 1800);

    return () => clearInterval(wsHeartbeat);
  }, [selectedPairSymbol, exchangeMode]);

  // Monitor price shifts in real-time to match candle plots, recent trade feeds, and audit standing limit orders
  useEffect(() => {
    if (exchangeMode === 'live') return;
    const activeMarket = markets.find(m => m.symbol === selectedPairSymbol) || markets[0];
    const livePrice = activeMarket.lastPrice;

    // A. Update historical Candlestick sequences with the latest walkers
    setCandles((prevCandles) => {
      if (prevCandles.length === 0) return prevCandles;
      const copy = [...prevCandles];
      const lastCandle = { ...copy[copy.length - 1] };
      lastCandle.close = livePrice;
      lastCandle.high = Math.max(lastCandle.high, livePrice);
      lastCandle.low = Math.min(lastCandle.low, livePrice);
      copy[copy.length - 1] = lastCandle;
      return copy;
    });

    // B. Append simulated transaction match trades to the feed block
    if (Math.random() > 0.45) {
      const isBuy = Math.random() > 0.48;
      const amt = Number((0.005 + Math.random() * 1.5).toFixed(4));
      const trObj: Trade = {
        id: `TR-${Math.random().toString(36).substring(2, 9).toUpperCase()}`,
        symbol: selectedPairSymbol,
        price: livePrice,
        amount: amt,
        total: Number((livePrice * amt).toFixed(2)),
        side: isBuy ? 'BUY' : 'SELL',
        timestamp: new Date(),
      };
      setTradeHistory(prev => [trObj, ...prev.slice(0, 40)]);
    }

    // C. Evaluate and trigger pending open limit orders
    setOpenOrders((prevOpen) => {
      const remaining: Order[] = [];
      const triggered: Order[] = [];

      prevOpen.forEach((ord) => {
        if (ord.symbol !== selectedPairSymbol) {
          remaining.push(ord);
          return;
        }

        let isTriggered = false;
        if (ord.side === 'BUY') {
          // Buy triggers if price drops below or meets the limit target
          if (livePrice <= ord.price) isTriggered = true;
        } else {
          // Sell triggers if price rises above or meets the limit target
          if (livePrice >= ord.price) isTriggered = true;
        }

        if (isTriggered) {
          triggered.push({
            ...ord,
            status: 'FILLED',
            filled: ord.amount,
            timestamp: new Date(),
          });
        } else {
          remaining.push(ord);
        }
      });

      // Execute balance updates, log registers, and move filled ones to historical arrays
      if (triggered.length > 0) {
        triggered.forEach((ord) => {
          // Log success message in system
          appendLog(
            `Standing order target reached! FILLED ${ord.side} ${ord.amount} ${ord.symbol} at rate $${ord.price}.`,
            'ORDER',
            'SUCCESS'
          );

          // Calculate fees
          const orderTotal = ord.amount * ord.price;
          const estFee = orderTotal * 0.0008;

          // Adjust wallets balances
          setBalances((prevBalances) => {
            const copy = [...prevBalances];
            const baseAssetIndex = copy.findIndex(b => b.asset === ord.symbol.split('/')[0]);
            const quoteAsset = ord.symbol.split('/')[1] || selectedMarketObj.quoteAsset;
            const quoteAssetIndex = copy.findIndex(b => b.asset === quoteAsset);

            if (ord.side === 'BUY') {
              // Lock released, cash spent, assets added
              if (quoteAssetIndex !== -1) {
                copy[quoteAssetIndex].locked = Math.max(0, copy[quoteAssetIndex].locked - orderTotal);
              }
              if (baseAssetIndex !== -1) {
                copy[baseAssetIndex].free += ord.amount;
                copy[baseAssetIndex].valueUsd = (copy[baseAssetIndex].free + copy[baseAssetIndex].locked) * livePrice;
              }
            } else {
              // Assets unlocked, sold, cash added
              if (baseAssetIndex !== -1) {
                copy[baseAssetIndex].locked = Math.max(0, copy[baseAssetIndex].locked - ord.amount);
                copy[baseAssetIndex].valueUsd = (copy[baseAssetIndex].free + copy[baseAssetIndex].locked) * livePrice;
              }
              if (quoteAssetIndex !== -1) {
                copy[quoteAssetIndex].free += (orderTotal - estFee);
                copy[quoteAssetIndex].valueUsd = copy[quoteAssetIndex].free;
              }
            }
            return copy;
          });

          // Register filled matches in account trade logs
          const myTrade: Trade = {
            id: `MY-${Math.random().toString(36).substring(2, 8).toUpperCase()}`,
            symbol: ord.symbol,
            price: ord.price,
            amount: ord.amount,
            total: orderTotal,
            side: ord.side,
            timestamp: new Date(),
          };
          setTradeHistory(prev => [myTrade, ...prev]);
          setOrderHistory(prev => [ord, ...prev]);
        });
      }

      return remaining;
    });

    // D. Simulate Running coded trading strategies
    strategies.forEach((strat) => {
      if (strat.status === 'RUNNING') {
        // Small chance ~1.5% each tick to register simulated execution trades!
        if (Math.random() < 0.015) {
          const isBuy = Math.random() > 0.45;
          const size = Number((0.01 + Math.random() * 0.05).toFixed(4));
          const sizeUsd = size * livePrice;
          const directionLabel = isBuy ? 'BUY' : 'SELL';

          appendLog(
            `Strategy Board [${strat.name}]: evaluation trigger -> Match target. Executed Market ${directionLabel} ${size} ${selectedPairSymbol} at ${livePrice}.`,
            'STRATEGY',
            'INFO'
          );

          // Update balances
          setBalances(prevBal => {
            const copy = [...prevBal];
            const baseIdx = copy.findIndex(b => b.asset === selectedPairSymbol.split('/')[0]);
            const quoteAsset = selectedPairSymbol.split('/')[1] || selectedMarketObj.quoteAsset;
            const quoteIdx = copy.findIndex(b => b.asset === quoteAsset);

            if (isBuy) {
              if (quoteIdx !== -1 && copy[quoteIdx].free >= sizeUsd) {
                copy[quoteIdx].free -= sizeUsd;
                if (baseIdx !== -1) copy[baseIdx].free += size;
              }
            } else {
              if (baseIdx !== -1 && copy[baseIdx].free >= size) {
                copy[baseIdx].free -= size;
                if (quoteIdx !== -1) copy[quoteIdx].free += sizeUsd;
              }
            }
            return copy;
          });
        }
      }
    });

  }, [markets, selectedPairSymbol, strategies, exchangeMode, appendLog]);

  useEffect(() => {
    let cancelled = false;

    const refreshExchangeSnapshot = async () => {
      setDexPricesLoading(true);
      setDexPricesError(null);
      try {
        await healthCheck();
        const remoteMarkets = await listMarkets();
        if (cancelled) return;

        if (remoteMarkets.length > 0) {
          setMarkets(prev => mergeMarkets(prev, remoteMarkets));

          if (!remoteMarkets.some(m => m.symbol === selectedPairSymbol)) {
            const firstMarket = remoteMarkets[0];
            setSelectedPairSymbol(firstMarket.symbol);
            setActiveTabId(firstMarket.symbol);
            setOpenTabs(prev => replaceMarketTabs(prev, remoteMarkets));
            return;
          }
        }

        const [
          orderBookResult,
          candlesResult,
          marketTradesResult,
          userOrdersResult,
          userTradesResult,
          balancesResult,
          assetPricesResult,
        ] = await Promise.allSettled([
          fetchOrderBook(selectedPairSymbol, 50),
          fetchCandles(selectedPairSymbol, timeframe, 120),
          fetchMarketTrades(selectedPairSymbol, 80),
          fetchUserOrders(activeUserID, selectedPairSymbol, 100),
          fetchUserTrades(activeUserID, selectedPairSymbol, 100),
          fetchBalances(activeUserID),
          fetchAssetPrices(selectedAssetSymbol),
        ]);

        if (cancelled) return;

        if (orderBookResult.status === 'fulfilled') {
          setActiveOrderBook(orderBookResult.value);
        }
        if (candlesResult.status === 'fulfilled') {
          const nextCandles = candlesResult.value.length > 0
            ? candlesResult.value
            : generateCandles(selectedPairSymbol, timeframe, 120);
          setCandles(nextCandles);
          const lastCandle = nextCandles[nextCandles.length - 1];
          setMarkets(prev => prev.map(m => m.symbol === selectedPairSymbol ? {
            ...m,
            lastPrice: lastCandle.close,
            high24h: Math.max(m.high24h, lastCandle.high),
            low24h: Math.min(m.low24h, lastCandle.low),
          } : m));
        } else {
          setCandles(generateCandles(selectedPairSymbol, timeframe, 120));
        }
        if (marketTradesResult.status === 'fulfilled' || userTradesResult.status === 'fulfilled') {
          const marketTrades = marketTradesResult.status === 'fulfilled' ? marketTradesResult.value : [];
          const userTrades = userTradesResult.status === 'fulfilled' ? userTradesResult.value : [];
          const byID = new Map<string, Trade>();
          [...userTrades, ...marketTrades].forEach(item => byID.set(item.id, item));
          if (byID.size > 0) {
            setTradeHistory(Array.from(byID.values()).sort((a, b) => b.timestamp.getTime() - a.timestamp.getTime()));
          }
        }
        if (userOrdersResult.status === 'fulfilled') {
          setOpenOrders(userOrdersResult.value.filter(o => o.status === 'PENDING'));
          setOrderHistory(userOrdersResult.value.filter(o => o.status !== 'PENDING'));
        }
        if (balancesResult.status === 'fulfilled') {
          setBalances(balancesResult.value);
        } else {
          setBalances([]);
        }
        if (assetPricesResult.status === 'fulfilled') {
          setDexPrices(assetPricesResult.value);
          setDexPricesError(null);
        } else {
          setDexPricesError(assetPricesResult.reason instanceof Error ? assetPricesResult.reason.message : 'DEX prices unavailable');
        }

        setConnectionStatus('connected');
        setExchangeMode('live');
        setExchangeMessage(`REST/WS bound to ${exchangeConfig.apiBaseURL}`);
        if (exchangeModeRef.current !== 'live') {
          appendLog('Exchange service connected. Limit order protocol is now REST/WS authoritative.', 'WEBSOCKET', 'SUCCESS');
        }
        exchangeModeRef.current = 'live';
      } catch (err) {
        if (cancelled) return;
        setConnectionStatus('disconnected');
        setExchangeMode('fallback');
        setExchangeMessage(err instanceof Error ? err.message : 'Exchange API unavailable');
        if (exchangeModeRef.current !== 'fallback') {
          appendLog('Exchange service unavailable. Workspace continues in local simulation fallback.', 'WEBSOCKET', 'WARNING');
        }
        exchangeModeRef.current = 'fallback';
        setDexPrices(null);
        setDexPricesError(err instanceof Error ? err.message : 'DEX prices unavailable');
      } finally {
        if (!cancelled) setDexPricesLoading(false);
      }
    };

    refreshExchangeSnapshot();
    const refreshTimer = window.setInterval(refreshExchangeSnapshot, 8000);
    return () => {
      cancelled = true;
      window.clearInterval(refreshTimer);
    };
  }, [selectedPairSymbol, selectedAssetSymbol, timeframe, protocolRevision, appendLog, activeUserID]);

  useEffect(() => {
    if (exchangeMode !== 'live') return;

    const socket = openExchangeSocket((event) => {
      if (event?.market && event.market !== selectedPairSymbol) return;
      setProtocolRevision(rev => rev + 1);
      if (event?.type === 'exchange.orderbook_updated') {
        appendLog(`Order book invalidated for ${event.market}. Pulling fresh depth snapshot.`, 'WEBSOCKET', 'INFO');
      } else if (event?.type?.startsWith('exchange.order_')) {
        appendLog(`Protocol event received: ${event.type}`, 'ORDER', 'INFO');
      }
    });

    socket.onopen = () => {
      setConnectionStatus('connected');
      setExchangeMessage(`REST/WS bound to ${exchangeConfig.apiBaseURL}`);
    };
    socket.onclose = () => {
      setConnectionStatus('reconnecting');
      setExchangeMessage('Websocket reconnect pending; REST polling remains active');
    };
    socket.onerror = () => {
      setConnectionStatus('reconnecting');
    };

    return () => socket.close();
  }, [exchangeMode, selectedPairSymbol, appendLog]);

  useEffect(() => {
    if (exchangeMode !== 'live') return;

    const socket = openPriceSocket((event) => {
      if (event?.type !== 'prices.updated' || !event.data?.symbol) return;
      if (String(event.data.symbol).toUpperCase() !== selectedAssetSymbol.toUpperCase()) return;
      setDexPrices(prev => mergeAssetPrices(prev, event.data as AssetPriceResponse));
      setDexPricesError(null);
      setDexPricesLoading(false);
    });

    socket.onopen = () => {
      appendLog(`Price stream subscribed for ${selectedAssetSymbol}.`, 'WEBSOCKET', 'SUCCESS');
    };
    socket.onerror = () => {
      setDexPricesError('DEX price stream unavailable');
    };

    return () => socket.close();
  }, [exchangeMode, selectedAssetSymbol, appendLog]);

  // Order Submit placement worker
  const handleOrderSubmit = async (ordData: {
    side: OrderSide;
    type: OrderType;
    price: number;
    amount: number;
    stopPrice?: number;
  }) => {
    if (exchangeMode === 'live') {
      try {
        setOrderSubmitError(null);
        appendLog(`Submitting ${ordData.type} ${ordData.side} through exchange limit protocol.`, 'ORDER', 'INFO');
        const result = await placeExchangeOrder({
          market: selectedPairSymbol,
          userID: activeUserID,
          side: ordData.side,
          type: ordData.type,
          price: ordData.price,
          amount: ordData.amount,
          stopPrice: ordData.stopPrice,
        });

        if (result.order.status === 'PENDING') {
          setOpenOrders(prev => [result.order, ...prev.filter(o => o.id !== result.order.id)]);
        } else {
          setOrderHistory(prev => [result.order, ...prev.filter(o => o.id !== result.order.id)]);
        }
        if (result.trades.length > 0) {
          setTradeHistory(prev => [...result.trades, ...prev]);
        }
        setProtocolRevision(rev => rev + 1);
        appendLog(`Exchange accepted ${result.order.id}: ${result.order.status} ${result.order.filled.toFixed(4)}/${result.order.amount.toFixed(4)}.`, 'ORDER', 'SUCCESS');
      } catch (err) {
        const message = err instanceof Error ? err.message : 'unknown protocol error';
        setOrderSubmitError(message);
        appendLog(`Exchange order rejected: ${message}`, 'ORDER', 'ERROR');
      }
      return;
    }

    const totalOrderValue = ordData.price * ordData.amount;
    const estFee = totalOrderValue * 0.0008;

    // A. Check Balances validation
    const quoteAsset = selectedMarketObj.quoteAsset;
    const baseAsset = selectedPairSymbol.split('/')[0];

    const walletQuote = balances.find(b => b.asset === quoteAsset);
    const walletBase = balances.find(b => b.asset === baseAsset);

    if (ordData.side === 'BUY') {
      if (!walletQuote || walletQuote.free < totalOrderValue) {
        appendLog(`Order submission failed: Insufficient ${quoteAsset} reserves. Required: ${totalOrderValue.toFixed(2)} ${quoteAsset}`, 'ORDER', 'ERROR');
        return;
      }
    } else {
      if (!walletBase || walletBase.free < ordData.amount) {
        appendLog(`Order submission failed: Insufficient ${baseAsset} coins. Required: ${ordData.amount}`, 'ORDER', 'ERROR');
        return;
      }
    }

    // B. Create standing order or match immediately for MARKET type
    if (ordData.type === 'MARKET') {
      // Deduct immediately, execute trade mapping, append history logs
      setBalances((prevBalances) => {
        const copy = [...prevBalances];
        const baseIdx = copy.findIndex(b => b.asset === baseAsset);
        const quoteIdx = copy.findIndex(b => b.asset === quoteAsset);

        if (ordData.side === 'BUY') {
          if (quoteIdx !== -1) copy[quoteIdx].free -= (totalOrderValue + estFee);
          if (baseIdx !== -1) copy[baseIdx].free += ordData.amount;
        } else {
          if (baseIdx !== -1) copy[baseIdx].free -= ordData.amount;
          if (quoteIdx !== -1) copy[quoteIdx].free += (totalOrderValue - estFee);
        }
        return copy;
      });

      const filledOrder: Order = {
        id: `ORD-${Math.floor(100000 + Math.random() * 900000)}`,
        symbol: selectedPairSymbol,
        side: ordData.side,
        type: ordData.type,
        price: ordData.price,
        amount: ordData.amount,
        filled: ordData.amount,
        total: totalOrderValue,
        status: 'FILLED',
        timestamp: new Date(),
      };

      const accountTrade: Trade = {
        id: `MY-${Math.random().toString(36).substring(2, 8).toUpperCase()}`,
        symbol: selectedPairSymbol,
        price: ordData.price,
        amount: ordData.amount,
        total: totalOrderValue,
        side: ordData.side,
        timestamp: new Date(),
      };

      setOrderHistory(prev => [filledOrder, ...prev]);
      setTradeHistory(prev => [accountTrade, ...prev]);
      appendLog(`Market spot order executed immediately: ${ordData.side} ${ordData.amount} ${selectedPairSymbol} at ${ordData.price} ${quoteAsset}`, 'ORDER', 'SUCCESS');
    } else {
      // LIMIT OR STOP_LIMIT - Place inside Standing Open array and lock relevant balances
      setBalances((prevBalances) => {
        const copy = [...prevBalances];
        const baseIdx = copy.findIndex(b => b.asset === baseAsset);
        const quoteIdx = copy.findIndex(b => b.asset === quoteAsset);

        if (ordData.side === 'BUY') {
          if (quoteIdx !== -1) {
            copy[quoteIdx].free -= totalOrderValue;
            copy[quoteIdx].locked += totalOrderValue;
          }
        } else {
          if (baseIdx !== -1) {
            copy[baseIdx].free -= ordData.amount;
            copy[baseIdx].locked += ordData.amount;
          }
        }
        return copy;
      });

      const pendingOrder: Order = {
        id: `ORD-${Math.floor(100000 + Math.random() * 900000)}`,
        symbol: selectedPairSymbol,
        side: ordData.side,
        type: ordData.type,
        price: ordData.price,
        amount: ordData.amount,
        filled: 0,
        total: totalOrderValue,
        stopPrice: ordData.stopPrice,
        status: 'PENDING',
        timestamp: new Date(),
      };

      setOpenOrders(prev => [pendingOrder, ...prev]);
      appendLog(`Limit order booked on terminal boards: ${ordData.side} ${ordData.amount} ${selectedPairSymbol} at rate ${ordData.price} ${quoteAsset}`, 'ORDER', 'INFO');
    }
  };

  // Cancele standing order
  const handleCancelOrder = async (id: string) => {
    if (exchangeMode === 'live') {
      try {
        const cancelled = await cancelExchangeOrder(id, activeUserID);
        setOpenOrders(prev => prev.filter(o => o.id !== id));
        setOrderHistory(prev => [cancelled, ...prev.filter(o => o.id !== id)]);
        setProtocolRevision(rev => rev + 1);
        appendLog(`Cancelled exchange order ${id}. Backend released reserved funds.`, 'ORDER', 'WARNING');
      } catch (err) {
        appendLog(`Cancel rejected by exchange: ${err instanceof Error ? err.message : 'unknown protocol error'}`, 'ORDER', 'ERROR');
      }
      return;
    }

    const ord = openOrders.find(o => o.id === id);
    if (!ord) return;

    // Refund locked cash/crypto indices
    const baseAsset = ord.symbol.split('/')[0];
    setBalances((prevBalances) => {
      const copy = [...prevBalances];
      const baseIdx = copy.findIndex(b => b.asset === baseAsset);
      const quoteAsset = ord.symbol.split('/')[1] || selectedMarketObj.quoteAsset;
      const quoteIdx = copy.findIndex(b => b.asset === quoteAsset);

      if (ord.side === 'BUY') {
        if (quoteIdx !== -1) {
          copy[quoteIdx].free += ord.total;
          copy[quoteIdx].locked = Math.max(0, copy[quoteIdx].locked - ord.total);
        }
      } else {
        if (baseIdx !== -1) {
          copy[baseIdx].free += ord.amount;
          copy[baseIdx].locked = Math.max(0, copy[baseIdx].locked - ord.amount);
        }
      }
      return copy;
    });

    setOpenOrders(prev => prev.filter(o => o.id !== id));
    
    // Move cancelled item into historical orders
    const cancelledOrder: Order = { ...ord, status: 'CANCELLED', timestamp: new Date() };
    setOrderHistory(prev => [cancelledOrder, ...prev]);
    appendLog(`Cancelled active standing order ${id} successfully. Funds released.`, 'ORDER', 'WARNING');
  };

  // Cancel all pending orders in batch
  const handleCancelAllOrders = () => {
    if (openOrders.length === 0) return;
    openOrders.forEach(o => handleCancelOrder(o.id));
    appendLog('All outstanding spot limit orders cancelled successfully.', 'ORDER', 'WARNING');
  };

  const handleOIDCLogin = () => {
    window.location.assign(oidcLoginURL());
  };

  const handleShowLoginScreen = () => {
    startTransition(() => {
      setActiveView('LOGIN');
    });
  };

  const handleOIDCLogout = async () => {
    try {
      const result = await logoutOIDC();
      setAuthUser(null);
      if (result.logout_url) {
        window.location.assign(result.logout_url);
        return;
      }
      await refreshAuth();
      appendLog('OIDC session closed. Operator identity released.', 'SYSTEM', 'WARNING');
    } catch (err) {
      appendLog(`OIDC logout failed: ${err instanceof Error ? err.message : 'unknown auth error'}`, 'SYSTEM', 'ERROR');
    }
  };

  // Edit Tab strategy code
  const handleUpdateStrategyCode = (id: string, code: string) => {
    setStrategies(prev => prev.map(s => s.id === id ? { ...s, code } : s));
  };

  // Start or pause automated strategies bot
  const handleToggleStrategy = (id: string) => {
    setStrategies(prev => prev.map(s => {
      if (s.id === id) {
        const nextStatus = s.status === 'RUNNING' ? 'IDLE' : 'RUNNING';
        appendLog(`Strategy bot state updated: '${s.name}' enters ${nextStatus}.`, 'STRATEGY', 'INFO');
        return { ...s, status: nextStatus };
      }
      return s;
    }));
  };

  // Handle deposit funds
  const handleDeposit = async (asset: string, amount: number) => {
    if (exchangeMode === 'live') {
      try {
        const settled = await settleExchangeDeposit(activeUserID, asset, amount);
        setBalances(prev => upsertBalance(prev, settled));
        setProtocolRevision(rev => rev + 1);
        const txId = `D-${Math.floor(10000 + Math.random() * 90000)}`;
        setWalletTransactions(prev => [{ id: txId, type: 'DEPOSIT', asset, amount, time: new Date() }, ...prev]);
        appendLog(`Gateway deposit settled on backend. +${amount} ${asset} credited to ${activeUserID}.`, 'SYSTEM', 'SUCCESS');
      } catch (err) {
        appendLog(`Gateway deposit rejected: ${err instanceof Error ? err.message : 'unknown settlement error'}`, 'SYSTEM', 'ERROR');
      }
      return;
    }

    setBalances(prev => prev.map(b => {
      if (b.asset === asset) {
        const nextFree = b.free + amount;
        return {
          ...b,
          free: nextFree,
          valueUsd: asset === 'USD' || asset === 'USDT' || asset === 'USDC' ? nextFree : nextFree * (markets.find(m => m.symbol.startsWith(asset))?.lastPrice || 1)
        };
      }
      return b;
    }));

    const txId = `D-${Math.floor(10000 + Math.random() * 90000)}`;
    setWalletTransactions(prev => [{ id: txId, type: 'DEPOSIT', asset, amount, time: new Date() }, ...prev]);
    appendLog(`Inward settlement completed. Settled +${amount} ${asset} securely.`, 'SYSTEM', 'SUCCESS');
  };

  // Handle withdrawals
  const handleWithdraw = (asset: string, amount: number) => {
    setBalances(prev => prev.map(b => {
      if (b.asset === asset) {
        const nextFree = b.free - amount;
        return {
          ...b,
          free: nextFree,
          valueUsd: asset === 'USD' || asset === 'USDT' || asset === 'USDC' ? nextFree : nextFree * (markets.find(m => m.symbol.startsWith(asset))?.lastPrice || 1)
        };
      }
      return b;
    }));

    const txId = `W-${Math.floor(10000 + Math.random() * 90000)}`;
    setWalletTransactions(prev => [{ id: txId, type: 'WITHDRAW', asset, amount, time: new Date() }, ...prev]);
    appendLog(`Outward hardware Ledger withdrawal published. Settled -${amount} ${asset} securely.`, 'SYSTEM', 'WARNING');
  };

  // Purge dbs settings reset
  const handlePurgeDbs = () => {
    setOpenOrders([]);
    setOrderHistory([]);
    setBalances(INITIAL_BALANCES);
    setSystemLogs([
      { id: 'LOG-RESET', timestamp: new Date(), type: 'SUCCESS', source: 'SYSTEM', message: 'Workspace cleared. System cache state rebuilt.' }
    ]);
  };

  // Workspace tab controllers
  const handleSelectTab = (tabId: string) => {
    const tabObj = openTabs.find(t => t.id === tabId);
    if (tabObj) {
      setActiveTabId(tabId);
      if (tabObj.symbol) {
        setSelectedPairSymbol(tabObj.symbol);
        setActiveView('TRADE');
      } else if (tabObj.type === 'PORTFOLIO') {
        setActiveView('PORTFOLIO');
      } else if (tabObj.type === 'STRATEGY_LAB') {
        setActiveView('STRATEGY_LAB');
      }
    }
  };

  const handleCloseTab = (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    if (openTabs.length === 1) return; // keep at least one tab open
    
    const remainingTabs = openTabs.filter(t => t.id !== id);
    setOpenTabs(remainingTabs);
    
    if (activeTabId === id) {
      const nextActive = remainingTabs[remainingTabs.length - 1];
      setActiveTabId(nextActive.id);
      if (nextActive.symbol) {
        setSelectedPairSymbol(nextActive.symbol);
      }
    }
  };

  const handleSelectPair = (symbol: string) => {
    setSelectedPairSymbol(symbol);
    
    // Check if symbol already exists as tabs, if not, append editor file tab
    const alreadyOpen = openTabs.find(t => t.id === symbol);
    if (!alreadyOpen) {
      const nextTabs = [...openTabs];
      // Place first or replace
      nextTabs.unshift({ id: symbol, title: symbol, type: 'MARKET', symbol });
      setOpenTabs(nextTabs);
    }
    
    setActiveTabId(symbol);
    setActiveView('TRADE');
  };

  const triggerRescanTickers = () => {
    setLatency(5);
    setMarkets(prev => prev.map(m => ({ ...m, lastPrice: m.lastPrice * (0.999 + Math.random() * 0.002) })));
    appendLog('Rescan event published. Polled high-speed validator nodes.', 'WEBSOCKET', 'SUCCESS');
  };

  // Preset Commands for Ctrl+K palette actions
  const primaryMarkets = markets.slice(0, 3);
  const commandPaletteActions = [
    {
      id: 'open-market-1',
      category: 'MARKET ASSETS',
      title: `Open ${primaryMarkets[0]?.symbol || 'Market'} Ticker`,
      subtitle: 'Launches historical candle charts and limit order screens.',
      icon: Cpu,
      action: () => primaryMarkets[0] && handleSelectPair(primaryMarkets[0].symbol),
    },
    {
      id: 'open-market-2',
      category: 'MARKET ASSETS',
      title: `Open ${primaryMarkets[1]?.symbol || 'Market'} Ticker`,
      subtitle: 'Focuses workspace on the next supported spot market.',
      icon: Cpu,
      action: () => primaryMarkets[1] && handleSelectPair(primaryMarkets[1].symbol),
    },
    {
      id: 'open-market-3',
      category: 'MARKET ASSETS',
      title: `Open ${primaryMarkets[2]?.symbol || 'Market'} Ticker`,
      subtitle: 'Accesses another supported spot terminal.',
      icon: Cpu,
      action: () => primaryMarkets[2] && handleSelectPair(primaryMarkets[2].symbol),
    },
    {
      id: 'p-orders',
      category: 'NAVIGATION',
      title: 'Show Open Orders Ledger',
      subtitle: 'Access bottom drawer detailing active standing entries.',
      icon: Coins,
      action: () => {
        setActiveView('ORDERS');
        setIsSidebarOpen(false);
      },
    },
    {
      id: 'p-portfolio',
      category: 'NAVIGATION',
      title: 'Show Portfolio holding allocations',
      subtitle: 'Inspect aggregate asset allocation values and cash reserves.',
      icon: Heart,
      action: () => {
        setActiveView('PORTFOLIO');
        // also focus portfolio tab
        const alreadyOpen = openTabs.find(t => t.type === 'PORTFOLIO');
        if (alreadyOpen) setActiveTabId(alreadyOpen.id);
      },
    },
    {
      id: 'p-strategy',
      category: 'NAVIGATION',
      title: 'Show Strategy Lab coding sandbox',
      subtitle: 'Backtest automated EMA crossover strategy algorithms.',
      icon: Code,
      action: () => {
        setActiveView('STRATEGY_LAB');
        const alreadyOpen = openTabs.find(t => t.type === 'STRATEGY_LAB');
        if (alreadyOpen) setActiveTabId(alreadyOpen.id);
      },
    },
    {
      id: 'act-deposit',
      category: 'ACCOUNT SEEDING',
      title: `Inject 10,000 ${selectedMarketObj.quoteAsset} cash capital`,
      subtitle: 'Boost available cash liquidity in mock workspace account.',
      icon: Wallet,
      action: () => handleDeposit(selectedMarketObj.quoteAsset, 10000),
    },
    {
      id: 'act-cancel-all',
      category: 'DANGEROUS CORE OPERATIONS',
      title: 'Cancel allstanding spot limit orders',
      subtitle: 'Instantly clear and recover all locked margin bookings.',
      icon: Trash2,
      action: () => handleCancelAllOrders(),
    },
    {
      id: 'act-toggle-theme',
      category: 'CANVAS PREFERENCES',
      title: 'Switch Color Canvas Theme',
      subtitle: 'Toggles between carbon-dark and pro-light layouts.',
      icon: Sun,
      action: () => setTheme(prev => prev === 'light' ? 'dark' : 'light'),
    },
    {
      id: authUser ? 'act-logout' : 'act-login',
      category: 'IDENTITY',
      title: authUser ? 'Close OIDC Session' : 'Start OIDC Login',
      subtitle: authUser ? 'Clears the secure exchange session cookie.' : `Open the ${authProvider} login screen.`,
      icon: User,
      action: () => authUser ? handleOIDCLogout() : handleShowLoginScreen(),
    },
  ];

  return (
    <div className={`w-full h-full flex flex-col overflow-hidden text-sm relative transition-colors duration-200 ${
      theme === 'light' ? 'bg-[#f6f8fa] text-gray-800' : 'bg-[var(--app-bg)] text-[var(--app-fg)]'
    } ${density === 'compact' ? 'layout-dense' : 'layout-cozy'}`}>

      {/* App Frame Panel (Top Navbar, Vertical activity, Sidebar, Main tabs Workspace, base Status bar) */}
      <div className="flex-1 flex overflow-hidden w-full">
        
        {/* VIEW COLUMN 1: LEFT VERTICAL ACTIVITY BAR (VS Code Style) */}
        <VerticalActivityBar
          activeView={activeView}
          setActiveView={(v) => {
            startTransition(() => {
              setActiveView(v);
              if (v === 'PORTFOLIO') {
                setActiveTabId('PORTFOLIO');
              } else if (v === 'STRATEGY_LAB') {
                setActiveTabId('STRATEGY_LAB');
              } else if (v === 'TRADE' && !openTabs.some(t => t.symbol)) {
                // ensure at least one market tab is selected if user goes to trade
                handleSelectPair(markets[0]?.symbol || selectedPairSymbol);
              }
            });
          }}
          openOrdersCount={openOrders.length}
          connectionStatus={connectionStatus}
          latency={latency}
          triggerRefresh={triggerRescanTickers}
          isSidebarOpen={isSidebarOpen}
          setIsSidebarOpen={setIsSidebarOpen}
          isAuthenticated={Boolean(authUser)}
          authEnabled={authEnabled}
          authLoading={authLoading}
          accountLabel={authUser?.email || authUser?.name || authUser?.sub}
          onLogin={handleShowLoginScreen}
          onLogout={handleOIDCLogout}
          onAuthRetry={refreshAuth}
        />

        {/* VIEW COLUMN 2: COLLAPSIBLE SIDEBAR MARKETS TREE PLOTTER */}
        <CollapsibleSidebar
          markets={markets}
          selectedPair={selectedPairSymbol}
          onSelectPair={handleSelectPair}
          onToggleFavorite={(symbol) => {
            setMarkets(prev => prev.map(m => m.symbol === symbol ? { ...m, isFavorite: !m.isFavorite } : m));
            appendLog(`Favorite state updated for pair ${symbol}.`, 'SYSTEM', 'INFO');
          }}
          isSidebarOpen={isSidebarOpen}
          assetMetadata={assetMetadata}
        />

        {/* VIEW COLUMN 3: MAIN WORKSPACE PANEL AREA WITH VS-CODE TABS */}
        <div className="flex-1 flex flex-col overflow-hidden min-w-0">
          
          {/* Editor-like tabs header list */}
          <div className="flex items-center justify-between border-b border-[#e1e4e8] dark:border-[#21262d] bg-[#f6f8fa] dark:bg-[#0d1117] overflow-x-auto scrollbar-hide py-1.5 px-3 min-h-[40px]">
            <div className="flex gap-1 overflow-x-auto scrollbar-hide">
              {openTabs.map((tab) => {
                const isActive = activeTabId === tab.id;
                
                // Render registry-backed asset tokens or file descriptors next to names
                const renderTabIcon = () => {
                  if (tab.type === 'MARKET' && tab.symbol) {
                    const market = markets.find(item => item.symbol === tab.symbol);
                    const baseAsset = market?.baseAsset || tab.symbol.split('/')[0] || tab.symbol;
                    return (
                      <AssetIcon symbol={baseAsset} iconURL={assetMetadata[baseAsset]?.icon_url} size="xs" />
                    );
                  } else if (tab.type === 'PORTFOLIO') {
                    return <Briefcase className="w-3.5 h-3.5 text-blue-500 shrink-0" />;
                  } else if (tab.type === 'STRATEGY_LAB') {
                    return <Code className="w-3.5 h-3.5 text-emerald-500 shrink-0" />;
                  }
                  return null;
                };

                return (
                  <button
                    key={tab.id}
                    onClick={() => handleSelectTab(tab.id)}
                    className={`flex items-center gap-1.5 px-2.5 py-1 text-xs font-mono font-semibold rounded border transition-all whitespace-nowrap cursor-pointer select-none ${
                      isActive
                        ? 'bg-white dark:bg-[#0c1015] border-[#e1e4e8] dark:border-[#21262d] text-accent-1 shadow-xs font-bold ring-1 ring-accent-1/10'
                        : 'border-transparent text-gray-500 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-surface-3'
                    }`}
                  >
                    {renderTabIcon()}
                    <span>{tab.title}</span>
                    <span
                      onClick={(e) => handleCloseTab(tab.id, e)}
                      className="w-4 h-4 rounded-full flex items-center justify-center ml-1 bg-pink-50/80 text-pink-400 border border-pink-200/80 hover:bg-pink-100 hover:text-pink-500 hover:border-pink-300 dark:bg-pink-400/10 dark:text-pink-300 dark:border-pink-300/20 dark:hover:bg-pink-400/15 dark:hover:text-pink-200 dark:hover:border-pink-300/35 shadow-[0_1px_2px_rgba(244,114,182,0.14)] hover:shadow-[0_2px_6px_rgba(244,114,182,0.2)] hover:scale-105 active:scale-95 transition-all cursor-pointer select-none"
                      title="Close Tab"
                    >
                      <X className="w-2.5 h-2.5 stroke-[2.25]" aria-hidden="true" />
                    </span>
                  </button>
                );
              })}
            </div>

          </div>

          {/* ACTIVE WORKSPACE RENDER PANEL SWITCH */}
          {isPending ? (
            <div className="flex-1 flex flex-col items-center justify-center text-gray-400 font-mono text-xs">
              <Cpu className="w-8 h-8 animate-spin mb-2 text-accent-1" />
              Allocating workspace memory...
            </div>
          ) : activeView === 'LOGIN' ? (
            <LoginScreen
              provider={authProvider}
              isOIDCEnabled={authEnabled}
              isLoading={authLoading}
              error={authError}
              onLogin={handleOIDCLogin}
              onRetry={refreshAuth}
              onContinueSandbox={() => setActiveView('TRADE')}
            />
          ) : activeView === 'PORTFOLIO' || activeView === 'WALLET' ? (
            /* PORTFOLIO VIEW */
            <PortfolioView
              balances={balances}
              onDeposit={handleDeposit}
              onWithdraw={handleWithdraw}
              transactions={walletTransactions}
            />
          ) : activeView === 'STRATEGY_LAB' ? (
            /* STRATEGY DEVELOPER LAB */
            <StrategyLab
              strategies={strategies}
              markets={markets}
              onToggleStrategy={handleToggleStrategy}
              onUpdateStrategyCode={handleUpdateStrategyCode}
              onAddSystemLog={appendLog}
            />
          ) : activeView === 'SETTINGS' ? (
            /* CONFIGURATION SETTINGS PANEL */
            <SettingsView
              theme={theme}
              setTheme={setTheme}
              density={density}
              setDensity={setDensity}
              confirmOrders={confirmOrders}
              setConfirmOrders={setConfirmOrders}
              soundEnabled={soundEnabled}
              setSoundEnabled={setSoundEnabled}
              onPurgeDbs={handlePurgeDbs}
              userEmail={authUser?.email || authUser?.name || authUser?.sub || 'Local Sandbox'}
            />
          ) : activeView === 'MARKETS' ? (
            /* BULK MARKETS LISTING SCREEN */
            <div className="flex-1 p-5 overflow-y-auto space-y-4 max-w-7xl mx-auto w-full select-none">
              <div className="flex justify-between items-center border-b border-[#e1e4e8] dark:border-[#21262d] pb-3">
                <h2 className="text-sm font-bold uppercase tracking-wider font-display text-gray-900 dark:text-gray-100">
                  Bulk Markets Liquidity Indices
                </h2>
                <div className="text-xs font-mono text-gray-400">Europe Gateway v3</div>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {markets.map((m) => {
                  const isUp = m.change24h >= 0;
                  return (
                    <div
                      key={m.symbol}
                      onClick={() => handleSelectPair(m.symbol)}
                      className="p-4 bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg cursor-pointer hover:border-accent-1 transition-all flex flex-col justify-between group h-32"
                    >
                      <div className="flex justify-between items-center mb-2">
                        <span className="font-display font-medium text-xs text-gray-800 dark:text-gray-200 uppercase tracking-widest flex items-center gap-2 min-w-0">
                          <AssetIcon symbol={m.baseAsset} iconURL={assetMetadata[m.baseAsset]?.icon_url} size="sm" />
                          <span className="truncate">{m.symbol}</span>
                        </span>
                        <span className={`text-[10px] px-1.5 py-0.5 rounded font-mono font-bold ${isUp ? 'text-trade-green bg-trade-green-bg' : 'text-trade-red bg-trade-red-bg'}`}>
                          {isUp ? '+' : ''}{m.change24h.toFixed(2)}%
                        </span>
                      </div>
                      <div className="font-mono text-xl font-bold tracking-tight text-gray-950 dark:text-gray-100 mb-2">
                        ${formatPrice(m.lastPrice)}
                      </div>
                      <div className="text-[9px] font-mono text-gray-400 flex justify-between border-t border-gray-100 dark:border-gray-800/60 pt-2">
                        <span>24h Vol: {m.volume24h.toLocaleString(undefined, { maximumFractionDigits: 0 })} {m.baseAsset}</span>
                        <span className="text-accent-1">Liq: ${m.liquidity.toFixed(1)}M</span>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          ) : activeView === 'ORDERS' ? (
            /* DETAILED ACCOUNTS ORDERS SCREEN */
            <div className="flex-1 p-5 overflow-y-auto space-y-4 max-w-7xl mx-auto w-full select-none">
              <div className="flex justify-between items-center border-b border-[#e1e4e8] dark:border-[#21262d] pb-3">
                <h2 className="text-sm font-bold uppercase tracking-wider font-display text-gray-900 dark:text-gray-100 flex items-center gap-2">
                  <Coins className="w-5 h-5 text-accent-1" />
                  Consolidated Accounts Ledger Orders
                </h2>
                {openOrders.length > 0 && (
                  <button
                    onClick={handleCancelAllOrders}
                    className="px-3 py-1 bg-rose-500 hover:bg-rose-600 text-white text-[10px] font-mono font-bold rounded cursor-pointer transition-colors shadow-sm"
                  >
                    Cancel all standing limits ({openOrders.length})
                  </button>
                )}
              </div>

              {/* Embed terminal components direct */}
              <TerminalPanel
                openOrders={openOrders}
                orderHistory={orderHistory}
                tradeHistory={tradeHistory}
                systemLogs={systemLogs}
                selectedAssetSymbol={selectedAssetSymbol}
                dexPrices={dexPrices}
                dexPricesLoading={dexPricesLoading}
                dexPricesError={dexPricesError}
                assetMetadata={assetMetadata}
                onCancelOrder={handleCancelOrder}
                onCancelAllOrders={handleCancelAllOrders}
              />
            </div>
          ) : (
            /* PRIMARY CORE WORKSTATION: TRADE DESK LAYOUT */
            <div className="flex-1 p-2 sm:p-3 overflow-y-auto grid grid-cols-1 lg:grid-cols-24 gap-3 bg-[#fafbfc] dark:bg-[#070b0f]">
              
              {/* Workspace Left Column: Core charts & base Terminal outputs drawers (col-span-16) */}
              <div className="lg:col-span-16 flex flex-col gap-3 min-w-0">
                {/* 1. Charts Canvas */}
                <MarketChart
                  pair={selectedMarketObj}
                  candles={candles}
                  timeframe={timeframe}
                  setTimeframe={setTimeframe}
                  assetMetadata={assetMetadata}
                />

                {/* 2. Base Terminal panel drawer */}
                <TerminalPanel
                  openOrders={openOrders}
                  orderHistory={orderHistory}
                  tradeHistory={tradeHistory}
                  systemLogs={systemLogs}
                  selectedAssetSymbol={selectedAssetSymbol}
                  dexPrices={dexPrices}
                  dexPricesLoading={dexPricesLoading}
                  dexPricesError={dexPricesError}
                  assetMetadata={assetMetadata}
                  onCancelOrder={handleCancelOrder}
                  onCancelAllOrders={handleCancelAllOrders}
                />
              </div>

              {/* Workspace Right Column: Order books, execution feed stream, and limits selectors (col-span-8) */}
              <div className="lg:col-span-8 flex flex-col gap-3">
                
                {/* 1. Inline Tabs containing Bid/Ask Book vs Executed trades feed */}
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3 shrink-0">
                  <OrderBookView
                    orderBook={activeOrderBook}
                    pair={selectedMarketObj}
                    onSelectPrice={(pr) => setOrderBookSelectedPrice(pr)}
                  />
                  <RecentTrades
                    trades={tradeHistory}
                    pair={selectedMarketObj}
                  />
                </div>

                {/* 2. Multi-forms Buy/Sell execution limits */}
                <div className="flex-1 min-h-[360px]">
                  <OrderForm
                    pair={selectedMarketObj}
                    availableUsdt={balances.find(b => b.asset === selectedMarketObj.quoteAsset)?.free || 0}
                    availableBase={balances.find(b => b.asset === selectedMarketObj.baseAsset)?.free || 0}
                    onSubmitOrder={handleOrderSubmit}
                    selectedPrice={orderBookSelectedPrice}
                    clearSelectedPrice={() => setOrderBookSelectedPrice(null)}
                    submitError={orderSubmitError}
                  />
                </div>

              </div>
            </div>
          )}

        </div>

      </div>

      {/* CORE BASE VIEW COLUMN 4: FOOTER STATUS BAR (IDE inspired, high-density) */}
      <footer className="h-6 sm:h-7 border-t border-[#e1e4e8] dark:border-[#21262d] bg-[#f6f8fa] dark:bg-[#0d1117] px-3 flex items-center justify-between font-mono text-[10px] text-gray-500 shrink-0 select-none">
        
        {/* Sync metrics left and commands link */}
        <div className="flex items-center gap-4.5">
          <span className="flex items-center gap-1.5 font-bold text-accent-1">
            <span className={`w-2 h-2 rounded-full inline-block ${
              exchangeMode === 'live' ? 'bg-trade-green' : exchangeMode === 'connecting' ? 'bg-[#f59e0b]' : 'bg-trade-red'
            }`}></span>
            {BRAND_NAME} LIMIT PROTOCOL
          </span>

          <span className="text-[#7e8c9a] hidden sm:inline">
            Exchange: {exchangeMode === 'live' ? 'LIVE REST/WS' : exchangeMode === 'connecting' ? 'CONNECTING' : 'LOCAL FALLBACK'}
          </span>

          <span className="hidden md:inline">
            Fee rate: <span className="font-semibold text-gray-700 dark:text-gray-300">Maker 0.08% / Taker 0.10%</span>
          </span>
        </div>

        {/* Sync triggers right */}
        <div className="flex items-center gap-4.5">
          <span className="text-[#ffe05c]" title="Spot mode operates without liabilities">
            Account: {authUser?.email || authUser?.name || 'Spot'}
          </span>

          {authUser ? (
            <button
              type="button"
              onClick={handleOIDCLogout}
              className="text-[#f6465d] hover:text-[#ff6f80] hidden sm:inline cursor-pointer"
            >
              Logout
            </button>
          ) : (
            <button
              type="button"
              onClick={authEnabled ? handleShowLoginScreen : refreshAuth}
              disabled={authLoading}
              title={authError || (authEnabled ? `Open ${authProvider} login screen` : 'OIDC status unavailable')}
              className="text-accent-1 hover:text-accent-1-hovered disabled:text-gray-400 hidden sm:inline cursor-pointer disabled:cursor-not-allowed"
            >
              {authLoading ? 'Auth...' : authEnabled ? 'Login' : 'Retry Auth'}
            </button>
          )}

          <span className="text-[#3b82f6] hidden sm:inline">
            Latency API: {latency}ms
          </span>

          <span className="text-[#f59e0b] hidden sm:inline" title={exchangeMessage}>
            Protocol rev: {protocolRevision}
          </span>

          <span className="text-[#7e8c9a] border-l border-gray-200 dark:border-gray-800 pl-3">
            UTC: {new Date().toLocaleTimeString(undefined, {hour12: false})}
          </span>
        </div>

      </footer>

      {/* COMMAND PALETTE POPUP */}
      <CommandPalette
        isOpen={isCommandPaletteOpen}
        onClose={() => setIsCommandPaletteOpen(false)}
        actions={commandPaletteActions}
      />

    </div>
  );
}

function mergeMarkets(current: MarketPair[], remote: MarketPair[]): MarketPair[] {
  return remote.map((market) => {
    const existing = current.find(item => item.symbol === market.symbol);
    if (!existing) return market;
    return {
      ...market,
      isFavorite: existing.isFavorite ?? market.isFavorite,
    };
  });
}

function replaceMarketTabs(current: Tab[], markets: MarketPair[]): Tab[] {
  const marketTabs = markets.slice(0, 3).map((market) => ({
    id: market.symbol,
    title: market.symbol,
    type: 'MARKET' as const,
    symbol: market.symbol,
  }));
  const utilityTabs = current.filter(tab => tab.type !== 'MARKET');
  return [...marketTabs, ...utilityTabs];
}

function emptyOrderBook(): OrderBook {
  return {
    bids: [],
    asks: [],
    spread: 0,
    spreadPercent: 0,
  };
}

function upsertBalance(current: AssetBalance[], next: AssetBalance): AssetBalance[] {
  const found = current.some(item => item.asset === next.asset);
  if (!found) return [next, ...current];
  return current.map(item => item.asset === next.asset ? next : item);
}

function assetMetadataBySymbol(assets: AssetInfo[]): Record<string, AssetInfo> {
  const out: Record<string, AssetInfo> = {};
  assets.forEach((asset) => {
    const symbol = asset.symbol?.toUpperCase();
    if (symbol) {
      out[symbol] = asset;
    }
    (asset.deployments || []).forEach((deployment) => {
      const deploymentSymbol = deployment.symbol?.toUpperCase();
      if (deploymentSymbol) {
        out[deploymentSymbol] = {
          ...asset,
          symbol: deployment.symbol,
          name: deployment.name || asset.name,
          decimals: deployment.decimals ?? asset.decimals,
          icon_url: deployment.icon_url || asset.icon_url,
        };
      }
    });
  });
  return out;
}

function mergeAssetPrices(current: AssetPriceResponse | null, incoming: AssetPriceResponse): AssetPriceResponse {
  if (!current || current.symbol.toUpperCase() !== incoming.symbol.toUpperCase()) {
    return incoming;
  }

  const byPool = new Map<string, AssetPriceResponse['prices'][number]>();
  current.prices.forEach((price) => byPool.set(priceKey(price), price));
  incoming.prices.forEach((price) => byPool.set(priceKey(price), price));

  return {
    ...current,
    ...incoming,
    asset: incoming.asset || current.asset,
    prices: Array.from(byPool.values()).sort(comparePoolPrices),
  };
}

function priceKey(price: AssetPriceResponse['prices'][number]): string {
  return `${price.chain_key}:${price.venue_key}:${price.pool_id}`;
}

function comparePoolPrices(a: AssetPriceResponse['prices'][number], b: AssetPriceResponse['prices'][number]): number {
  const chainOrder = ['chiliz', 'base', 'solana', 'ethereum', 'avalanche'];
  const chainDelta = chainRank(a.chain_key, chainOrder) - chainRank(b.chain_key, chainOrder);
  if (chainDelta !== 0) return chainDelta;
  const venueDelta = a.venue_key.localeCompare(b.venue_key);
  if (venueDelta !== 0) return venueDelta;
  return a.pool_id.localeCompare(b.pool_id);
}

function chainRank(chain: string, order: string[]): number {
  const index = order.indexOf(chain);
  return index === -1 ? order.length : index;
}
