package rest

import "github.com/gofiber/fiber/v3"

const openAPISpec = `{
  "openapi": "3.0.3",
  "info": {
    "title": "Exchange API",
    "version": "1.0.0",
    "description": "Exchange order, market data, balance, deposit, withdrawal, swap and websocket API."
  },
  "servers": [
    { "url": "http://localhost:8080" }
  ],
  "tags": [
    { "name": "Auth" },
    { "name": "Orders" },
    { "name": "Market Data" },
    { "name": "Balances" },
    { "name": "Deposits" },
    { "name": "Withdrawals" },
    { "name": "Wallets" },
    { "name": "Swaps" },
    { "name": "Prices" },
    { "name": "Websocket" }
  ],
  "paths": {
    "/health": {
      "get": {
        "summary": "Health check",
        "responses": { "200": { "description": "OK" } }
      }
    },
    "/auth/oidc/status": {
      "get": {
        "tags": ["Auth"],
        "summary": "OIDC configuration status",
        "responses": { "200": { "description": "OIDC status" } }
      }
    },
    "/auth/oidc/login": {
      "get": {
        "tags": ["Auth"],
        "summary": "Start OIDC login",
        "responses": { "303": { "description": "Redirect to OIDC provider" }, "503": { "$ref": "#/components/responses/Error" } }
      }
    },
    "/auth/oidc/callback": {
      "get": {
        "tags": ["Auth"],
        "summary": "OIDC authorization callback",
        "parameters": [
          { "name": "code", "in": "query", "schema": { "type": "string" } },
          { "name": "state", "in": "query", "schema": { "type": "string" } }
        ],
        "responses": { "200": { "description": "Authenticated" }, "401": { "$ref": "#/components/responses/Error" } }
      }
    },
    "/auth/me": {
      "get": {
        "tags": ["Auth"],
        "summary": "Current authenticated OIDC user",
        "security": [{ "oidcSession": [] }],
        "responses": { "200": { "description": "Current user" }, "401": { "$ref": "#/components/responses/Error" } }
      }
    },
    "/auth/logout": {
      "post": {
        "tags": ["Auth"],
        "summary": "Clear OIDC session cookie",
        "responses": { "200": { "description": "Logged out" } }
      }
    },
    "/v1/markets": {
      "get": {
        "tags": ["Market Data"],
        "summary": "List enabled exchange markets",
        "responses": {
          "200": { "description": "Markets", "content": { "application/json": { "schema": { "type": "array", "items": { "$ref": "#/components/schemas/Market" } } } } }
        }
      }
    },
    "/v1/assets": {
      "get": {
        "tags": ["Prices"],
        "summary": "List registry assets with deployment metadata and icons",
        "responses": {
          "200": { "description": "Assets", "content": { "application/json": { "schema": { "type": "array", "items": { "$ref": "#/components/schemas/AssetInfo" } } } } }
        }
      }
    },
    "/v1/orders": {
      "post": {
        "tags": ["Orders"],
        "summary": "Place a limit, market or stop-limit order",
        "security": [{ "oidcSession": [] }],
        "requestBody": {
          "required": true,
          "content": { "application/json": { "schema": { "$ref": "#/components/schemas/PlaceOrderRequest" } } }
        },
        "responses": {
          "201": { "description": "Order accepted", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/MatchResult" } } } },
          "400": { "$ref": "#/components/responses/Error" },
          "422": { "$ref": "#/components/responses/Error" }
        }
      }
    },
    "/v1/orders/{id}": {
      "get": {
        "tags": ["Orders"],
        "summary": "Get order by id",
        "security": [{ "oidcSession": [] }],
        "parameters": [{ "$ref": "#/components/parameters/OrderID" }],
        "responses": {
          "200": { "description": "Order", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Order" } } } },
          "404": { "$ref": "#/components/responses/Error" }
        }
      },
      "delete": {
        "tags": ["Orders"],
        "summary": "Cancel order",
        "security": [{ "oidcSession": [] }],
        "parameters": [
          { "$ref": "#/components/parameters/OrderID" },
          { "name": "user_id", "in": "query", "schema": { "type": "string" }, "description": "Optional owner check." }
        ],
        "responses": {
          "200": { "description": "Canceled order", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Order" } } } },
          "404": { "$ref": "#/components/responses/Error" },
          "422": { "$ref": "#/components/responses/Error" }
        }
      }
    },
    "/v1/orders/triggers": {
      "post": {
        "tags": ["Orders"],
        "summary": "Manually trigger stop-limit orders for a market",
        "requestBody": { "required": true, "content": { "application/json": { "schema": { "$ref": "#/components/schemas/TriggerStopsRequest" } } } },
        "responses": { "200": { "description": "Triggered orders", "content": { "application/json": { "schema": { "type": "array", "items": { "$ref": "#/components/schemas/MatchResult" } } } } } }
      }
    },
    "/v1/users/{user_id}/orders": {
      "get": {
        "tags": ["Orders"],
        "summary": "User order history",
        "security": [{ "oidcSession": [] }],
        "parameters": [
          { "$ref": "#/components/parameters/UserID" },
          { "$ref": "#/components/parameters/MarketQuery" },
          { "name": "status", "in": "query", "schema": { "type": "string", "enum": ["pending_stop", "open", "partially_filled", "filled", "canceled", "rejected"] } },
          { "$ref": "#/components/parameters/Limit" }
        ],
        "responses": { "200": { "description": "Orders", "content": { "application/json": { "schema": { "type": "array", "items": { "$ref": "#/components/schemas/Order" } } } } } }
      }
    },
    "/v1/users/{user_id}/trades": {
      "get": {
        "tags": ["Orders"],
        "summary": "User trade history",
        "security": [{ "oidcSession": [] }],
        "parameters": [
          { "$ref": "#/components/parameters/UserID" },
          { "$ref": "#/components/parameters/MarketQuery" },
          { "$ref": "#/components/parameters/Limit" }
        ],
        "responses": { "200": { "description": "Trades", "content": { "application/json": { "schema": { "type": "array", "items": { "$ref": "#/components/schemas/Trade" } } } } } }
      }
    },
    "/v1/orderbook/{market}": {
      "get": {
        "tags": ["Market Data"],
        "summary": "Order book snapshot",
        "parameters": [
          { "$ref": "#/components/parameters/MarketPath" },
          { "$ref": "#/components/parameters/Depth" }
        ],
        "responses": { "200": { "description": "Order book", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/OrderBook" } } } } }
      }
    },
    "/v1/markets/{market}/trades": {
      "get": {
        "tags": ["Market Data"],
        "summary": "Recent market trades",
        "parameters": [
          { "$ref": "#/components/parameters/MarketPath" },
          { "$ref": "#/components/parameters/Limit" }
        ],
        "responses": { "200": { "description": "Trades", "content": { "application/json": { "schema": { "type": "array", "items": { "$ref": "#/components/schemas/Trade" } } } } } }
      }
    },
    "/v1/markets/{market}/candles": {
      "get": {
        "tags": ["Market Data"],
        "summary": "OHLC candles",
        "parameters": [
          { "$ref": "#/components/parameters/MarketPath" },
          { "name": "interval", "in": "query", "schema": { "type": "string", "enum": ["1m", "5m", "15m", "1h", "4h", "1d"], "default": "1m" } },
          { "$ref": "#/components/parameters/Limit" }
        ],
        "responses": { "200": { "description": "Candles", "content": { "application/json": { "schema": { "type": "array", "items": { "$ref": "#/components/schemas/Candle" } } } } } }
      }
    },
    "/v1/users/{user_id}/balances": {
      "get": {
        "tags": ["Balances"],
        "summary": "List user balances",
        "security": [{ "oidcSession": [] }],
        "parameters": [{ "$ref": "#/components/parameters/UserID" }],
        "responses": { "200": { "description": "Balances", "content": { "application/json": { "schema": { "type": "array", "items": { "$ref": "#/components/schemas/Balance" } } } } } }
      }
    },
    "/v1/users/{user_id}/deposits/pending": {
      "post": {
        "tags": ["Deposits"],
        "summary": "Mark deposit as pending",
        "parameters": [{ "$ref": "#/components/parameters/UserID" }, { "$ref": "#/components/parameters/GatewaySecret" }],
        "requestBody": { "required": true, "content": { "application/json": { "schema": { "$ref": "#/components/schemas/BalanceAmountRequest" } } } },
        "responses": { "200": { "description": "Updated balance", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Balance" } } } } }
      }
    },
    "/v1/users/{user_id}/deposits/settle": {
      "post": {
        "tags": ["Deposits"],
        "summary": "Settle pending deposit into available balance",
        "parameters": [{ "$ref": "#/components/parameters/UserID" }, { "$ref": "#/components/parameters/GatewaySecret" }],
        "requestBody": { "required": true, "content": { "application/json": { "schema": { "$ref": "#/components/schemas/BalanceAmountRequest" } } } },
        "responses": { "200": { "description": "Updated balance", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Balance" } } } } }
      }
    },
    "/v1/users/{user_id}/withdrawals": {
      "get": {
        "tags": ["Withdrawals"],
        "summary": "List user withdrawals",
        "security": [{ "oidcSession": [] }],
        "parameters": [{ "$ref": "#/components/parameters/UserID" }, { "$ref": "#/components/parameters/Limit" }],
        "responses": { "200": { "description": "Withdrawals", "content": { "application/json": { "schema": { "type": "array", "items": { "$ref": "#/components/schemas/Withdrawal" } } } } } }
      },
      "post": {
        "tags": ["Withdrawals"],
        "summary": "Request withdrawal",
        "security": [{ "oidcSession": [] }],
        "parameters": [{ "$ref": "#/components/parameters/UserID" }],
        "requestBody": { "required": true, "content": { "application/json": { "schema": { "$ref": "#/components/schemas/WithdrawalRequest" } } } },
        "responses": { "201": { "description": "Withdrawal requested", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Withdrawal" } } } } }
      }
    },
    "/v1/withdrawals/{id}/complete": {
      "post": {
        "tags": ["Withdrawals"],
        "summary": "Complete withdrawal",
        "parameters": [{ "$ref": "#/components/parameters/WithdrawalID" }, { "$ref": "#/components/parameters/GatewaySecret" }],
        "responses": { "200": { "description": "Completed withdrawal", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Withdrawal" } } } } }
      }
    },
    "/v1/withdrawals/{id}/cancel": {
      "post": {
        "tags": ["Withdrawals"],
        "summary": "Cancel withdrawal and return pending amount to available balance",
        "parameters": [{ "$ref": "#/components/parameters/WithdrawalID" }, { "$ref": "#/components/parameters/GatewaySecret" }],
        "responses": { "200": { "description": "Canceled withdrawal", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Withdrawal" } } } } }
      }
    },
    "/v1/users/{user_id}/wallets": {
      "get": {
        "tags": ["Wallets"],
        "summary": "List gateway-registered wallets",
        "security": [{ "oidcSession": [] }],
        "parameters": [{ "$ref": "#/components/parameters/UserID" }],
        "responses": { "200": { "description": "Wallets", "content": { "application/json": { "schema": { "type": "array", "items": { "$ref": "#/components/schemas/Wallet" } } } } } }
      },
      "put": {
        "tags": ["Wallets"],
        "summary": "Register gateway-generated wallet address",
        "parameters": [{ "$ref": "#/components/parameters/UserID" }, { "$ref": "#/components/parameters/GatewaySecret" }],
        "requestBody": { "required": true, "content": { "application/json": { "schema": { "$ref": "#/components/schemas/WalletRequest" } } } },
        "responses": { "200": { "description": "Wallet", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Wallet" } } } } }
      }
    },
    "/v1/prices/{symbol}": {
      "get": {
        "tags": ["Prices"],
        "summary": "Get asset prices across chains and venues",
        "parameters": [{ "name": "symbol", "in": "path", "required": true, "schema": { "type": "string" } }],
        "responses": { "200": { "description": "Asset prices" } }
      }
    },
    "/v1/swaps/quote": {
      "post": { "tags": ["Swaps"], "summary": "Build swap quote", "responses": { "200": { "description": "Quote" } } }
    },
    "/v1/swaps/transaction": {
      "post": { "tags": ["Swaps"], "summary": "Build unsigned swap transaction", "responses": { "200": { "description": "Unsigned transaction" } } }
    },
    "/v1/swaps/approve": {
      "post": { "tags": ["Swaps"], "summary": "Build ERC20 approve transaction", "responses": { "200": { "description": "Approve transaction" } } }
    },
    "/ws/orders": {
      "get": {
        "tags": ["Websocket"],
        "summary": "Order, trade, deposit, withdrawal and wallet websocket stream",
        "description": "Websocket endpoint. Emits exchange.order_accepted, exchange.order_updated, exchange.order_filled, exchange.order_expired, exchange.order_canceled, exchange.trades_created, exchange.orderbook_updated, exchange.deposit_pending, exchange.deposit_settled, exchange.withdrawal_requested, exchange.withdrawal_completed, exchange.withdrawal_canceled and exchange.wallet_registered.",
        "responses": { "101": { "description": "Switching Protocols" } }
      }
    },
    "/ws/prices": {
      "get": {
        "tags": ["Websocket"],
        "summary": "Price websocket stream",
        "responses": { "101": { "description": "Switching Protocols" } }
      }
    }
  },
  "components": {
    "parameters": {
      "OrderID": { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } },
      "WithdrawalID": { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } },
      "UserID": { "name": "user_id", "in": "path", "required": true, "schema": { "type": "string" } },
      "MarketPath": { "name": "market", "in": "path", "required": true, "schema": { "type": "string", "example": "PEPPER/USD" } },
      "MarketQuery": { "name": "market", "in": "query", "schema": { "type": "string", "example": "PEPPER/USD" } },
      "Limit": { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 100 } },
      "Depth": { "name": "depth", "in": "query", "schema": { "type": "integer", "default": 100 } },
      "GatewaySecret": { "name": "X-Gateway-Secret", "in": "header", "schema": { "type": "string" }, "description": "Required when PAYMENT_GATEWAY_SECRET is set." }
    },
    "securitySchemes": {
      "oidcSession": { "type": "apiKey", "in": "cookie", "name": "exchange_session" }
    },
    "responses": {
      "Error": { "description": "Error", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Error" } } } }
    },
    "schemas": {
      "Error": { "type": "object", "properties": { "error": { "type": "string" } } },
      "Market": {
        "type": "object",
        "properties": {
          "symbol": { "type": "string", "example": "PEPPER/USD" },
          "base_asset": { "type": "string", "example": "PEPPER" },
          "quote_asset": { "type": "string", "example": "USD" },
          "enabled": { "type": "boolean" },
          "last_price": { "type": "string", "example": "0.000000001" },
          "change_24h": { "type": "string", "example": "0" },
          "high_24h": { "type": "string", "example": "0.0000000012" },
          "low_24h": { "type": "string", "example": "0.0000000008" },
          "volume_24h": { "type": "string", "example": "1000000" },
          "liquidity": { "type": "string", "example": "125000.25" }
        }
      },
      "AssetInfo": {
        "type": "object",
        "properties": {
          "symbol": { "type": "string", "example": "PEPPER" },
          "name": { "type": "string", "example": "PEPPER" },
          "type": { "type": "string", "example": "token" },
          "decimals": { "type": "integer", "example": 18 },
          "icon_url": { "type": "string", "example": "https://s2.coinmarketcap.com/static/img/coins/64x64/33603.png" },
          "deployments": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "chain_key": { "type": "string", "example": "chiliz" },
                "asset_id": { "type": "string" },
                "address": { "type": "string" },
                "mint": { "type": "string" },
                "symbol": { "type": "string" },
                "name": { "type": "string" },
                "decimals": { "type": "integer" },
                "enabled": { "type": "boolean" },
                "icon_url": { "type": "string" }
              }
            }
          }
        }
      },
      "PlaceOrderRequest": {
        "type": "object",
        "required": ["client_order_id", "user_id", "market", "side", "price", "quantity"],
        "properties": {
          "client_order_id": { "type": "string", "example": "client-1" },
          "user_id": { "type": "string", "example": "user-a" },
          "market": { "type": "string", "example": "PEPPER/USD" },
          "side": { "type": "string", "enum": ["buy", "sell"] },
          "type": { "type": "string", "enum": ["limit", "market", "stop_limit"], "default": "limit" },
          "time_in_force": { "type": "string", "enum": ["gtc", "ioc"], "default": "gtc" },
          "price": { "type": "string", "example": "0.000000001", "description": "Limit price. For market orders this is the mandatory protection price: max price for buy, min price for sell." },
          "stop_price": { "type": "string", "example": "0.0000000009" },
          "quantity": { "type": "string", "example": "1000000" }
        }
      },
      "TriggerStopsRequest": {
        "type": "object",
        "required": ["market", "last_price"],
        "properties": {
          "market": { "type": "string", "example": "PEPPER/USD" },
          "last_price": { "type": "string", "example": "0.000000001" }
        }
      },
      "Order": {
        "type": "object",
        "properties": {
          "id": { "type": "string" },
          "client_order_id": { "type": "string" },
          "user_id": { "type": "string" },
          "market": { "type": "string" },
          "base_asset": { "type": "string" },
          "quote_asset": { "type": "string" },
          "side": { "type": "string" },
          "type": { "type": "string" },
          "status": { "type": "string" },
          "time_in_force": { "type": "string" },
          "price": { "type": "string" },
          "stop_price": { "type": "string" },
          "quantity": { "type": "string" },
          "filled_quantity": { "type": "string" },
          "remaining_quantity": { "type": "string" },
          "created_at": { "type": "string", "format": "date-time" },
          "updated_at": { "type": "string", "format": "date-time" }
        }
      },
      "Trade": {
        "type": "object",
        "properties": {
          "id": { "type": "string" },
          "market": { "type": "string" },
          "maker_order_id": { "type": "string" },
          "taker_order_id": { "type": "string" },
          "maker_user_id": { "type": "string" },
          "taker_user_id": { "type": "string" },
          "taker_side": { "type": "string" },
          "price": { "type": "string" },
          "quantity": { "type": "string" },
          "quote_quantity": { "type": "string" },
          "created_at": { "type": "string", "format": "date-time" }
        }
      },
      "MatchResult": {
        "type": "object",
        "properties": {
          "order": { "$ref": "#/components/schemas/Order" },
          "trades": { "type": "array", "items": { "$ref": "#/components/schemas/Trade" } }
        }
      },
      "OrderBook": {
        "type": "object",
        "properties": {
          "market": { "type": "string" },
          "bids": { "type": "array", "items": { "$ref": "#/components/schemas/PriceLevel" } },
          "asks": { "type": "array", "items": { "$ref": "#/components/schemas/PriceLevel" } }
        }
      },
      "PriceLevel": {
        "type": "object",
        "properties": {
          "market": { "type": "string" },
          "side": { "type": "string" },
          "price": { "type": "string" },
          "quantity": { "type": "string" },
          "order_count": { "type": "integer" }
        }
      },
      "Candle": {
        "type": "object",
        "properties": {
          "market": { "type": "string" },
          "interval": { "type": "string" },
          "open_time": { "type": "string", "format": "date-time" },
          "close_time": { "type": "string", "format": "date-time" },
          "open": { "type": "string" },
          "high": { "type": "string" },
          "low": { "type": "string" },
          "close": { "type": "string" },
          "volume_base": { "type": "string" },
          "volume_quote": { "type": "string" },
          "trade_count": { "type": "integer" }
        }
      },
      "Balance": {
        "type": "object",
        "properties": {
          "user_id": { "type": "string" },
          "asset": { "type": "string" },
          "available": { "type": "string" },
          "locked": { "type": "string" },
          "pending": { "type": "string" },
          "updated_at": { "type": "string", "format": "date-time" }
        }
      },
      "BalanceAmountRequest": {
        "type": "object",
        "required": ["asset", "amount"],
        "properties": {
          "asset": { "type": "string", "example": "USDC" },
          "amount": { "type": "string", "example": "100" }
        }
      },
      "WithdrawalRequest": {
        "type": "object",
        "required": ["asset", "amount", "chain_key", "address"],
        "properties": {
          "asset": { "type": "string", "example": "USDC" },
          "amount": { "type": "string", "example": "25" },
          "chain_key": { "type": "string", "example": "chiliz" },
          "address": { "type": "string", "example": "0xDestinationAddress" }
        }
      },
      "Withdrawal": {
        "type": "object",
        "properties": {
          "id": { "type": "string" },
          "user_id": { "type": "string" },
          "asset": { "type": "string" },
          "amount": { "type": "string" },
          "chain_key": { "type": "string" },
          "address": { "type": "string" },
          "status": { "type": "string", "enum": ["requested", "completed", "canceled"] },
          "created_at": { "type": "string", "format": "date-time" },
          "updated_at": { "type": "string", "format": "date-time" }
        }
      },
      "WalletRequest": {
        "type": "object",
        "required": ["chain_key", "address"],
        "properties": {
          "chain_key": { "type": "string", "example": "chiliz" },
          "address": { "type": "string", "example": "0xGatewayGeneratedDepositAddress" }
        }
      },
      "Wallet": {
        "type": "object",
        "properties": {
          "user_id": { "type": "string" },
          "chain_key": { "type": "string" },
          "address": { "type": "string" },
          "created_at": { "type": "string", "format": "date-time" },
          "updated_at": { "type": "string", "format": "date-time" }
        }
      }
    }
  }
}`

const swaggerHTML = `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <title>Exchange API Swagger</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function () {
      SwaggerUIBundle({ url: "/swagger.json", dom_id: "#swagger-ui" });
    };
  </script>
</body>
</html>`

func (s *Server) swagger(c fiber.Ctx) error {
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(swaggerHTML)
}

func (s *Server) openapi(c fiber.Ctx) error {
	c.Set("Content-Type", "application/json; charset=utf-8")
	return c.SendString(openAPISpec)
}
