/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import React, { useState, useEffect, useTransition, useCallback, useMemo, useRef } from 'react';
import { Sparkles, Cpu, Coins, ShieldCheck, Heart, User, Sun, Moon, CheckSquare, Layers, Wallet, Trash2, Briefcase, X } from 'lucide-react';
import VerticalActivityBar from './components/VerticalActivityBar';
import CollapsibleSidebar from './components/CollapsibleSidebar';
import MarketChart from './components/MarketChart';
import OrderBookView, { type OrderBookSelection } from './components/OrderBook';
import OrderForm from './components/OrderForm';
import TerminalPanel from './components/TerminalPanel';
import CommandPalette from './components/CommandPalette';
import PortfolioView from './components/PortfolioView';
import SettingsView from './components/SettingsModal';
import LoginScreen from './components/LoginScreen';
import AssetIcon from './components/AssetIcon';
import { BRAND_DOCUMENT_TITLE, BRAND_NAME } from './constants/brand';

import {
  MarketPair,
  Candle,
  Timeframe,
  Order,
  Trade,
  SystemLog,
  AssetBalance,
  OrderType,
  OrderSide,
  OrderBook,
} from './types/trading';
import {
  type DepositAddressInfo,
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
  listMarkets,
  openExchangeSocket,
  openPriceSocket,
  placeOrder as placeExchangeOrder,
  requestDepositAddress as requestExchangeDepositAddress,
  requestWithdrawal as requestExchangeWithdrawal,
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
  type: 'MARKET' | 'PORTFOLIO' | 'CUSTOM_PAIR';
  symbol?: string;
}

type WalletTransaction = {
  id: string;
  type: string;
  asset: string;
  chainKey?: string;
  amount: number;
  time: Date;
};

type ExchangeMode = 'connecting' | 'live' | 'offline';
type ThemeMode = 'light' | 'dark';

type RouteSnapshot = {
  view: string;
  market: string;
};

const THEME_STORAGE_KEY = 'kewl.theme';
const ORDER_BOOK_DEPTH = 500;
const ACTIVE_ORDER_REMAINING_EPSILON = 0.000000005;

function initialTheme(): ThemeMode {
  if (typeof window === 'undefined') return 'dark';
  const stored = window.localStorage.getItem(THEME_STORAGE_KEY);
  if (stored === 'light' || stored === 'dark') return stored;
  return 'dark';
}

function sanitizeWorkspaceTabs(tabs: Tab[]): Tab[] {
  return tabs.filter(tab => (tab.type as string) !== 'STRATEGY_LAB');
}

export default function App() {
  const [isPending, startTransition] = useTransition();
  const exchangeModeRef = useRef<ExchangeMode>('connecting');
  const protocolRefreshTimerRef = useRef<number | null>(null);
  const orderBookRefreshTimerRef = useRef<number | null>(null);
  const orderBookCacheRef = useRef<Map<string, OrderBook>>(new Map());
  const initialRouteRef = useRef<RouteSnapshot>(readRouteSnapshot());
  const didSyncRouteRef = useRef(false);

  // Primary Workspace views: MARKETS, TRADE (docked terminal), PORTFOLIO, ORDERS, WALLET, SETTINGS
  const [activeView, setActiveView] = useState<string>(() => initialRouteRef.current.view);
  const [isSidebarOpen, setIsSidebarOpen] = useState(true);
  const [selectedPairSymbol, setSelectedPairSymbol] = useState(() => initialRouteRef.current.market);

  // Command palette toggle state
  const [isCommandPaletteOpen, setIsCommandPaletteOpen] = useState(false);

  // Layout preferences states
  const [theme, setTheme] = useState<ThemeMode>(() => initialTheme());
  const [density, setDensity] = useState<'compact' | 'comfortable'>('compact');
  const [confirmOrders, setConfirmOrders] = useState(true);
  const [soundEnabled, setSoundEnabled] = useState(true);

  // Core exchange data structures states
  const [markets, setMarkets] = useState<MarketPair[]>([]);
  const [balances, setBalances] = useState<AssetBalance[]>([]);
  const [balancesError, setBalancesError] = useState<string | null>(null);
  const [openOrders, setOpenOrders] = useState<Order[]>([]);
  const [orderHistory, setOrderHistory] = useState<Order[]>([]);
  
  // Historical executions
  const selectedMarketObj = markets.find(m => m.symbol === selectedPairSymbol) || null;
  const [tradeHistory, setTradeHistory] = useState<Trade[]>([]);
  const [recentTrades, setRecentTrades] = useState<Trade[]>([]);
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
  const canUseConfiguredUserID = !authLoading && !authUser;
  const activeUserID = authUser?.sub || (canUseConfiguredUserID ? exchangeConfig.userID : '');
  const accountLabel = authUser?.email || authUser?.name || authUser?.sub || (canUseConfiguredUserID ? `${exchangeConfig.userID} (dev)` : '');
  const selectedAssetSymbol = selectedMarketObj?.baseAsset || selectedPairSymbol.split('/')[0] || '';
  const displayedOrderBook = activeOrderBook.market === selectedPairSymbol
    ? activeOrderBook
    : emptyOrderBook(selectedPairSymbol);
  const [dexPrices, setDexPrices] = useState<AssetPriceResponse | null>(null);
  const [dexPricesLoading, setDexPricesLoading] = useState(false);
  const [dexPricesError, setDexPricesError] = useState<string | null>(null);
  const [assetMetadata, setAssetMetadata] = useState<Record<string, AssetInfo>>({});

  // Visual terminal logs
  const [systemLogs, setSystemLogs] = useState<SystemLog[]>([]);

  // Wallet transaction ledger (Deposits/Withdrawals)
  const [walletTransactions, setWalletTransactions] = useState<WalletTransaction[]>([]);

  // Map of active candlesticks series for the loaded asset pair
  const [timeframe, setTimeframe] = useState<Timeframe>('15m');
  const [candles, setCandles] = useState<Candle[]>([]);

  // Selected pricing feedback from Order book click (to flow into Order Form)
  const [orderBookSelection, setOrderBookSelection] = useState<OrderBookSelection | null>(null);

  // Connection parameters
  const [connectionStatus, setConnectionStatus] = useState<'connected' | 'reconnecting' | 'disconnected'>('connected');
  const [latency, setLatency] = useState(0);

  // Active Workspace Open editor tabs
  const [openTabs, setOpenTabs] = useState<Tab[]>(() => initialTabsForRoute(initialRouteRef.current));
  const [activeTabId, setActiveTabId] = useState<string>(() => tabIdForRoute(initialRouteRef.current));

  useEffect(() => {
    setOpenTabs(prev => {
      const next = sanitizeWorkspaceTabs(prev);
      if (next.length === prev.length) return prev;
      return next.length > 0 ? next : initialTabsForRoute(initialRouteRef.current);
    });
    if (activeTabId === 'STRATEGY_LAB') {
      setActiveTabId('PORTFOLIO');
      setActiveView('PORTFOLIO');
    }
  }, [activeTabId]);

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

  const scheduleProtocolRefresh = useCallback((delayMs = 600): boolean => {
    if (protocolRefreshTimerRef.current !== null) return false;
    protocolRefreshTimerRef.current = window.setTimeout(() => {
      protocolRefreshTimerRef.current = null;
      setProtocolRevision(rev => rev + 1);
    }, delayMs);
    return true;
  }, []);

  const refreshOrderBookSnapshot = useCallback((marketSymbol: string, delayMs = 80): boolean => {
    const targetMarket = marketSymbol || selectedPairSymbol;
    if (!targetMarket || targetMarket !== selectedPairSymbol) return false;
    if (orderBookRefreshTimerRef.current !== null) return false;

    orderBookRefreshTimerRef.current = window.setTimeout(async () => {
      orderBookRefreshTimerRef.current = null;
      try {
        const nextOrderBook = await fetchOrderBook(targetMarket, ORDER_BOOK_DEPTH);
        if (nextOrderBook.market === targetMarket) {
          orderBookCacheRef.current.set(targetMarket, nextOrderBook);
          setActiveOrderBook(nextOrderBook);
        }
      } catch (err) {
        appendLog(err instanceof Error ? err.message : 'Order book refresh failed', 'WEBSOCKET', 'WARNING');
      }
    }, delayMs);
    return true;
  }, [appendLog, selectedPairSymbol]);

  const resetMarketData = useCallback((symbol: string) => {
    setActiveOrderBook(orderBookCacheRef.current.get(symbol) ?? emptyOrderBook(symbol));
    setCandles([]);
    setRecentTrades([]);
    setTradeHistory([]);
    setOpenOrders([]);
    setOrderHistory([]);
    setOrderBookSelection(null);
  }, []);

  useEffect(() => {
    const targetMarket = selectedPairSymbol;
    if (!targetMarket) {
      setActiveOrderBook(emptyOrderBook());
      return;
    }

    if (orderBookRefreshTimerRef.current !== null) {
      window.clearTimeout(orderBookRefreshTimerRef.current);
      orderBookRefreshTimerRef.current = null;
    }

    let cancelled = false;
    setActiveOrderBook(orderBookCacheRef.current.get(targetMarket) ?? emptyOrderBook(targetMarket));

    fetchOrderBook(targetMarket, ORDER_BOOK_DEPTH)
      .then((nextOrderBook) => {
        if (cancelled || nextOrderBook.market !== targetMarket) return;
        orderBookCacheRef.current.set(targetMarket, nextOrderBook);
        setActiveOrderBook(nextOrderBook);
      })
      .catch((err) => {
        if (cancelled) return;
        appendLog(err instanceof Error ? err.message : 'Order book snapshot failed', 'WEBSOCKET', 'WARNING');
      });

    return () => {
      cancelled = true;
    };
  }, [appendLog, selectedPairSymbol]);

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

  useEffect(() => {
    if (authLoading || !authUser || activeView !== 'LOGIN') return;

    startTransition(() => {
      setActiveView('TRADE');
      setIsSidebarOpen(true);
    });
  }, [activeView, authLoading, authUser, startTransition]);

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
    const applyBrowserRoute = () => {
      const snapshot = readRouteSnapshot();
      setActiveView(snapshot.view);
      if (snapshot.market) {
        resetMarketData(snapshot.market);
      }
      setSelectedPairSymbol(snapshot.market);
      setActiveTabId(tabIdForRoute(snapshot));
      setOpenTabs(prev => ensureRouteTab(prev, snapshot));
      if (snapshot.view === 'TRADE') {
        setIsSidebarOpen(true);
      }
    };

    window.addEventListener('popstate', applyBrowserRoute);
    return () => window.removeEventListener('popstate', applyBrowserRoute);
  }, [resetMarketData]);

  useEffect(() => {
    const nextPath = routePathForState(activeView, selectedPairSymbol);
    const currentPath = `${window.location.pathname}${window.location.search}${window.location.hash}`;
    if (currentPath === nextPath) {
      didSyncRouteRef.current = true;
      return;
    }

    if (didSyncRouteRef.current) {
      window.history.pushState(null, '', nextPath);
    } else {
      window.history.replaceState(null, '', nextPath);
      didSyncRouteRef.current = true;
    }
  }, [activeView, selectedPairSymbol]);

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

  useEffect(() => {
    let cancelled = false;

    const refreshExchangeSnapshot = async () => {
      setDexPricesLoading(true);
      setDexPricesError(null);
      try {
        const remoteMarkets = await listMarkets();
        if (cancelled) return;

        if (remoteMarkets.length === 0) {
          setMarkets([]);
          setSelectedPairSymbol('');
          setActiveOrderBook(emptyOrderBook());
          setCandles([]);
          setTradeHistory([]);
          setOpenOrders([]);
          setOrderHistory([]);
          setBalances([]);
          setBalancesError(null);
          setConnectionStatus('connected');
          setExchangeMode('live');
          setExchangeMessage(`REST/WS bound to ${exchangeConfig.apiBaseURL}; no markets returned`);
          exchangeModeRef.current = 'live';
          return;
        }

        const sortedMarkets = sortMarkets(remoteMarkets);
        setMarkets(prev => mergeMarkets(prev, sortedMarkets));

        let activeSymbol = selectedPairSymbol;
        if (!sortedMarkets.some(m => m.symbol === activeSymbol)) {
          activeSymbol = sortedMarkets[0].symbol;
          setSelectedPairSymbol(activeSymbol);
          setActiveTabId(activeSymbol);
          setOpenTabs(prev => replaceMarketTabs(prev, sortedMarkets));
        }
        const activeMarket = sortedMarkets.find(m => m.symbol === activeSymbol) || sortedMarkets[0];
        const activeAsset = activeMarket.baseAsset || activeSymbol.split('/')[0] || '';
        const userOrdersPromise = activeUserID ? fetchUserOrders(activeUserID, activeSymbol, 100) : Promise.resolve<Order[]>([]);
        const userTradesPromise = activeUserID ? fetchUserTrades(activeUserID, activeSymbol, 100) : Promise.resolve<Trade[]>([]);
        const balancesPromise = activeUserID ? fetchBalances(activeUserID) : Promise.resolve<AssetBalance[]>([]);

        const [
          orderBookResult,
          candlesResult,
          marketTradesResult,
          userOrdersResult,
          userTradesResult,
          balancesResult,
          assetPricesResult,
        ] = await Promise.allSettled([
          fetchOrderBook(activeSymbol, ORDER_BOOK_DEPTH),
          fetchCandles(activeSymbol, timeframe, 120),
          fetchMarketTrades(activeSymbol, 80),
          userOrdersPromise,
          userTradesPromise,
          balancesPromise,
          activeAsset ? fetchAssetPrices(activeAsset) : Promise.resolve(null),
        ]);

        if (cancelled) return;

        if (orderBookResult.status === 'fulfilled') {
          if (orderBookResult.value.market === activeSymbol) {
            orderBookCacheRef.current.set(activeSymbol, orderBookResult.value);
            setActiveOrderBook(orderBookResult.value);
          } else {
            setActiveOrderBook(emptyOrderBook(activeSymbol));
          }
        } else {
          setActiveOrderBook(emptyOrderBook(activeSymbol));
        }
        if (candlesResult.status === 'fulfilled') {
          const nextCandles = candlesResult.value;
          setCandles(nextCandles);
        } else {
          setCandles([]);
        }
        if (marketTradesResult.status === 'fulfilled' || userTradesResult.status === 'fulfilled') {
          const marketTrades = marketTradesResult.status === 'fulfilled' ? marketTradesResult.value : [];
          const userTrades = userTradesResult.status === 'fulfilled' ? userTradesResult.value : [];
          setRecentTrades(sortTradesDesc(marketTrades));
          setTradeHistory(sortTradesDesc(userTrades));
        } else {
          setRecentTrades([]);
          setTradeHistory([]);
        }
        if (userOrdersResult.status === 'fulfilled') {
          setOpenOrders(userOrdersResult.value.filter(isActiveOrder));
          setOrderHistory(userOrdersResult.value.filter(o => !isActiveOrder(o)));
        } else {
          setOpenOrders([]);
          setOrderHistory([]);
        }
        if (balancesResult.status === 'fulfilled') {
          setBalances(balancesResult.value);
          setBalancesError(activeUserID ? null : (authEnabled && !authLoading ? 'OIDC session is required for balances.' : null));
        } else {
          setBalances([]);
          setBalancesError(balancesResult.reason instanceof Error ? balancesResult.reason.message : 'Balances unavailable');
        }
        if (assetPricesResult.status === 'fulfilled' && assetPricesResult.value) {
          setDexPrices(assetPricesResult.value);
          setDexPricesError(null);
        } else {
          setDexPrices(null);
          setDexPricesError(assetPricesResult.status === 'rejected' && assetPricesResult.reason instanceof Error ? assetPricesResult.reason.message : 'DEX prices unavailable');
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
        setExchangeMode('offline');
        setExchangeMessage(err instanceof Error ? err.message : 'Exchange API unavailable');
        if (exchangeModeRef.current !== 'offline') {
          appendLog('Exchange service unavailable. Backend-only workspace is waiting for API data.', 'WEBSOCKET', 'WARNING');
        }
        exchangeModeRef.current = 'offline';
        setMarkets([]);
        setSelectedPairSymbol('');
        setActiveOrderBook(emptyOrderBook());
        setCandles([]);
        setRecentTrades([]);
        setTradeHistory([]);
        setOpenOrders([]);
        setOrderHistory([]);
        setBalances([]);
        setBalancesError(err instanceof Error ? err.message : 'Balances unavailable');
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

    let pingIntervalID: number | null = null;
    const sendLatencyProbe = (socket: WebSocket) => {
      if (socket.readyState !== WebSocket.OPEN) return;
      socket.send(JSON.stringify({
        type: 'exchange.ping',
        sent_at: performance.now(),
      }));
    };

    const socket = openExchangeSocket((event) => {
      const eventType = typeof event?.type === 'string' ? event.type : '';
      if (eventType === 'exchange.pong') {
        const sentAt = typeof event?.sent_at === 'number' ? event.sent_at : Number(event?.sent_at || 0);
        if (Number.isFinite(sentAt) && sentAt > 0) {
          setLatency(Math.max(1, Math.round(performance.now() - sentAt)));
        }
        return;
      }
      if (event?.market && event.market !== selectedPairSymbol) return;
      if (eventType === 'exchange.orderbook_delta') {
        const targetMarket = event.market || selectedPairSymbol;
        const currentBook = orderBookCacheRef.current.get(targetMarket) ?? emptyOrderBook(targetMarket);
        const nextOrderBook = applyOrderBookDelta(currentBook, event);
        orderBookCacheRef.current.set(targetMarket, nextOrderBook);
        if (targetMarket === selectedPairSymbol) {
          setActiveOrderBook(nextOrderBook);
        }
      } else if (eventType === 'exchange.orderbook_updated') {
        refreshOrderBookSnapshot(event.market || selectedPairSymbol, 220);
      } else if (eventType.startsWith('exchange.order_') || eventType === 'exchange.trades_created') {
        const socketOrder = orderFromSocketEvent(event);
        const socketTrades = tradesFromSocketEvent(event);

        if (socketOrder && socketEventBelongsToUser(event, activeUserID)) {
          if (isActiveOrder(socketOrder)) {
            setOpenOrders(prev => [socketOrder, ...prev.filter(order => order.id !== socketOrder.id)]);
            setOrderHistory(prev => prev.filter(order => order.id !== socketOrder.id));
          } else {
            setOpenOrders(prev => prev.filter(order => order.id !== socketOrder.id));
            setOrderHistory(prev => insertOrReplaceOrder(prev, socketOrder));
          }
        }
        if (socketTrades.length > 0) {
          setRecentTrades(prev => mergeTrades(socketTrades, prev));
          if (socketEventBelongsToUser(event, activeUserID)) {
            setTradeHistory(prev => mergeTrades(socketTrades, prev));
          }
          setMarkets(prev => applyMarketTrades(prev, socketTrades));
        }
        if (scheduleProtocolRefresh(900)) {
          appendLog(`Protocol event received: ${eventType}`, 'ORDER', 'INFO');
        }
      } else if (eventType === 'exchange.deposit_pending' || eventType === 'exchange.deposit_settled') {
        if (!socketEventBelongsToUser(event, activeUserID)) return;
        const balance = balanceFromSocketEvent(event);
        if (!balance) return;
        setBalances(prev => upsertBalance(prev, balance));
        setBalancesError(null);
        scheduleProtocolRefresh(400);
        appendLog(`Balance updated: ${balance.asset} ${eventType.replace('exchange.', '')}.`, 'WEBSOCKET', 'SUCCESS');
      } else if (eventType.startsWith('exchange.withdrawal_')) {
        if (!socketEventBelongsToUser(event, activeUserID)) return;
        scheduleProtocolRefresh(400);
        appendLog(`Withdrawal update received: ${eventType.replace('exchange.', '')}.`, 'WEBSOCKET', 'INFO');
      }
    });

    socket.onopen = () => {
      setConnectionStatus('connected');
      setExchangeMessage(`REST/WS bound to ${exchangeConfig.apiBaseURL}`);
      sendLatencyProbe(socket);
      pingIntervalID = window.setInterval(() => sendLatencyProbe(socket), 2000);
    };
    socket.onclose = () => {
      setConnectionStatus('reconnecting');
      setExchangeMessage('Websocket reconnect pending; REST polling remains active');
      if (pingIntervalID !== null) {
        window.clearInterval(pingIntervalID);
        pingIntervalID = null;
      }
    };
    socket.onerror = () => {
      setConnectionStatus('reconnecting');
    };

    return () => {
      if (pingIntervalID !== null) {
        window.clearInterval(pingIntervalID);
        pingIntervalID = null;
      }
      socket.close();
      if (protocolRefreshTimerRef.current !== null) {
        window.clearTimeout(protocolRefreshTimerRef.current);
        protocolRefreshTimerRef.current = null;
      }
      if (orderBookRefreshTimerRef.current !== null) {
        window.clearTimeout(orderBookRefreshTimerRef.current);
        orderBookRefreshTimerRef.current = null;
      }
    };
  }, [exchangeMode, selectedPairSymbol, activeUserID, appendLog, scheduleProtocolRefresh, refreshOrderBookSnapshot]);

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
    if (exchangeMode !== 'live' || !selectedPairSymbol) {
      const message = 'Exchange API is not connected; order was not submitted.';
      setOrderSubmitError(message);
      appendLog(message, 'ORDER', 'ERROR');
      return;
    }
    if (!activeUserID) {
      const message = 'Order rejected: OIDC session is not loaded.';
      setOrderSubmitError(message);
      appendLog(message, 'ORDER', 'ERROR');
      return;
    }

    try {
      setOrderSubmitError(null);
      const executionPrice = ordData.type === 'MARKET'
        ? marketProtectionPrice(ordData.side, displayedOrderBook, ordData.price)
        : ordData.price;
      appendLog(`Submitting ${ordData.type} ${ordData.side} through exchange API.`, 'ORDER', 'INFO');
      const result = await placeExchangeOrder({
        market: selectedPairSymbol,
        userID: activeUserID,
        side: ordData.side,
        type: ordData.type,
        price: executionPrice,
        amount: ordData.amount,
        stopPrice: ordData.stopPrice,
      });

      if (isActiveOrder(result.order)) {
        setOpenOrders(prev => [result.order, ...prev.filter(o => o.id !== result.order.id)]);
      } else {
        setOrderHistory(prev => [result.order, ...prev.filter(o => o.id !== result.order.id)]);
      }
      if (result.trades.length > 0) {
        setTradeHistory(prev => mergeTrades(result.trades, prev));
        setRecentTrades(prev => mergeTrades(result.trades, prev));
      }
      scheduleProtocolRefresh(900);
      appendLog(`Exchange accepted ${result.order.id}: ${result.order.status} ${formatProtocolDecimal(result.order.filled)}/${formatProtocolDecimal(result.order.amount)}.`, 'ORDER', 'SUCCESS');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'unknown protocol error';
      setOrderSubmitError(message);
      appendLog(`Exchange order rejected: ${message}`, 'ORDER', 'ERROR');
    }
  };

  // Cancele standing order
  const handleCancelOrder = async (id: string) => {
    if (exchangeMode !== 'live') {
      appendLog(`Cancel rejected: exchange API is not connected for order ${id}.`, 'ORDER', 'ERROR');
      return;
    }
    if (!activeUserID) {
      appendLog(`Cancel rejected: OIDC session is not loaded for order ${id}.`, 'ORDER', 'ERROR');
      return;
    }

    try {
      const cancelled = await cancelExchangeOrder(id, activeUserID);
      setOpenOrders(prev => prev.filter(o => o.id !== id));
      setOrderHistory(prev => [cancelled, ...prev.filter(o => o.id !== id)]);
      scheduleProtocolRefresh(900);
      appendLog(`Cancelled exchange order ${id}. Backend released reserved funds.`, 'ORDER', 'WARNING');
    } catch (err) {
      appendLog(`Cancel rejected by exchange: ${err instanceof Error ? err.message : 'unknown protocol error'}`, 'ORDER', 'ERROR');
    }
  };

  // Cancel all pending orders in batch
  const handleCancelAllOrders = () => {
    if (openOrders.length === 0) return;
    openOrders.forEach(o => handleCancelOrder(o.id));
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

  // Handle deposit funds
  const handleDeposit = async (asset: string, chainKey: string): Promise<DepositAddressInfo> => {
    if (exchangeMode !== 'live') {
      const message = 'Deposit rejected: exchange API is not connected.';
      appendLog(message, 'SYSTEM', 'ERROR');
      throw new Error(message);
    }
    if (!activeUserID) {
      const message = 'Deposit rejected: OIDC session is not loaded.';
      appendLog(message, 'SYSTEM', 'ERROR');
      throw new Error(message);
    }

    try {
      const depositAddress = await requestExchangeDepositAddress(activeUserID, asset, chainKey);
      appendLog(`Gateway deposit address ready for ${asset} on ${chainKey}: ${depositAddress.address}.`, 'SYSTEM', 'SUCCESS');
      return depositAddress;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'unknown deposit address error';
      appendLog(`Gateway deposit rejected: ${message}`, 'SYSTEM', 'ERROR');
      throw new Error(message);
    }
  };

  // Handle withdrawals
  const handleWithdraw = async (asset: string, chainKey: string, address: string, amount: number) => {
    if (exchangeMode !== 'live') {
      const message = 'Withdrawal rejected: exchange API is not connected.';
      appendLog(message, 'SYSTEM', 'ERROR');
      throw new Error(message);
    }
    if (!activeUserID) {
      const message = 'Withdrawal rejected: OIDC session is not loaded.';
      appendLog(message, 'SYSTEM', 'ERROR');
      throw new Error(message);
    }

    try {
      const withdrawal = await requestExchangeWithdrawal(activeUserID, asset, chainKey, address, amount);
      const refreshedBalances = await fetchBalances(activeUserID).catch(() => null);
      if (refreshedBalances) {
        setBalances(refreshedBalances);
        setBalancesError(null);
      }
      scheduleProtocolRefresh(900);
      const txId = withdrawal.id || `W-${Date.now()}`;
      setWalletTransactions(prev => [{ id: txId, type: 'WITHDRAW', asset, chainKey, amount, time: new Date() }, ...prev]);
      appendLog(`Gateway withdrawal requested. -${amount} ${asset} on ${chainKey} to ${address}.`, 'SYSTEM', 'WARNING');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'unknown withdrawal error';
      appendLog(`Gateway withdrawal rejected: ${message}`, 'SYSTEM', 'ERROR');
      throw new Error(message);
    }
  };

  // Purge dbs settings reset
  const handlePurgeDbs = () => {
    setOpenOrders([]);
    setOrderHistory([]);
    setBalances([]);
    setBalancesError(null);
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
        resetMarketData(tabObj.symbol);
        setSelectedPairSymbol(tabObj.symbol);
        setActiveView('TRADE');
      } else if (tabObj.type === 'PORTFOLIO') {
        setActiveView('PORTFOLIO');
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
        resetMarketData(nextActive.symbol);
        setSelectedPairSymbol(nextActive.symbol);
      }
    }
  };

  const handleSelectPair = (symbol: string) => {
    resetMarketData(symbol);
    setSelectedPairSymbol(symbol);
    
    // Check if symbol already exists as tabs, if not, append editor file tab
    const alreadyOpen = openTabs.find(t => t.id === symbol);
    if (!alreadyOpen) {
      setOpenTabs(prev => sortTabs([...prev, { id: symbol, title: symbol, type: 'MARKET', symbol }]));
    }
    
    setActiveTabId(symbol);
    setActiveView('TRADE');
  };

  const triggerRescanTickers = () => {
    setConnectionStatus('reconnecting');
    setProtocolRevision(rev => rev + 1);
    appendLog('Manual backend refresh requested.', 'WEBSOCKET', 'INFO');
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
      id: 'act-deposit',
      category: 'ACCOUNT FUNDING',
      title: `Get ${selectedMarketObj?.quoteAsset || 'QUOTE'} deposit address`,
      subtitle: 'Requests a gateway static address and QR code.',
      icon: Wallet,
      action: () => {
        if (!selectedMarketObj) return;
        const chainKey = defaultChainKeyForAsset(assetMetadata, selectedMarketObj.quoteAsset);
        if (!chainKey) {
          appendLog(`No registry chain is available for ${selectedMarketObj.quoteAsset}.`, 'SYSTEM', 'ERROR');
          return;
        }
        void handleDeposit(selectedMarketObj.quoteAsset, chainKey).catch(() => undefined);
      },
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

  const renderOrderTicket = (docked = false) => selectedMarketObj ? (
    <OrderForm
      pair={selectedMarketObj}
      availableUsdt={balances.find(b => b.asset === selectedMarketObj.quoteAsset)?.free || 0}
      availableBase={balances.find(b => b.asset === selectedMarketObj.baseAsset)?.free || 0}
      onSubmitOrder={handleOrderSubmit}
      selectedPrice={orderBookSelection?.price ?? null}
      selectedAmount={orderBookSelection?.amount ?? null}
      clearSelectedPrice={() => setOrderBookSelection(null)}
      submitError={orderSubmitError}
      docked={docked}
    />
  ) : null;

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
              } else if (v === 'TRADE' && !openTabs.some(t => t.symbol)) {
                // ensure at least one market tab is selected if user goes to trade
                if (markets[0]) {
                  handleSelectPair(markets[0].symbol);
                }
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
          accountLabel={accountLabel}
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
          <div className="flex items-center border-b border-[#e1e4e8] dark:border-[#21262d] bg-[#f6f8fa] dark:bg-[#0d1117] overflow-hidden py-1.5 px-3 min-h-[40px]">
            <div className="flex min-w-0 flex-1 gap-1 overflow-x-auto overflow-y-hidden scrollbar-hide">
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
                  }
                  return null;
                };

                return (
                  <button
                    key={tab.id}
                    onClick={() => handleSelectTab(tab.id)}
                    className={`flex shrink-0 items-center gap-1.5 px-2.5 py-1 text-xs font-mono font-semibold rounded border transition-all whitespace-nowrap cursor-pointer select-none ${
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
              onContinueWithoutOIDC={() => setActiveView('TRADE')}
            />
          ) : activeView === 'PORTFOLIO' || activeView === 'WALLET' ? (
            /* PORTFOLIO VIEW */
            <PortfolioView
              balances={balances}
              balancesError={balancesError}
              onDeposit={handleDeposit}
              onWithdraw={handleWithdraw}
              transactions={walletTransactions}
              assetMetadata={assetMetadata}
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
              userEmail={accountLabel || 'Unauthenticated'}
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
            <div className="flex-1 w-full min-w-0 max-w-none overflow-y-auto p-4 sm:p-5 bg-[#fafbfc] dark:bg-[#070b0f] space-y-4 select-none h-full">
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
                recentTrades={recentTrades}
                systemLogs={systemLogs}
                selectedMarket={selectedMarketObj}
                selectedAssetSymbol={selectedAssetSymbol}
                dexPrices={dexPrices}
                dexPricesLoading={dexPricesLoading}
                dexPricesError={dexPricesError}
                assetMetadata={assetMetadata}
                onCancelOrder={handleCancelOrder}
                onCancelAllOrders={handleCancelAllOrders}
              />
            </div>
          ) : !selectedMarketObj ? (
            <div className="flex-1 p-5 bg-[#fafbfc] dark:bg-[#070b0f] flex items-center justify-center">
              <div className="max-w-md w-full bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg p-6 text-center shadow-sm">
                <Cpu className="w-8 h-8 text-accent-1 mx-auto mb-3" />
                <h2 className="text-sm font-display font-semibold text-gray-900 dark:text-gray-100 mb-2">
                  Waiting for backend markets
                </h2>
                <p className="text-xs font-mono text-gray-500 dark:text-gray-400">
                  {exchangeMode === 'offline'
                    ? exchangeMessage
                    : 'No market data has been returned by the exchange API yet.'}
                </p>
              </div>
            </div>
          ) : (
            /* PRIMARY CORE WORKSTATION: TRADE DESK LAYOUT */
            <div className="flex-1 overflow-y-auto bg-[#fafbfc] p-2 dark:bg-[#070b0f] sm:p-3">
              <div className="grid grid-cols-1 gap-3 xl:grid-cols-[minmax(0,1fr)_minmax(220px,0.58fr)]">

                {/* Top row: chart and orderbook aligned; order ticket is docked on desktop */}
                <div data-testid="trade-chart-panel" className="min-w-0 xl:h-[calc(100vh-132px)] xl:min-h-[520px] xl:max-h-[640px]">
                  <MarketChart
                    pair={selectedMarketObj}
                    candles={candles}
                    timeframe={timeframe}
                    setTimeframe={setTimeframe}
                    assetMetadata={assetMetadata}
                  />
                </div>

                <div data-testid="trade-orderbook-panel" className="min-w-0 xl:h-[calc(100vh-132px)] xl:min-h-[520px] xl:max-h-[640px]">
                  <OrderBookView
                    orderBook={displayedOrderBook}
                    pair={selectedMarketObj}
                    onSelectPrice={setOrderBookSelection}
                  />
                </div>

                <div data-testid="trade-order-ticket-panel" className="min-w-0 xl:hidden">
                  {renderOrderTicket(false)}
                </div>

                <div className="min-w-0 xl:col-span-2">
                  <TerminalPanel
                    openOrders={openOrders}
                    orderHistory={orderHistory}
                    tradeHistory={tradeHistory}
                    recentTrades={recentTrades}
                    systemLogs={systemLogs}
                    selectedMarket={selectedMarketObj}
                    selectedAssetSymbol={selectedAssetSymbol}
                    dexPrices={dexPrices}
                    dexPricesLoading={dexPricesLoading}
                    dexPricesError={dexPricesError}
                    assetMetadata={assetMetadata}
                    onCancelOrder={handleCancelOrder}
                    onCancelAllOrders={handleCancelAllOrders}
                  />
                </div>
              </div>
            </div>
          )}

        </div>

        {activeView === 'TRADE' && selectedMarketObj && (
          <aside
            data-testid="trade-order-ticket-dock"
            className="hidden h-full w-[320px] shrink-0 flex-col border-l border-[#e1e4e8] bg-[#fdfdfd] dark:border-[#21262d] dark:bg-[#090d12] xl:flex 2xl:w-[340px]"
          >
            {renderOrderTicket(true)}
          </aside>
        )}

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
            Exchange: {exchangeMode === 'live' ? 'LIVE REST/WS' : exchangeMode === 'connecting' ? 'CONNECTING' : 'API OFFLINE'}
          </span>

        </div>

        {/* Sync triggers right */}
        <div className="flex items-center gap-4.5">
          <span className="text-[#ffe05c]" title="Spot mode operates without liabilities">
            Account: {accountLabel || 'Unauthenticated'}
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
  return sortMarkets(remote).map((market) => {
    const existing = current.find(item => item.symbol === market.symbol);
    if (!existing) return market;
    return {
      ...market,
      isFavorite: existing.isFavorite ?? market.isFavorite,
    };
  });
}

function readRouteSnapshot(): RouteSnapshot {
  if (typeof window === 'undefined') {
    return { view: 'TRADE', market: '' };
  }

  const path = normalizePath(window.location.pathname);
  const params = new URLSearchParams(window.location.search);
  const marketFromQuery = params.get('market') || params.get('pair') || '';
  const tradePathMatch = path.match(/^\/trade\/(.+)$/);
  const marketFromPath = tradePathMatch ? decodePathSegment(tradePathMatch[1]) : '';
  const market = normalizeMarketSymbol(marketFromQuery || marketFromPath);

  return {
    view: viewFromPath(path),
    market,
  };
}

function routePathForState(view: string, market: string): string {
  const normalizedView = normalizeView(view);
  const path = pathForView(normalizedView);
  if (normalizedView !== 'TRADE') return path;

  const normalizedMarket = normalizeMarketSymbol(market);
  if (!normalizedMarket) return path;
  const params = new URLSearchParams({ market: compactMarketSymbol(normalizedMarket) });
  return `${path}?${params.toString()}`;
}

function initialTabsForRoute(snapshot: RouteSnapshot): Tab[] {
  return ensureRouteTab([
    { id: 'PORTFOLIO', title: 'Portfolio', type: 'PORTFOLIO' },
  ], snapshot);
}

function ensureRouteTab(tabs: Tab[], snapshot: RouteSnapshot): Tab[] {
  const supportedTabs = sanitizeWorkspaceTabs(tabs);
  if (!snapshot.market) return supportedTabs;
  if (supportedTabs.some(tab => tab.id === snapshot.market)) return supportedTabs;
  return sortTabs([...supportedTabs, { id: snapshot.market, title: snapshot.market, type: 'MARKET', symbol: snapshot.market }]);
}

function tabIdForRoute(snapshot: RouteSnapshot): string {
  if (snapshot.view === 'TRADE') return snapshot.market;
  if (snapshot.view === 'PORTFOLIO' || snapshot.view === 'WALLET') return 'PORTFOLIO';
  return '';
}

function viewFromPath(path: string): string {
  if (path === '/markets') return 'MARKETS';
  if (path === '/portfolio') return 'PORTFOLIO';
  if (path === '/orders') return 'ORDERS';
  if (path === '/wallet') return 'WALLET';
  if (path === '/settings') return 'SETTINGS';
  if (path === '/login') return 'LOGIN';
  return 'TRADE';
}

function pathForView(view: string): string {
  switch (normalizeView(view)) {
    case 'MARKETS':
      return '/markets';
    case 'PORTFOLIO':
      return '/portfolio';
    case 'ORDERS':
      return '/orders';
    case 'WALLET':
      return '/wallet';
    case 'SETTINGS':
      return '/settings';
    case 'LOGIN':
      return '/login';
    default:
      return '/trade';
  }
}

function normalizeView(view: string): string {
  const upper = view.toUpperCase();
  if (['MARKETS', 'TRADE', 'PORTFOLIO', 'ORDERS', 'WALLET', 'SETTINGS', 'LOGIN'].includes(upper)) {
    return upper;
  }
  return 'TRADE';
}

function normalizePath(path: string): string {
  if (!path || path === '/') return '/trade';
  return path.replace(/\/+$/, '') || '/trade';
}

function normalizeMarketSymbol(symbol: string): string {
  const upper = symbol.trim().toUpperCase().replace(/[_:\-\s]+/g, '/');
  if (!upper) return '';
  if (upper.includes('/')) {
    const [base, quote] = upper.split('/').filter(Boolean);
    return base && quote ? `${base}/${quote}` : upper.replace(/\//g, '');
  }

  const compact = upper.replace(/[^A-Z0-9]/g, '');
  const quote = compactQuoteAssets.find((item) => compact.endsWith(item) && compact.length > item.length);
  if (!quote) return compact;
  return `${compact.slice(0, -quote.length)}/${quote}`;
}

function compactMarketSymbol(symbol: string): string {
  return normalizeMarketSymbol(symbol).replace('/', '');
}

function decodePathSegment(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

const compactQuoteAssets = [
  'CHZINU',
  'USDC',
  'USDT',
  'WETH',
  'WBTC',
  'USD',
  'CHZ',
  'SOL',
  'ETH',
  'BNB',
  'BTC',
  'AVAX',
];

function replaceMarketTabs(current: Tab[], markets: MarketPair[]): Tab[] {
  const marketTabs = sortMarkets(markets).slice(0, 3).map((market) => ({
    id: market.symbol,
    title: market.symbol,
    type: 'MARKET' as const,
    symbol: market.symbol,
  }));
  const utilityTabs = sanitizeWorkspaceTabs(current).filter(tab => tab.type !== 'MARKET');
  return [...marketTabs, ...utilityTabs];
}

function sortTabs(tabs: Tab[]): Tab[] {
  const supportedTabs = sanitizeWorkspaceTabs(tabs);
  const marketTabs = supportedTabs
    .filter(tab => tab.type === 'MARKET')
    .sort((a, b) => (a.symbol || a.id).localeCompare(b.symbol || b.id));
  const utilityTabs = supportedTabs.filter(tab => tab.type !== 'MARKET');
  return [...marketTabs, ...utilityTabs];
}

function sortMarkets(markets: MarketPair[]): MarketPair[] {
  return [...markets].sort(compareMarkets);
}

function compareMarkets(a: MarketPair, b: MarketPair): number {
  const baseDelta = a.baseAsset.localeCompare(b.baseAsset);
  if (baseDelta !== 0) return baseDelta;
  const quoteDelta = a.quoteAsset.localeCompare(b.quoteAsset);
  if (quoteDelta !== 0) return quoteDelta;
  return a.symbol.localeCompare(b.symbol);
}

function emptyOrderBook(market = ''): OrderBook {
  return {
    market,
    bids: [],
    asks: [],
    spread: 0,
    spreadPercent: 0,
  };
}

function marketProtectionPrice(side: OrderSide, book: OrderBook, fallback: number): number {
  const levels = side === 'SELL' ? book.bids : book.asks;
  const visibleEdgePrice = levels[levels.length - 1]?.price;
  if (Number.isFinite(visibleEdgePrice) && visibleEdgePrice > 0) {
    return visibleEdgePrice;
  }
  if (Number.isFinite(fallback) && fallback > 0) {
    return side === 'BUY' ? fallback * 1.05 : fallback * 0.95;
  }
  return fallback;
}

function isActiveOrder(order: Order): boolean {
  const isOpenStatus = order.status === 'OPEN' || order.status === 'PENDING' || order.status === 'PARTIALLY_FILLED';
  return isOpenStatus && order.type !== 'MARKET' && order.remaining >= ACTIVE_ORDER_REMAINING_EPSILON;
}

function formatProtocolDecimal(value: number): string {
  if (!Number.isFinite(value)) return '0.00000000';
  return value.toLocaleString('en-US', {
    useGrouping: false,
    minimumFractionDigits: 8,
    maximumFractionDigits: 8,
  });
}

function mergeTrades(incoming: Trade[], current: Trade[]): Trade[] {
  const byID = new Map<string, Trade>();
  [...incoming, ...current].forEach((trade) => byID.set(trade.id, trade));
  return sortTradesDesc(Array.from(byID.values())).slice(0, 120);
}

function insertOrReplaceOrder(current: Order[], incoming: Order): Order[] {
  const next = [incoming, ...current.filter(order => order.id !== incoming.id)];
  next.sort((left, right) => right.timestamp.getTime() - left.timestamp.getTime());
  return next.slice(0, 120);
}

function sortTradesDesc(trades: Trade[]): Trade[] {
  return [...trades].sort((a, b) => b.timestamp.getTime() - a.timestamp.getTime());
}

function applyMarketTrades(current: MarketPair[], trades: Trade[]): MarketPair[] {
  if (trades.length === 0) return current;
  const latestTradeByMarket = new Map<string, Trade>();
  trades.forEach((trade) => {
    const existing = latestTradeByMarket.get(trade.symbol);
    if (!existing || trade.timestamp.getTime() > existing.timestamp.getTime()) {
      latestTradeByMarket.set(trade.symbol, trade);
    }
  });
  return current.map((market) => {
    const latestTrade = latestTradeByMarket.get(market.symbol);
    if (!latestTrade) return market;
    return {
      ...market,
      lastPrice: latestTrade.price,
    };
  });
}

function assetMetadataBySymbol(assets: AssetInfo[]): Record<string, AssetInfo> {
  const out: Record<string, AssetInfo> = {};
  assets.forEach((asset) => {
    const symbol = asset.symbol?.toUpperCase();
    if (symbol) {
      out[symbol] = {
        ...asset,
        registry_symbol: symbol,
      };
    }
    (asset.deployments || []).forEach((deployment) => {
      const deploymentSymbol = deployment.symbol?.toUpperCase();
      if (deploymentSymbol && deploymentSymbol !== symbol) {
        out[deploymentSymbol] = {
          ...asset,
          symbol: deployment.symbol,
          registry_symbol: symbol,
          name: deployment.name || asset.name,
          decimals: deployment.decimals ?? asset.decimals,
          icon_url: deployment.icon_url || asset.icon_url,
        };
      }
    });
  });
  return out;
}

function defaultChainKeyForAsset(assetMetadata: Record<string, AssetInfo>, symbol: string): string {
  const asset = assetMetadata[symbol.toUpperCase()];
  const deployments = asset?.deployments || [];
  const ordered = deployments.filter((deployment) => deployment.enabled !== false && deployment.chain_key);
  return ordered[0]?.chain_key || '';
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

function socketEventBelongsToUser(event: unknown, activeUserID: string): boolean {
  if (!activeUserID) return false;
  if (!isUnknownRecord(event)) return false;
  const eventUserID = stringValue(event, ['user_id', 'userID', 'UserID']);
  return eventUserID === '' || eventUserID === activeUserID;
}

function orderFromSocketEvent(event: unknown): Order | null {
  if (!isUnknownRecord(event)) return null;
  const rawOrder = event.order || event.Order;
  if (!isUnknownRecord(rawOrder)) return null;

  const id = stringValue(rawOrder, ['id', 'ID']);
  const symbol = stringValue(rawOrder, ['market', 'symbol', 'Market', 'Symbol']);
  const side = stringValue(rawOrder, ['side', 'Side']).toUpperCase();
  const type = stringValue(rawOrder, ['type', 'Type']).toUpperCase();
  const status = stringValue(rawOrder, ['status', 'Status']).toUpperCase();
  const price = numericValue(rawOrder, ['price', 'Price']);
  const amount = numericValue(rawOrder, ['quantity', 'Quantity']);
  const filled = numericValue(rawOrder, ['filled_quantity', 'filled', 'FilledQuantity']);
  const remaining = numericValue(rawOrder, ['remaining_quantity', 'remaining', 'RemainingQuantity']);
  if (!id || !symbol || !side || !type || !status) {
    return null;
  }

  return {
    id,
    symbol,
    side: side as OrderSide,
    type: type === 'STOP_LIMIT' ? 'STOP_LIMIT' : (type as OrderType),
    price,
    amount,
    filled,
    remaining,
    total: price * amount,
    stopPrice: numericValue(rawOrder, ['stop_price', 'stopPrice', 'StopPrice']) || undefined,
    status: mapSocketOrderStatus(status),
    timestamp: socketEventTime(rawOrder),
  };
}

function tradesFromSocketEvent(event: unknown): Trade[] {
  if (!isUnknownRecord(event) || !Array.isArray(event.trades)) {
    return [];
  }
  return event.trades.flatMap((item) => {
    if (!isUnknownRecord(item)) return [];
    const id = stringValue(item, ['id', 'ID']);
    const symbol = stringValue(item, ['market', 'symbol', 'Market', 'Symbol']);
    const side = stringValue(item, ['taker_side', 'side', 'Side']).toUpperCase();
    const price = numericValue(item, ['price', 'Price']);
    const amount = numericValue(item, ['quantity', 'amount', 'Quantity']);
    const total = numericValue(item, ['quote_quantity', 'total', 'QuoteQuantity']) || (price * amount);
    if (!id || !symbol || !side) {
      return [];
    }
    return [{
      id,
      symbol,
      side: side as OrderSide,
      price,
      amount,
      total,
      timestamp: socketEventTime(item),
    }];
  });
}

function mapSocketOrderStatus(status: string): Order['status'] {
  switch (status.toLowerCase()) {
    case 'open':
      return 'OPEN';
    case 'pending_match':
    case 'pending_stop':
      return 'PENDING';
    case 'partially_filled':
      return 'PARTIALLY_FILLED';
    case 'filled':
      return 'FILLED';
    case 'canceled':
      return 'CANCELLED';
    case 'expired':
      return 'EXPIRED';
    case 'rejected':
      return 'REJECTED';
    default:
      return 'PENDING';
  }
}

function socketEventTime(record: Record<string, unknown>): Date {
  const value = stringValue(record, ['created_at', 'createdAt', 'timestamp', 'Timestamp']);
  if (!value) return new Date();
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? new Date() : parsed;
}

function balanceFromSocketEvent(event: unknown): AssetBalance | null {
  if (!isUnknownRecord(event)) return null;
  const rawBalance = event.balance || event.Balance;
  if (!isUnknownRecord(rawBalance)) return null;

  const asset = stringValue(rawBalance, ['asset', 'Asset']).toUpperCase();
  if (!asset) return null;

  const free = numericValue(rawBalance, ['available', 'Available', 'free', 'Free']);
  const locked = numericValue(rawBalance, ['locked', 'Locked']);
  const frozen = numericValue(rawBalance, ['frozen', 'Frozen', 'pending', 'Pending']);

  return {
    asset,
    name: asset,
    free,
    locked,
    frozen,
    valueUsd: free + locked + frozen,
    change24h: 0,
  };
}

function upsertBalance(current: AssetBalance[], incoming: AssetBalance): AssetBalance[] {
  const asset = incoming.asset.toUpperCase();
  const existing = current.find(item => item.asset.toUpperCase() === asset);
  const next = {
    ...incoming,
    asset,
    name: existing?.name || incoming.name || asset,
    change24h: existing?.change24h || incoming.change24h,
  };

  if (!existing) {
    return [...current, next];
  }

  return current.map(item => item.asset.toUpperCase() === asset ? next : item);
}

function applyOrderBookDelta(current: OrderBook, event: unknown): OrderBook {
  if (!isUnknownRecord(event)) return current;
  const market = stringValue(event, ['market', 'Market']) || current.market;
  const bids = mergeOrderBookSide(current.bids, Array.isArray(event.bids) ? event.bids : [], 'bid');
  const asks = mergeOrderBookSide(current.asks, Array.isArray(event.asks) ? event.asks : [], 'ask');
  const bestBid = bids[0]?.price || 0;
  const bestAsk = asks[0]?.price || 0;
  const spread = bestBid > 0 && bestAsk > 0 ? bestAsk - bestBid : 0;
  const spreadPercent = bestAsk > 0 ? (spread / bestAsk) * 100 : 0;
  return {
    market,
    bids,
    asks,
    spread,
    spreadPercent,
  };
}

function mergeOrderBookSide(current: OrderBook['bids'], deltas: unknown[], side: 'bid' | 'ask'): OrderBook['bids'] {
  const byPrice = new Map<string, { price: number; amount: number }>();
  current.forEach((level) => {
    byPrice.set(orderBookPriceKey(level.price), { price: level.price, amount: level.amount });
  });
  deltas.forEach((item) => {
    if (!isUnknownRecord(item)) return;
    const price = numericValue(item, ['price', 'Price']);
    const amount = numericValue(item, ['quantity', 'amount', 'Quantity']);
    const key = orderBookPriceKey(price);
    if (!Number.isFinite(price) || price <= 0) return;
    if (!Number.isFinite(amount) || amount < ACTIVE_ORDER_REMAINING_EPSILON) {
      byPrice.delete(key);
      return;
    }
    byPrice.set(key, { price, amount });
  });

  const levels = Array.from(byPrice.values())
    .sort((left, right) => side === 'bid' ? right.price - left.price : left.price - right.price)
    .map(({ price, amount }) => ({
      price,
      amount,
      total: price * amount,
      cumulativeAmount: 0,
      cumulativeTotal: 0,
      depthPercent: 0,
    }));

  let cumulativeAmount = 0;
  let cumulativeTotal = 0;
  levels.forEach((level) => {
    cumulativeAmount += level.amount;
    cumulativeTotal += level.total;
    level.cumulativeAmount = cumulativeAmount;
    level.cumulativeTotal = cumulativeTotal;
  });

  const maxTotal = levels[levels.length - 1]?.cumulativeTotal || 1;
  levels.forEach((level) => {
    level.depthPercent = (level.cumulativeTotal / maxTotal) * 100;
  });

  return levels;
}

function orderBookPriceKey(price: number): string {
  return Number.isFinite(price) ? price.toFixed(8) : '0.00000000';
}

function isUnknownRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function stringValue(record: Record<string, unknown>, keys: string[]): string {
  for (const key of keys) {
    const value = record[key];
    if (typeof value === 'string') return value.trim();
    if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  }
  return '';
}

function numericValue(record: Record<string, unknown>, keys: string[]): number {
  for (const key of keys) {
    const value = record[key];
    if (typeof value === 'number' && Number.isFinite(value)) return value;
    if (typeof value === 'string' && value.trim() !== '') {
      const parsed = Number(value);
      if (Number.isFinite(parsed)) return parsed;
    }
  }
  return 0;
}

function priceKey(price: AssetPriceResponse['prices'][number]): string {
  return `${price.chain_key}:${price.venue_key}:${price.pool_id}`;
}

function comparePoolPrices(a: AssetPriceResponse['prices'][number], b: AssetPriceResponse['prices'][number]): number {
  const chainDelta = chainRank(a.chain_key, defaultChainOrder) - chainRank(b.chain_key, defaultChainOrder);
  if (chainDelta !== 0) return chainDelta;
  const venueDelta = a.venue_key.localeCompare(b.venue_key);
  if (venueDelta !== 0) return venueDelta;
  return a.pool_id.localeCompare(b.pool_id);
}

const defaultChainOrder = ['chiliz', 'base', 'solana', 'ethereum', 'avalanche', 'arbitrum', 'unichain', 'binance_smart_chain'];

function chainRank(chain: string, order: string[]): number {
  const index = order.indexOf(chain);
  return index === -1 ? order.length : index;
}
