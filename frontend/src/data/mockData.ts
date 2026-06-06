/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import { MarketPair, Candle, Order, Trade, AssetBalance, OrderBook, OrderBookLevel, SystemLog, TradingStrategy } from '../types/trading';
import { BRAND_NAME } from '../constants/brand';

// Available market pairs
export const INITIAL_MARKETS: MarketPair[] = [
  {
    symbol: 'PEPPER/USD',
    baseAsset: 'PEPPER',
    quoteAsset: 'USD',
    lastPrice: 0.000000001,
    change24h: 4.2,
    high24h: 0.0000000012,
    low24h: 0.0000000008,
    volume24h: 1250000000,
    liquidity: 4.5,
    isFavorite: true,
  },
  {
    symbol: 'CHZ/USD',
    baseAsset: 'CHZ',
    quoteAsset: 'USD',
    lastPrice: 0.08,
    change24h: 1.85,
    high24h: 0.083,
    low24h: 0.076,
    volume24h: 18450000,
    liquidity: 18.4,
    isFavorite: true,
  },
  {
    symbol: 'SOL/USD',
    baseAsset: 'SOL',
    quoteAsset: 'USD',
    lastPrice: 184.25,
    change24h: 8.65,
    high24h: 186.90,
    low24h: 169.10,
    volume24h: 642300.00,
    liquidity: 95.8,
    isFavorite: false,
  },
  {
    symbol: 'ETH/USD',
    baseAsset: 'ETH',
    quoteAsset: 'USD',
    lastPrice: 3412.80,
    change24h: -1.25,
    high24h: 3510.50,
    low24h: 3380.00,
    volume24h: 89420.15, // in ETH
    liquidity: 210.2,
    isFavorite: false,
  },
  {
    symbol: 'AVAX/USD',
    baseAsset: 'AVAX',
    quoteAsset: 'USD',
    lastPrice: 32.40,
    change24h: -4.12,
    high24h: 34.20,
    low24h: 31.95,
    volume24h: 981200.00,
    liquidity: 38.6,
    isFavorite: false,
  },
  {
    symbol: 'USDC/USD',
    baseAsset: 'USDC',
    quoteAsset: 'USD',
    lastPrice: 1.00,
    change24h: 0.01,
    high24h: 1.01,
    low24h: 0.99,
    volume24h: 9800000,
    liquidity: 125.0,
    isFavorite: false,
  }
];

// Seed initial wallet asset balances
export const INITIAL_BALANCES: AssetBalance[] = [
  { asset: 'USD', name: 'US Dollar', free: 42550.00, locked: 1200.00, valueUsd: 43750.00, change24h: 0 },
  { asset: 'PEPPER', name: 'PEPPER', free: 125000000.00000000, locked: 25000000.00000000, valueUsd: 0.15, change24h: 4.2 },
  { asset: 'CHZ', name: 'Chiliz', free: 1250.00000000, locked: 100.00000000, valueUsd: 108.00, change24h: 1.85 },
  { asset: 'ETH', name: 'Ethereum', free: 2.14500000, locked: 0.00000000, valueUsd: 7320.46, change24h: -1.25 },
  { asset: 'SOL', name: 'Solana', free: 45.22000000, locked: 5.00000000, valueUsd: 9253.04, change24h: 8.65 },
  { asset: 'AVAX', name: 'Avalanche', free: 140.00000000, locked: 10.00000000, valueUsd: 4860.00, change24h: -4.12 },
  { asset: 'USDC', name: 'USD Coin', free: 5000.00000000, locked: 0.00000000, valueUsd: 5000.00, change24h: 0.01 },
];

// Helper to generate historical pseudo-currencies candlesticks
export function generateCandles(symbol: string, timeframe: string, count: number = 100): Candle[] {
  const market = INITIAL_MARKETS.find(m => m.symbol === symbol) || INITIAL_MARKETS[0];
  let price = market.lastPrice;
  const candles: Candle[] = [];
  
  // Calculate historical starting timestamp
  const nowInSeconds = Math.floor(Date.now() / 1000);
  let step = 60; // 1m default
  if (timeframe === '5m') step = 5 * 60;
  else if (timeframe === '15m') step = 15 * 60;
  else if (timeframe === '1h') step = 60 * 60;
  else if (timeframe === '4h') step = 4 * 60 * 60;
  else if (timeframe === '1d') step = 24 * 60 * 60;

  let currentPrice = price;
  for (let i = count - 1; i >= 0; i--) {
    const candleTime = nowInSeconds - i * step;
    
    // Generate price volatility
    const change = currentPrice * (0.01 + Math.random() * 0.015) * (Math.random() > 0.48 ? 1 : -1);
    const open = currentPrice - change;
    const close = currentPrice;
    const high = Math.max(open, close) + (Math.random() * 0.008 * currentPrice);
    const low = Math.min(open, close) - (Math.random() * 0.008 * currentPrice);
    const volume = (market.volume24h / (24 * 4)) * (0.3 + Math.random() * 1.5);
    
    candles.push({
      time: candleTime,
      open,
      high,
      low,
      close,
      volume,
    });

    currentPrice = open; // work backwards
  }

  // Correlate final candle close to lastPrice exactly and fix array order
  candles[candles.length - 1].close = price;
  
  return candles;
}

// Generate highly detailed order books
export function generateOrderBook(lastPrice: number, spreadPercent: number = 0.03): OrderBook {
  const bids: OrderBookLevel[] = [];
  const asks: OrderBookLevel[] = [];
  const precision = lastPrice < 0.01 ? 12 : 2;
  
  const stepPercent = spreadPercent / 20; // depth levels
  const baseBidPrice = lastPrice * (1 - spreadPercent / 100);
  const baseAskPrice = lastPrice * (1 + spreadPercent / 100);

  let cumulativeBidTotal = 0;
  let cumulativeAskTotal = 0;

  // Generate 15 levels of bids and asks
  for (let i = 0; i < 15; i++) {
    // Price offsets
    const bidPrice = baseBidPrice * (1 - (i * stepPercent) / 100);
    const askPrice = baseAskPrice * (1 + (i * stepPercent) / 100);

    // Dynamic, realistic volumes with higher depth clustered at specific areas
    const bidAmount = (Math.random() * 1.5 + 0.1) * (1.5 - (i * 0.05));
    const askAmount = (Math.random() * 1.5 + 0.1) * (1.5 - (i * 0.05));

    const bidTotal = bidPrice * bidAmount;
    const askTotal = askPrice * askAmount;

    cumulativeBidTotal += bidTotal;
    cumulativeAskTotal += askTotal;

    bids.push({
      price: Number(bidPrice.toFixed(precision)),
      amount: Number(bidAmount.toFixed(4)),
      total: Number(bidTotal.toFixed(2)),
      cumulativeTotal: Number(cumulativeBidTotal.toFixed(2)),
      depthPercent: 0, // calculated later
    });

    asks.push({
      price: Number(askPrice.toFixed(precision)),
      amount: Number(askAmount.toFixed(4)),
      total: Number(askTotal.toFixed(2)),
      cumulativeTotal: Number(cumulativeAskTotal.toFixed(2)),
      depthPercent: 0, // calculated later
    });
  }

  // Calculate deep relative percentages for visual visualization in order book
  const maxBidCumulative = bids[bids.length - 1].cumulativeTotal;
  bids.forEach(level => {
    level.depthPercent = (level.cumulativeTotal / maxBidCumulative) * 100;
  });

  const maxAskCumulative = asks[asks.length - 1].cumulativeTotal;
  asks.forEach(level => {
    level.depthPercent = (level.cumulativeTotal / maxAskCumulative) * 100;
  });

  const spread = asks[0].price - bids[0].price;
  const spreadPercentVal = (spread / asks[0].price) * 100;

  return {
    bids,
    asks: asks.sort((a,b) => a.price - b.price), // Sort lower asks first (descending visuals, or reverse)
    spread: Number(spread.toFixed(precision)),
    spreadPercent: Number(spreadPercentVal.toFixed(4)),
  };
}

// Generate seed recent trades list
export function generateRecentTrades(lastPrice: number, symbol: string = 'PEPPER/USD'): Trade[] {
  const trades: Trade[] = [];
  const now = new Date();
  
  for (let i = 0; i < 30; i++) {
    const isBuy = Math.random() > 0.48;
    const priceOffset = lastPrice * (Math.random() * 0.001) * (Math.random() > 0.5 ? 1 : -1);
    const precision = lastPrice < 0.01 ? 12 : 2;
    const tradePrice = Number((lastPrice + priceOffset).toFixed(precision));
    const amount = Number((Math.random() * 2.5 + 0.005).toFixed(4));
    
    trades.push({
      id: `TR-${Math.random().toString(36).substring(2, 9).toUpperCase()}`,
      symbol,
      price: tradePrice,
      amount,
      total: Number((tradePrice * amount).toFixed(2)),
      side: isBuy ? 'BUY' : 'SELL',
      timestamp: new Date(now.getTime() - i * 15 * 1000), // staggered backwards in seconds
    });
  }
  return trades;
}

// Seed initial orders
export const INITIAL_ORDERS: Order[] = [
  {
    id: 'ORD-984210',
    symbol: 'PEPPER/USD',
    side: 'BUY',
    type: 'LIMIT',
    price: 0.0000000009,
    amount: 50000000,
    filled: 0.0000,
    total: 0.045,
    status: 'PENDING',
    timestamp: new Date(Date.now() - 4 * 60 * 60 * 1000),
  },
  {
    id: 'ORD-983194',
    symbol: 'CHZ/USD',
    side: 'SELL',
    type: 'LIMIT',
    price: 0.09,
    amount: 250,
    filled: 0.0000,
    total: 22.50,
    status: 'PENDING',
    timestamp: new Date(Date.now() - 2 * 60 * 1000),
  },
  {
    id: 'ORD-975123',
    symbol: 'ETH/USD',
    side: 'BUY',
    type: 'LIMIT',
    price: 3350.00,
    amount: 1.5000,
    filled: 1.5000,
    total: 5025.00,
    status: 'FILLED',
    timestamp: new Date(Date.now() - 20 * 60 * 1000),
  },
  {
    id: 'ORD-971033',
    symbol: 'SOL/USD',
    side: 'SELL',
    type: 'STOP_LIMIT',
    price: 180.00,
    amount: 10.0000,
    filled: 0.0000,
    total: 1800.00,
    stopPrice: 181.00,
    status: 'PENDING',
    timestamp: new Date(Date.now() - 1 * 24 * 60 * 60 * 1000),
  }
];

// Seed system activity logs (IDE style)
export const INITIAL_LOGS: SystemLog[] = [
  {
    id: 'LOG-1',
    timestamp: new Date(Date.now() - 10 * 60 * 1000),
    type: 'INFO',
    source: 'SYSTEM',
    message: `${BRAND_NAME} Terminal Workspace initialized successfully.`,
  },
  {
    id: 'LOG-2',
    timestamp: new Date(Date.now() - 9.5 * 60 * 1000),
    type: 'SUCCESS',
    source: 'WEBSOCKET',
    message: 'WS streaming connected to high-speed cluster (Region: Europe-West; Latency: 16ms).',
  },
  {
    id: 'LOG-3',
    timestamp: new Date(Date.now() - 8 * 60 * 1000),
    type: 'INFO',
    source: 'STRATEGY',
    message: 'Strategy compiler loaded (v4.1.2-wasm). Ready for Strategy Lab deployment.',
  },
  {
    id: 'LOG-4',
    timestamp: new Date(Date.now() - 5 * 60 * 1000),
    type: 'WARNING',
    source: 'SYSTEM',
    message: 'Spot liquidity deep block warning: high volatility detected near historical margins.',
  },
];

// Initial preconfig user strategies (Strategy Lab)
export const INITIAL_STRATEGIES: TradingStrategy[] = [
  {
    id: 'STRAT-EMA-CROSS',
    name: 'Exponential Moving Average (EMA) Crossover Bot',
    status: 'IDLE',
    code: `// Simple EMA Crossover Strategy
// Buy when EMA(9) crosses above EMA(21). Sell when it crosses below.

function evaluate(marketCandles, currentBalance) {
  const emaFast = calculateEMA(marketCandles, 9);
  const emaSlow = calculateEMA(marketCandles, 21);
  
  const lastEmaFast = emaFast[emaFast.length - 1];
  const prevEmaFast = emaFast[emaFast.length - 2];
  const lastEmaSlow = emaSlow[emaSlow.length - 1];
  const prevEmaSlow = emaSlow[emaSlow.length - 2];
  
  // Golden Cross detection
  if (prevEmaFast <= prevEmaSlow && lastEmaFast > lastEmaSlow) {
    return { action: 'BUY', sizePercent: 100, reason: 'Golden Cross EMA(9, 21)' };
  }
  
  // Death Cross detection
  if (prevEmaFast >= prevEmaSlow && lastEmaFast < lastEmaSlow) {
    return { action: 'SELL', sizePercent: 100, reason: 'Death Cross EMA(9, 21)' };
  }
  
  return { action: 'HOLD', reason: 'Consolidating indicators' };
}`,
    winRate: 64.2,
    totalTrades: 128,
    profitPercent: 42.15,
  },
  {
    id: 'STRAT-RSI-OUTFLOW',
    name: 'RSI Anti-Overbought Mean Reversion',
    status: 'RUNNING',
    code: `// Relative Strength Index (RSI) Divergence Tracker
// BUY when RSI bounces from below 30 (Oversold).
// SELL when RSI retracts from above 70 (Overbought).

function evaluate(candles, balance) {
  const rsiValues = calculateRSI(candles, 14);
  const currentRSI = rsiValues[rsiValues.length - 1];
  const pastRSI = rsiValues[rsiValues.length - 2];
  
  if (pastRSI < 30 && currentRSI >= 30) {
    return { action: 'BUY', sizePercent: 50, reason: 'RSI Oversold Recovered: ' + currentRSI.toFixed(1) };
  }
  
  if (pastRSI > 70 && currentRSI <= 70) {
    return { action: 'SELL', sizePercent: 80, reason: 'RSI Overbought Pullback: ' + currentRSI.toFixed(1) };
  }
  
  return { action: 'HOLD', reason: 'RSI stable at ' + currentRSI.toFixed(1) };
}`,
    winRate: 58.8,
    totalTrades: 312,
    profitPercent: 15.30,
  }
];
