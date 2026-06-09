/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

export interface Candle {
  time: number; // timestamp in seconds
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

export type Timeframe = '1m' | '5m' | '15m' | '1h' | '4h' | '1d';

export interface MarketPair {
  symbol: string;
  baseAsset: string; // e.g., BTC
  quoteAsset: string; // e.g., USD
  lastPrice: number;
  change24h: number; // percentage, e.g., +2.45
  high24h: number;
  low24h: number;
  volume24h: number;
  liquidity: number; // In millions USD
  isFavorite?: boolean;
}

export type OrderType = 'LIMIT' | 'MARKET' | 'STOP_LIMIT';
export type OrderSide = 'BUY' | 'SELL';
export type OrderStatus = 'OPEN' | 'PENDING' | 'PARTIALLY_FILLED' | 'FILLED' | 'CANCELLED' | 'EXPIRED' | 'REJECTED';

export interface Order {
  id: string;
  symbol: string;
  side: OrderSide;
  type: OrderType;
  price: number;
  amount: number;
  filled: number;
  remaining: number;
  total: number;
  stopPrice?: number;
  status: OrderStatus;
  timestamp: Date;
}

export interface Trade {
  id: string;
  symbol: string;
  price: number;
  amount: number;
  total: number;
  side: OrderSide;
  timestamp: Date;
}

export interface OrderBookLevel {
  price: number;
  amount: number;
  total: number;
  cumulativeAmount: number;
  cumulativeTotal: number;
  depthPercent: number; // for bar representation
}

export interface OrderBook {
  market: string;
  bids: OrderBookLevel[];
  asks: OrderBookLevel[];
  spread: number;
  spreadPercent: number;
}

export interface AssetBalance {
  asset: string;
  name: string;
  free: number;
  locked: number;
  frozen: number;
  valueUsd: number;
  change24h: number;
}

export interface SystemLog {
  id: string;
  timestamp: Date;
  type: 'INFO' | 'SUCCESS' | 'WARNING' | 'ERROR';
  source: 'SYSTEM' | 'ORDER' | 'WEBSOCKET' | 'STRATEGY';
  message: string;
}

export interface TradingStrategy {
  id: string;
  name: string;
  status: 'IDLE' | 'RUNNING' | 'PAUSED';
  code: string;
  lastExecution?: Date;
  winRate: number;
  totalTrades: number;
  profitPercent: number;
}
