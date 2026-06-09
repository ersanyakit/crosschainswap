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
  remaining_quantity?: string;
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
  asset?: string;
  Asset?: string;
  available?: string | number;
  Available?: string | number;
  locked?: string | number;
  Locked?: string | number;
  pending?: string | number;
  Pending?: string | number;
  frozen?: string | number;
  Frozen?: string | number;
};

export type WithdrawalInfo = {
  id: string;
  asset: string;
  amount: string;
  chain_key: string;
  address: string;
  status: string;
  created_at?: string;
};

export type DepositAddressInfo = {
  user_id: string;
  asset: string;
  chain_key: string;
  chain_id?: number;
  address: string;
  wallet_id?: string;
  label?: string;
  qr_url?: string;
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
  native?: boolean;
  icon_url?: string;
  chain_logo_url?: string;
};

export type AssetInfo = {
  symbol?: string;
  registry_symbol?: string;
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
const API_DECIMAL_PLACES = 8;

export const exchangeConfig = {
  apiBaseURL: stripTrailingSlash(env.VITE_EXCHANGE_API_URL || '/api'),
  wsURL: env.VITE_EXCHANGE_WS_URL || defaultWebsocketURL('/ws/orders'),
  pricesWSURL: env.VITE_EXCHANGE_PRICES_WS_URL || defaultWebsocketURL('/ws/prices'),
  userID: env.VITE_EXCHANGE_USER_ID || 'demo-user',
};

export async function healthCheck(): Promise<boolean> {
  const response = await fetch(`${exchangeConfig.apiBaseURL}/health`, {
    credentials: 'include',
    cache: 'no-store',
  });
  return response.ok;
}

export async function measureAPILatency(): Promise<number> {
  const startedAt = performance.now();
  const response = await fetch(`${exchangeConfig.apiBaseURL}/health?ts=${Date.now()}`, {
    credentials: 'include',
    cache: 'no-store',
  });
  if (!response.ok) {
    throw new Error(`Exchange API error ${response.status}`);
  }
  return Math.max(1, Math.round(performance.now() - startedAt));
}

export async function listMarkets(): Promise<MarketPair[]> {
  const markets = await apiJSON<ApiMarket[]>('/v1/markets');
  return markets
    .filter((item) => item.enabled ?? item.Enabled ?? true)
    .map(mapMarket)
    .filter((item) => item.symbol && item.baseAsset && item.quoteAsset);
}

export async function fetchOrderBook(market: string, depth = 500): Promise<OrderBook> {
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
  const payload = await apiJSON<unknown>(`/v1/users/${encodeURIComponent(userID)}/balances`);
  return unwrapArrayPayload<ApiBalance>(payload, ['balances', 'Balances'])
    .map(mapBalance)
    .filter((item) => item.asset);
}

export async function requestDepositAddress(userID: string, asset: string, chainKey: string): Promise<DepositAddressInfo> {
  const result = await apiJSON<DepositAddressInfo>(`/v1/users/${encodeURIComponent(userID)}/deposit-addresses`, {
    method: 'POST',
    body: JSON.stringify({
      asset,
      chain_key: chainKey,
      label: `${asset.toUpperCase()} ${chainKey}`,
    }),
  });
  return {
    ...result,
    qr_url: result.qr_url ? apiResourceURL(result.qr_url) : depositQRCodeURL(result.address),
  };
}

export function depositQRCodeURL(address: string, size = 300): string {
  const query = new URLSearchParams({ address, size: String(size) });
  return apiResourceURL(`/v1/payment-gateway/qrcode?${query.toString()}`);
}

export async function requestWithdrawal(userID: string, asset: string, chainKey: string, address: string, amount: number): Promise<WithdrawalInfo> {
  return apiJSON<WithdrawalInfo>(`/v1/users/${encodeURIComponent(userID)}/withdrawals`, {
    method: 'POST',
    body: JSON.stringify({
      asset,
      chain_key: chainKey,
      address,
      amount: decimalString(amount),
    }),
  });
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
  const lastPrice = Number(item.last_price || 0);

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
    isFavorite: false,
  };
}

function mapOrder(item: ApiOrder): Order {
  const price = Number(item.price || 0);
  const amount = Number(item.quantity || 0);
  const filled = Number(item.filled_quantity || 0);
  const remaining = Number(item.remaining_quantity ?? Math.max(0, amount - filled));

  return {
    id: item.id,
    symbol: item.market,
    side: item.side.toUpperCase() as OrderSide,
    type: item.type.toUpperCase() as OrderType,
    price,
    amount,
    filled,
    remaining,
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

function mapBalance(item: unknown): AssetBalance {
  const record = isRecord(item) ? item : {};
  const asset = stringField(record, ['asset', 'Asset']).toUpperCase();
  const free = numberField(record, ['available', 'Available']);
  const locked = numberField(record, ['locked', 'Locked']);
  const frozen = numberField(record, ['frozen', 'Frozen', 'pending', 'Pending']);

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

function mapOrderBook(snapshot: ApiOrderBook): OrderBook {
  const bids = mapLevels(snapshot.bids || [], 'bid');
  const asks = mapLevels(snapshot.asks || [], 'ask');
  const bestBid = bids[0]?.price || 0;
  const bestAsk = asks[0]?.price || 0;
  const spread = bestBid > 0 && bestAsk > 0 ? bestAsk - bestBid : 0;
  const spreadPercent = bestAsk > 0 ? (spread / bestAsk) * 100 : 0;

  return {
    market: snapshot.market || '',
    bids,
    asks,
    spread,
    spreadPercent,
  };
}

function mapLevels(levels: ApiPriceLevel[], side: 'bid' | 'ask'): OrderBookLevel[] {
  const mapped = levels.map((level) => {
    const price = Number(level.price || 0);
    const amount = Number(level.quantity || 0);
    return {
      price,
      amount,
      total: price * amount,
      cumulativeAmount: 0,
      cumulativeTotal: 0,
      depthPercent: 0,
    };
  })
    .filter((level) => level.price > 0 && level.amount >= 0.000000005)
    .sort((left, right) => side === 'bid' ? right.price - left.price : left.price - right.price);

  let cumulativeAmount = 0;
  let cumulativeTotal = 0;
  mapped.forEach((level) => {
    cumulativeAmount += level.amount;
    cumulativeTotal += level.total;
    level.cumulativeAmount = cumulativeAmount;
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

function decimalString(value: number): string {
  if (!Number.isFinite(value)) return '0';
  return trimTrailingDecimalZeros(truncateDecimalString(expandDecimalNumber(value), API_DECIMAL_PLACES));
}

function expandDecimalNumber(value: number): string {
  if (!Number.isFinite(value)) return '0';
  const raw = value.toString();
  if (!/[eE]/.test(raw)) return raw;

  const [mantissa, exponentPart] = raw.toLowerCase().split('e');
  const exponent = Number(exponentPart);
  if (!Number.isFinite(exponent)) return '0';

  const sign = mantissa.startsWith('-') ? '-' : '';
  const unsignedMantissa = mantissa.replace('-', '');
  const [integerPart, fractionalPart = ''] = unsignedMantissa.split('.');
  const digits = `${integerPart}${fractionalPart}`;
  const decimalIndex = integerPart.length + exponent;

  if (decimalIndex <= 0) {
    return `${sign}0.${'0'.repeat(Math.abs(decimalIndex))}${digits}`;
  }
  if (decimalIndex >= digits.length) {
    return `${sign}${digits}${'0'.repeat(decimalIndex - digits.length)}`;
  }
  return `${sign}${digits.slice(0, decimalIndex)}.${digits.slice(decimalIndex)}`;
}

function truncateDecimalString(value: string, decimals: number): string {
  const normalized = value.replace(',', '.').trim();
  if (!normalized) return '0';
  const sign = normalized.startsWith('-') ? '-' : '';
  const unsigned = normalized.replace(/^[+-]/, '');
  const [integerPartRaw, fractionalPartRaw = ''] = unsigned.split('.');
  const integerPart = integerPartRaw.replace(/^0+(?=\d)/, '') || '0';
  if (decimals <= 0) return `${sign}${integerPart}`;
  const fractionalPart = fractionalPartRaw.slice(0, decimals);
  return fractionalPart.length > 0 ? `${sign}${integerPart}.${fractionalPart}` : `${sign}${integerPart}`;
}

function trimTrailingDecimalZeros(value: string): string {
  if (!value.includes('.')) return value;
  return value.replace(/0+$/, '').replace(/\.$/, '');
}

function apiResourceURL(path: string): string {
  if (/^https?:\/\//i.test(path)) return path;
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;
  return `${exchangeConfig.apiBaseURL}${normalizedPath}`;
}

type UnknownRecord = Record<string, unknown>;

function unwrapArrayPayload<T>(payload: unknown, keys: string[]): T[] {
  if (Array.isArray(payload)) return payload as T[];
  if (!isRecord(payload)) return [];

  for (const key of [...keys, 'items', 'Items']) {
    const direct = payload[key];
    if (Array.isArray(direct)) return direct as T[];
  }

  for (const wrapperKey of ['data', 'Data', 'result', 'Result', 'payload', 'Payload']) {
    const wrapped = payload[wrapperKey];
    if (Array.isArray(wrapped)) return wrapped as T[];
    if (!isRecord(wrapped)) continue;
    for (const key of [...keys, 'items', 'Items']) {
      const nested = wrapped[key];
      if (Array.isArray(nested)) return nested as T[];
    }
  }

  return [];
}

function isRecord(value: unknown): value is UnknownRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function stringField(record: UnknownRecord, keys: string[]): string {
  for (const key of keys) {
    const value = record[key];
    if (typeof value === 'string') return value.trim();
    if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  }
  return '';
}

function numberField(record: UnknownRecord, keys: string[]): number {
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

function stripTrailingSlash(value: string): string {
  return value.endsWith('/') ? value.slice(0, -1) : value;
}

function defaultWebsocketURL(path: string): string {
  if (typeof window === 'undefined') return `ws://127.0.0.1:8080${path}`;
  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
  return `${protocol}://${window.location.host}${path}`;
}
