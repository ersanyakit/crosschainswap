import {
  AssetBalance,
  Candle,
  MarketPair,
  Order,
  OrderBook,
  OrderBookLevel,
  OrderSide,
  OrderType,
  Trade,
} from '../types/trading';

type ApiMarket = {
  symbol?: string;
  Symbol?: string;
  base_asset?: string;
  BaseAsset?: string;
  quote_asset?: string;
  QuoteAsset?: string;
  enabled?: boolean;
  Enabled?: boolean;
  last_price?: string;
  change_24h?: string;
  high_24h?: string;
  low_24h?: string;
  volume_24h?: string;
  liquidity?: string;
};

type ApiOrder = {
  id: string;
  market: string;
  side: string;
  type: string;
  status: string;
  price: string;
  stop_price?: string;
  quantity: string;
  filled_quantity: string;
  created_at: string;
};

type ApiTrade = {
  id: string;
  market: string;
  taker_side: string;
  price: string;
  quantity: string;
  quote_quantity: string;
  created_at: string;
};

type ApiPriceLevel = {
  price: string;
  quantity: string;
};

type ApiOrderBook = {
  market: string;
  bids: ApiPriceLevel[];
  asks: ApiPriceLevel[];
};

type ApiCandle = {
  open_time: string;
  open: string;
  high: string;
  low: string;
  close: string;
  volume_base: string;
};

type ApiBalance = {
  asset: string;
  available: string;
  locked: string;
  pending: string;
};

export type AssetDeploymentInfo = {
  chain_key?: string;
  asset_id?: string;
  address?: string;
  mint?: string;
  symbol?: string;
  name?: string;
  decimals?: number;
  enabled?: boolean;
  icon_url?: string;
};

export type AssetInfo = {
  symbol?: string;
  name?: string;
  type?: string;
  decimals?: number;
  icon_url?: string;
  deployments?: AssetDeploymentInfo[];
};

export type DexPoolPrice = {
  chain_key: string;
  venue_key: string;
  pool_id: string;
  base_symbol: string;
  base_asset_id: string;
  quote_symbol: string;
  quote_asset_id: string;
  price: string;
  inverse_price?: string;
  base_price_usdc?: string;
  price_usdc?: string;
  quote_price_usdc?: string;
  usdc_route?: {
    chain_key?: string;
    venue_key?: string;
    pool_id?: string;
    base_symbol?: string;
    quote_symbol?: string;
    price?: string;
    inverse_price?: string;
  };
  base_asset?: AssetDeploymentInfo;
  quote_asset?: AssetDeploymentInfo;
  reserve_base: string;
  reserve_quote: string;
  pool_kind: string;
};

export type AssetPriceResponse = {
  symbol: string;
  asset?: AssetInfo;
  prices: DexPoolPrice[];
};

type MatchResult = {
  order: ApiOrder;
  trades?: ApiTrade[];
};

type PlaceOrderInput = {
  market: string;
  userID: string;
  side: OrderSide;
  type: OrderType;
  price: number;
  amount: number;
  stopPrice?: number;
};

const env = import.meta.env as Record<string, string | undefined>;

export const exchangeConfig = {
  apiBaseURL: stripTrailingSlash(env.VITE_EXCHANGE_API_URL || '/api'),
  wsURL: env.VITE_EXCHANGE_WS_URL || defaultWebsocketURL('/ws/orders'),
  pricesWSURL: env.VITE_EXCHANGE_PRICES_WS_URL || defaultWebsocketURL('/ws/prices'),
  userID: env.VITE_EXCHANGE_USER_ID || 'demo-user',
};

export async function healthCheck(): Promise<boolean> {
  const response = await fetch(`${exchangeConfig.apiBaseURL}/health`, {
    credentials: 'include',
  });
  return response.ok;
}

export async function listMarkets(): Promise<MarketPair[]> {
  const markets = await apiJSON<ApiMarket[]>('/v1/markets');
  return markets
    .filter((item) => item.enabled ?? item.Enabled ?? true)
    .map(mapMarket)
    .filter((item) => item.symbol && item.baseAsset && item.quoteAsset);
}

export async function fetchOrderBook(market: string, depth = 50): Promise<OrderBook> {
  const query = new URLSearchParams({ market, depth: String(depth) });
  const snapshot = await apiJSON<ApiOrderBook>(`/v1/orderbook?${query.toString()}`);
  return mapOrderBook(snapshot);
}

export async function fetchCandles(market: string, interval: string, limit = 120): Promise<Candle[]> {
  const query = new URLSearchParams({ market, interval, limit: String(limit) });
  const candles = await apiJSON<ApiCandle[]>(
    `/v1/markets/candles?${query.toString()}`
  );
  return candles.map(mapCandle);
}

export async function fetchMarketTrades(market: string, limit = 80): Promise<Trade[]> {
  const query = new URLSearchParams({ market, limit: String(limit) });
  const trades = await apiJSON<ApiTrade[]>(
    `/v1/markets/trades?${query.toString()}`
  );
  return trades.map(mapTrade);
}

export async function fetchUserOrders(userID: string, market?: string, limit = 100): Promise<Order[]> {
  const query = new URLSearchParams({ limit: String(limit) });
  if (market) query.set('market', market);
  const orders = await apiJSON<ApiOrder[]>(
    `/v1/users/${encodeURIComponent(userID)}/orders?${query.toString()}`
  );
  return orders.map(mapOrder);
}

export async function fetchUserTrades(userID: string, market?: string, limit = 100): Promise<Trade[]> {
  const query = new URLSearchParams({ limit: String(limit) });
  if (market) query.set('market', market);
  const trades = await apiJSON<ApiTrade[]>(
    `/v1/users/${encodeURIComponent(userID)}/trades?${query.toString()}`
  );
  return trades.map(mapTrade);
}

export async function fetchBalances(userID: string): Promise<AssetBalance[]> {
  const balances = await apiJSON<ApiBalance[]>(`/v1/users/${encodeURIComponent(userID)}/balances`);
  return balances.map(mapBalance);
}

export async function fetchAssetPrices(symbol: string): Promise<AssetPriceResponse> {
  return apiJSON<AssetPriceResponse>(`/v1/prices/${encodeURIComponent(symbol)}`);
}

export async function fetchAssets(): Promise<AssetInfo[]> {
  return apiJSON<AssetInfo[]>('/v1/assets');
}

export async function placeOrder(input: PlaceOrderInput): Promise<{ order: Order; trades: Trade[] }> {
  const clientOrderID = `ui-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
  const result = await apiJSON<MatchResult>('/v1/orders', {
    method: 'POST',
    body: JSON.stringify({
      client_order_id: clientOrderID,
      user_id: input.userID,
      market: input.market,
      side: input.side.toLowerCase(),
      type: input.type.toLowerCase(),
      time_in_force: input.type === 'MARKET' ? 'ioc' : 'gtc',
      price: decimalString(input.price),
      stop_price: input.stopPrice ? decimalString(input.stopPrice) : undefined,
      quantity: decimalString(input.amount),
    }),
  });

  return {
    order: mapOrder(result.order),
    trades: (result.trades || []).map(mapTrade),
  };
}

export async function cancelOrder(orderID: string, userID: string): Promise<Order> {
  const query = new URLSearchParams({ user_id: userID });
  const order = await apiJSON<ApiOrder>(`/v1/orders/${encodeURIComponent(orderID)}?${query.toString()}`, {
    method: 'DELETE',
  });
  return mapOrder(order);
}

export function openExchangeSocket(onMessage: (event: any) => void): WebSocket {
  const socket = new WebSocket(exchangeConfig.wsURL);
  socket.onmessage = (message) => {
    try {
      onMessage(JSON.parse(message.data));
    } catch {
      // Ignore malformed websocket payloads; REST polling remains authoritative.
    }
  };
  return socket;
}

export function openPriceSocket(onMessage: (event: any) => void): WebSocket {
  const socket = new WebSocket(exchangeConfig.pricesWSURL);
  socket.onmessage = (message) => {
    try {
      onMessage(JSON.parse(message.data));
    } catch {
      // Ignore malformed websocket payloads; REST polling remains authoritative.
    }
  };
  return socket;
}

async function apiJSON<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(`${exchangeConfig.apiBaseURL}${path}`, {
    ...init,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init.headers || {}),
    },
  });

  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    throw new Error(payload.error || `Exchange API error ${response.status}`);
  }

  return response.json() as Promise<T>;
}

function mapMarket(item: ApiMarket): MarketPair {
  const symbol = item.symbol || item.Symbol || '';
  const baseAsset = item.base_asset || item.BaseAsset || symbol.split('/')[0] || '';
  const quoteAsset = item.quote_asset || item.QuoteAsset || symbol.split('/')[1] || '';
  const fallbackPrice = seedPrice(symbol, baseAsset);
  const lastPrice = Number(item.last_price || fallbackPrice);

  return {
    symbol,
    baseAsset,
    quoteAsset,
    lastPrice,
    change24h: Number(item.change_24h || 0),
    high24h: Number(item.high_24h || lastPrice),
    low24h: Number(item.low_24h || lastPrice),
    volume24h: Number(item.volume_24h || 0),
    liquidity: Number(item.liquidity || 0) / 1_000_000,
    isFavorite: ['PEPPER/USD', 'CHZ/USD', 'SOL/USD'].includes(symbol),
  };
}

function mapOrder(item: ApiOrder): Order {
  const price = Number(item.price || 0);
  const amount = Number(item.quantity || 0);
  const filled = Number(item.filled_quantity || 0);

  return {
    id: item.id,
    symbol: item.market,
    side: item.side.toUpperCase() as OrderSide,
    type: item.type.toUpperCase() as OrderType,
    price,
    amount,
    filled,
    total: price * amount,
    stopPrice: item.stop_price ? Number(item.stop_price) : undefined,
    status: mapOrderStatus(item.status),
    timestamp: item.created_at ? new Date(item.created_at) : new Date(),
  };
}

function mapTrade(item: ApiTrade): Trade {
  const price = Number(item.price || 0);
  const amount = Number(item.quantity || 0);

  return {
    id: item.id,
    symbol: item.market,
    price,
    amount,
    total: Number(item.quote_quantity || price * amount),
    side: item.taker_side.toUpperCase() as OrderSide,
    timestamp: item.created_at ? new Date(item.created_at) : new Date(),
  };
}

function mapBalance(item: ApiBalance): AssetBalance {
  const free = Number(item.available || 0);
  const locked = Number(item.locked || 0);

  return {
    asset: item.asset,
    name: item.asset,
    free,
    locked,
    valueUsd: free + locked,
    change24h: 0,
  };
}

function mapOrderBook(snapshot: ApiOrderBook): OrderBook {
  const bids = mapLevels(snapshot.bids || []);
  const asks = mapLevels(snapshot.asks || []);
  const bestBid = bids[0]?.price || 0;
  const bestAsk = asks[0]?.price || 0;
  const spread = bestBid > 0 && bestAsk > 0 ? bestAsk - bestBid : 0;
  const spreadPercent = bestAsk > 0 ? (spread / bestAsk) * 100 : 0;

  return {
    bids,
    asks,
    spread,
    spreadPercent,
  };
}

function mapLevels(levels: ApiPriceLevel[]): OrderBookLevel[] {
  const mapped = levels.map((level) => {
    const price = Number(level.price || 0);
    const amount = Number(level.quantity || 0);
    return {
      price,
      amount,
      total: price * amount,
      cumulativeTotal: 0,
      depthPercent: 0,
    };
  });

  let cumulativeTotal = 0;
  mapped.forEach((level) => {
    cumulativeTotal += level.total;
    level.cumulativeTotal = cumulativeTotal;
  });

  const maxTotal = mapped[mapped.length - 1]?.cumulativeTotal || 1;
  mapped.forEach((level) => {
    level.depthPercent = (level.cumulativeTotal / maxTotal) * 100;
  });

  return mapped;
}

function mapCandle(item: ApiCandle): Candle {
  return {
    time: Math.floor(new Date(item.open_time).getTime() / 1000),
    open: Number(item.open || 0),
    high: Number(item.high || 0),
    low: Number(item.low || 0),
    close: Number(item.close || 0),
    volume: Number(item.volume_base || 0),
  };
}

function mapOrderStatus(status: string): Order['status'] {
  switch (status) {
    case 'filled':
      return 'FILLED';
    case 'canceled':
    case 'expired':
    case 'rejected':
      return 'CANCELLED';
    default:
      return 'PENDING';
  }
}

function seedPrice(symbol: string, baseAsset: string): number {
  if (symbol === 'PEPPER/USD') return 0.000000001;
  if (baseAsset === 'CHZ') return 0.08;
  if (baseAsset === 'SOL') return 184.25;
  if (baseAsset === 'AVAX') return 32.4;
  if (baseAsset === 'ETH') return 3412.8;
  return 1;
}

function decimalString(value: number): string {
  if (!Number.isFinite(value)) return '0';
  return value.toLocaleString('en-US', {
    useGrouping: false,
    maximumFractionDigits: 18,
  });
}

function stripTrailingSlash(value: string): string {
  return value.endsWith('/') ? value.slice(0, -1) : value;
}

function defaultWebsocketURL(path: string): string {
  if (typeof window === 'undefined') return `ws://127.0.0.1:8080${path}`;
  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
  return `${protocol}://${window.location.host}${path}`;
}
