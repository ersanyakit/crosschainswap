package rest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	apporders "exchange/internal/app/orders"
	"exchange/internal/core/decimal"
	"exchange/internal/core/order"
	"exchange/internal/core/trade"

	"github.com/gofiber/fiber/v3"
)

type apiCredential struct {
	UserID string
	Key    string
	Secret string
}

func (s *Server) binanceAccount(c fiber.Ctx) error {
	userID, err := s.requireBinanceAPIUser(c)
	if err != nil {
		return err
	}
	balances, err := s.orders.ListBalances(c.Context(), userID)
	if err != nil {
		return orderError(c, err)
	}
	out := make([]fiber.Map, 0, len(balances))
	for _, item := range balances {
		out = append(out, fiber.Map{
			"asset":  item.Asset,
			"free":   item.Available,
			"locked": item.Locked,
		})
	}
	now := time.Now().UTC().UnixMilli()
	return okJSON(c, fiber.Map{
		"makerCommission":  8,
		"takerCommission":  10,
		"buyerCommission":  0,
		"sellerCommission": 0,
		"canTrade":         true,
		"canWithdraw":      true,
		"canDeposit":       true,
		"updateTime":       now,
		"accountType":      "SPOT",
		"balances":         out,
		"permissions":      []string{"SPOT"},
	})
}

func (s *Server) binancePlaceOrder(c fiber.Ctx) error {
	userID, err := s.requireBinanceAPIUser(c)
	if err != nil {
		return err
	}
	req, err := s.binanceOrderRequest(c, userID)
	if err != nil {
		return err
	}
	result, err := s.orders.Place(c.Context(), req)
	if err != nil {
		return orderError(c, err)
	}
	return okJSON(c, binanceOrderResponse(result.Order, result.Trades))
}

func (s *Server) binanceGetOrder(c fiber.Ctx) error {
	userID, err := s.requireBinanceAPIUser(c)
	if err != nil {
		return err
	}
	id := order.ID(firstNonEmpty(c.FormValue("orderId"), c.FormValue("origClientOrderId")))
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(binanceError(-1102, "orderId or origClientOrderId is required"))
	}
	item, err := s.orders.Get(c.Context(), id)
	if err != nil {
		return orderError(c, err)
	}
	if item.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(binanceError(-2013, "Order does not exist."))
	}
	return okJSON(c, binanceOrderResponse(*item, nil))
}

func (s *Server) binanceCancelOrder(c fiber.Ctx) error {
	userID, err := s.requireBinanceAPIUser(c)
	if err != nil {
		return err
	}
	id := order.ID(firstNonEmpty(c.FormValue("orderId"), c.FormValue("origClientOrderId")))
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(binanceError(-1102, "orderId or origClientOrderId is required"))
	}
	item, err := s.orders.Cancel(c.Context(), id, apporders.CancelRequest{UserID: userID})
	if err != nil {
		return orderError(c, err)
	}
	return okJSON(c, binanceOrderResponse(*item, nil))
}

func (s *Server) binanceOpenOrders(c fiber.Ctx) error {
	userID, err := s.requireBinanceAPIUser(c)
	if err != nil {
		return err
	}
	market, err := s.optionalBinanceMarket(c)
	if err != nil {
		return err
	}
	orders, err := s.openOrdersForUser(c, userID, market)
	if err != nil {
		return err
	}
	out := make([]fiber.Map, 0, len(orders))
	for _, item := range orders {
		out = append(out, binanceOrderResponse(item, nil))
	}
	return okJSON(c, out)
}

func (s *Server) binanceAllOrders(c fiber.Ctx) error {
	userID, err := s.requireBinanceAPIUser(c)
	if err != nil {
		return err
	}
	market, err := s.optionalBinanceMarket(c)
	if err != nil {
		return err
	}
	limit, err := queryInt(c, "limit", 500)
	if err != nil {
		return err
	}
	items, err := s.orders.OrderHistory(c.Context(), apporders.HistoryRequest{UserID: userID, Market: market, Limit: limit})
	if err != nil {
		return orderError(c, err)
	}
	out := make([]fiber.Map, 0, len(items))
	for _, item := range items {
		out = append(out, binanceOrderResponse(item, nil))
	}
	return okJSON(c, out)
}

func (s *Server) binanceMyTrades(c fiber.Ctx) error {
	userID, err := s.requireBinanceAPIUser(c)
	if err != nil {
		return err
	}
	market, err := s.resolveMarketSymbol(c, c.FormValue("symbol"))
	if err != nil {
		return err
	}
	limit, err := queryInt(c, "limit", 500)
	if err != nil {
		return err
	}
	items, err := s.orders.UserTrades(c.Context(), apporders.HistoryRequest{UserID: userID, Market: market, Limit: limit})
	if err != nil {
		return orderError(c, err)
	}
	out := make([]fiber.Map, 0, len(items))
	for i, item := range items {
		out = append(out, fiber.Map{
			"symbol":          binanceSymbol(item.Market),
			"id":              i,
			"orderId":         string(userTradeOrderID(item, userID)),
			"price":           item.Price,
			"qty":             item.Quantity,
			"quoteQty":        item.QuoteQuantity,
			"commission":      "0",
			"commissionAsset": item.QuoteAsset,
			"time":            item.CreatedAt.UTC().UnixMilli(),
			"isBuyer":         userIsBuyer(item, userID),
			"isMaker":         userIsMaker(item, userID),
			"isBestMatch":     true,
		})
	}
	return okJSON(c, out)
}

func (s *Server) binanceOrderRequest(c fiber.Ctx, userID string) (apporders.PlaceRequest, error) {
	market, err := s.resolveMarketSymbol(c, c.FormValue("symbol"))
	if err != nil {
		return apporders.PlaceRequest{}, err
	}
	orderType, err := binanceOrderType(c.FormValue("type", "LIMIT"))
	if err != nil {
		return apporders.PlaceRequest{}, c.Status(fiber.StatusBadRequest).JSON(binanceError(-1116, err.Error()))
	}
	price := c.FormValue("price")
	if orderType == "market" && strings.TrimSpace(price) == "" {
		price, err = s.marketProtectionPrice(c, market, c.FormValue("side"))
		if err != nil {
			return apporders.PlaceRequest{}, err
		}
	}
	return apporders.PlaceRequest{
		ClientOrderID: firstNonEmpty(c.FormValue("newClientOrderId"), fmt.Sprintf("api_%d", time.Now().UTC().UnixNano())),
		UserID:        userID,
		Market:        market,
		Side:          strings.ToLower(c.FormValue("side")),
		Type:          orderType,
		TimeInForce:   strings.ToLower(c.FormValue("timeInForce")),
		Price:         price,
		StopPrice:     c.FormValue("stopPrice"),
		Quantity:      c.FormValue("quantity"),
	}, nil
}

func (s *Server) requireBinanceAPIUser(c fiber.Ctx) (string, error) {
	credential, err := lookupAPICredential(c.Get("X-MBX-APIKEY"))
	if err != nil {
		return "", c.Status(fiber.StatusUnauthorized).JSON(binanceError(-2015, err.Error()))
	}
	payload, signature := signedPayload(c)
	if payload == "" || signature == "" {
		return "", c.Status(fiber.StatusUnauthorized).JSON(binanceError(-1022, "Signature for this request is not valid."))
	}
	expected := hmacSHA256Hex(credential.Secret, payload)
	if !hmac.Equal([]byte(strings.ToLower(signature)), []byte(expected)) {
		return "", c.Status(fiber.StatusUnauthorized).JSON(binanceError(-1022, "Signature for this request is not valid."))
	}
	if err := validateSignedTimestamp(c); err != nil {
		return "", err
	}
	return credential.UserID, nil
}

func lookupAPICredential(apiKey string) (apiCredential, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return apiCredential{}, fmt.Errorf("API-key format invalid")
	}
	for _, credential := range configuredAPICredentials() {
		if credential.Key == apiKey {
			return credential, nil
		}
	}
	return apiCredential{}, fmt.Errorf("Invalid API-key, IP, or permissions for action")
}

func configuredAPICredentials() []apiCredential {
	out := make([]apiCredential, 0)
	if key := strings.TrimSpace(os.Getenv("EXCHANGE_API_KEY")); key != "" {
		out = append(out, apiCredential{
			UserID: firstNonEmpty(os.Getenv("EXCHANGE_API_USER_ID"), "demo-user"),
			Key:    key,
			Secret: strings.TrimSpace(os.Getenv("EXCHANGE_API_SECRET")),
		})
	}
	for _, raw := range strings.FieldsFunc(os.Getenv("EXCHANGE_API_KEYS"), func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
		parts := strings.Split(strings.TrimSpace(raw), ":")
		if len(parts) != 3 {
			continue
		}
		out = append(out, apiCredential{UserID: strings.TrimSpace(parts[0]), Key: strings.TrimSpace(parts[1]), Secret: strings.TrimSpace(parts[2])})
	}
	filtered := out[:0]
	for _, item := range out {
		if item.UserID != "" && item.Key != "" && item.Secret != "" {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func signedPayload(c fiber.Ctx) (string, string) {
	rawQuery := string(c.Request().URI().QueryString())
	if strings.Contains(rawQuery, "signature=") {
		return splitSignature(rawQuery)
	}
	return splitSignature(string(c.BodyRaw()))
}

func splitSignature(payload string) (string, string) {
	if payload == "" {
		return "", ""
	}
	parts := strings.Split(payload, "&")
	out := make([]string, 0, len(parts))
	signature := ""
	for _, part := range parts {
		if strings.HasPrefix(part, "signature=") {
			signature = strings.TrimPrefix(part, "signature=")
			continue
		}
		if strings.HasPrefix(part, "signature%3D") {
			continue
		}
		out = append(out, part)
	}
	if decoded, err := url.QueryUnescape(signature); err == nil {
		signature = decoded
	}
	return strings.Join(out, "&"), signature
}

func hmacSHA256Hex(secret string, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func validateSignedTimestamp(c fiber.Ctx) error {
	timestamp, err := strconv.ParseInt(c.FormValue("timestamp"), 10, 64)
	if err != nil || timestamp <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(binanceError(-1021, "Timestamp for this request is outside of the recvWindow."))
	}
	recvWindow := int64(5000)
	if raw := strings.TrimSpace(c.FormValue("recvWindow")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			recvWindow = parsed
		}
	}
	if recvWindow > 60000 {
		recvWindow = 60000
	}
	now := time.Now().UTC().UnixMilli()
	if timestamp < now-recvWindow || timestamp > now+1000 {
		return c.Status(fiber.StatusBadRequest).JSON(binanceError(-1021, "Timestamp for this request is outside of the recvWindow."))
	}
	return nil
}

func binanceOrderType(raw string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "", "LIMIT":
		return string(order.TypeLimit), nil
	case "MARKET":
		return string(order.TypeMarket), nil
	case "STOP_LOSS_LIMIT", "STOP_LIMIT":
		return string(order.TypeStopLimit), nil
	default:
		return "", fmt.Errorf("Invalid order type")
	}
}

func (s *Server) marketProtectionPrice(c fiber.Ctx, market string, side string) (string, error) {
	book, err := s.orders.Book(c.Context(), market, 1)
	if err != nil {
		return "", orderError(c, err)
	}
	if strings.EqualFold(side, "BUY") && len(book.Asks) > 0 {
		return book.Asks[0].Price, nil
	}
	if strings.EqualFold(side, "SELL") && len(book.Bids) > 0 {
		return book.Bids[0].Price, nil
	}
	items, err := s.orders.MarketSummaries(c.Context())
	if err != nil {
		return "", orderError(c, err)
	}
	for _, item := range items {
		if item.Symbol == market {
			return item.LastPrice, nil
		}
	}
	return "", c.Status(fiber.StatusBadRequest).JSON(binanceError(-1013, "Market order requires price protection when no reference price exists."))
}

func (s *Server) optionalBinanceMarket(c fiber.Ctx) (string, error) {
	if strings.TrimSpace(c.FormValue("symbol")) == "" {
		return "", nil
	}
	return s.resolveMarketSymbol(c, c.FormValue("symbol"))
}

func (s *Server) openOrdersForUser(c fiber.Ctx, userID string, market string) ([]order.Order, error) {
	statuses := []string{string(order.StatusOpen), string(order.StatusPartiallyFilled), string(order.StatusPendingStop), string(order.StatusPendingMatch)}
	out := make([]order.Order, 0)
	for _, status := range statuses {
		items, err := s.orders.OrderHistory(c.Context(), apporders.HistoryRequest{UserID: userID, Market: market, Status: status, Limit: 500})
		if err != nil {
			return nil, orderError(c, err)
		}
		out = append(out, items...)
	}
	return out, nil
}

func binanceOrderResponse(item order.Order, trades []trade.Trade) fiber.Map {
	executedQuote := "0"
	fills := make([]fiber.Map, 0, len(trades))
	for _, tr := range trades {
		executedQuote = decimalAddString(executedQuote, tr.QuoteQuantity)
		fills = append(fills, fiber.Map{
			"price":           tr.Price,
			"qty":             tr.Quantity,
			"commission":      "0",
			"commissionAsset": tr.QuoteAsset,
			"tradeId":         string(tr.ID),
		})
	}
	out := fiber.Map{
		"symbol":                  binanceSymbol(item.Market),
		"orderId":                 string(item.ID),
		"clientOrderId":           string(item.ClientOrderID),
		"transactTime":            item.UpdatedAt.UTC().UnixMilli(),
		"price":                   item.Price,
		"origQty":                 item.Quantity,
		"executedQty":             item.FilledQuantity,
		"cummulativeQuoteQty":     executedQuote,
		"status":                  binanceOrderStatus(item.Status),
		"timeInForce":             strings.ToUpper(string(item.TimeInForce)),
		"type":                    binanceOrderTypeName(item.Type),
		"side":                    strings.ToUpper(string(item.Side)),
		"workingTime":             item.CreatedAt.UTC().UnixMilli(),
		"selfTradePreventionMode": "NONE",
	}
	if item.Type == order.TypeStopLimit {
		out["stopPrice"] = item.StopPrice
	}
	if len(fills) > 0 {
		out["fills"] = fills
	}
	return out
}

func binanceOrderStatus(status order.Status) string {
	switch status {
	case order.StatusOpen, order.StatusPendingStop, order.StatusPendingMatch:
		return "NEW"
	case order.StatusPartiallyFilled:
		return "PARTIALLY_FILLED"
	case order.StatusFilled:
		return "FILLED"
	case order.StatusCanceled:
		return "CANCELED"
	case order.StatusExpired:
		return "EXPIRED"
	case order.StatusRejected:
		return "REJECTED"
	default:
		return strings.ToUpper(string(status))
	}
}

func binanceOrderTypeName(orderType order.Type) string {
	switch orderType {
	case order.TypeStopLimit:
		return "STOP_LOSS_LIMIT"
	default:
		return strings.ToUpper(string(orderType))
	}
}

func userTradeOrderID(item trade.Trade, userID string) order.ID {
	if item.MakerUserID == userID {
		return item.MakerOrderID
	}
	return item.TakerOrderID
}

func userIsMaker(item trade.Trade, userID string) bool {
	return item.MakerUserID == userID
}

func userIsBuyer(item trade.Trade, userID string) bool {
	if item.TakerSide == order.SideBuy {
		return item.TakerUserID == userID
	}
	return item.MakerUserID == userID
}

func decimalAddString(left string, right string) string {
	return decimal.Add(left, right)
}

func binanceError(code int, message string) fiber.Map {
	return fiber.Map{"code": code, "msg": message}
}
