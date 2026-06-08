package rest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	apporders "exchange/internal/app/orders"

	"github.com/gofiber/fiber/v3"
)

func (s *Server) gatewayUnifiedCallback(c fiber.Ctx) error {
	body := c.Body()
	if err := requireGatewayCallbackSignature(c, body); err != nil {
		return err
	}
	eventType := strings.ToLower(strings.TrimSpace(c.Get("X-Gateway-Event")))
	if eventType == "" {
		eventType = gatewayEventTypeFromBody(body)
	}
	eventID := strings.TrimSpace(c.Get("X-Gateway-Event-Id"))
	switch eventType {
	case "payment_succeeded", "payment_failed", "payment_expired":
		req, err := gatewayDepositCallbackFromBody(eventType, body)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
		}
		if req.EventID == "" {
			req.EventID = eventID
		}
		result, err := s.orders.ApplyGatewayDepositCallback(c.Context(), req)
		if err != nil {
			return orderError(c, err)
		}
		return c.Status(fiber.StatusOK).JSON(result)
	case "native_transfer", "token_transfer", "erc20_transfer", "spl_transfer", "deposit_detected", "deposit_confirmed", "manual_test_deposit":
		req, err := gatewayDepositCallbackFromBody(eventType, body)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
		}
		if req.EventID == "" {
			req.EventID = eventID
		}
		result, err := s.orders.ApplyGatewayDepositCallback(c.Context(), req)
		if err != nil {
			return orderError(c, err)
		}
		return c.Status(fiber.StatusOK).JSON(result)
	case "payout_completed", "payout_succeeded", "payout_failed", "payout_canceled", "withdrawal_completed", "withdrawal_failed", "withdrawal_canceled":
		req, err := gatewayWithdrawalCallbackFromBody(eventType, body)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
		}
		if req.EventID == "" {
			req.EventID = eventID
		}
		result, err := s.orders.ApplyGatewayWithdrawalCallback(c.Context(), req)
		if err != nil {
			return orderError(c, err)
		}
		return c.Status(fiber.StatusOK).JSON(result)
	default:
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok", "action": "event_ignored", "event": eventType})
	}
}

func (s *Server) gatewayDepositCallback(c fiber.Ctx) error {
	body := c.Body()
	if err := requireGatewayCallbackSignature(c, body); err != nil {
		return err
	}
	var req apporders.GatewayDepositCallback
	if err := json.Unmarshal(body, &req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
	}
	result, err := s.orders.ApplyGatewayDepositCallback(c.Context(), req)
	if err != nil {
		return orderError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(result)
}

func (s *Server) gatewayWithdrawalCallback(c fiber.Ctx) error {
	body := c.Body()
	if err := requireGatewayCallbackSignature(c, body); err != nil {
		return err
	}
	var req apporders.GatewayWithdrawalCallback
	if err := json.Unmarshal(body, &req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errorResponse{Error: err.Error()})
	}
	result, err := s.orders.ApplyGatewayWithdrawalCallback(c.Context(), req)
	if err != nil {
		return orderError(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(result)
}

func requireGatewayCallbackSignature(c fiber.Ctx, body []byte) error {
	secret := gatewayCallbackSecret()
	if secret == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(errorResponse{Error: "payment gateway webhook secret is not configured"})
	}
	signature := strings.TrimSpace(c.Get("X-Gateway-Signature"))
	timestamp := strings.TrimSpace(c.Get("X-Gateway-Timestamp"))
	if signature == "" || timestamp == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(errorResponse{Error: "gateway webhook signature is required"})
	}
	if !gatewayTimestampValid(timestamp, 5*time.Minute) {
		return c.Status(fiber.StatusUnauthorized).JSON(errorResponse{Error: "invalid gateway timestamp"})
	}
	expected := gatewaySignature(secret, timestamp, body)
	signature = strings.TrimPrefix(strings.ToLower(signature), "sha256=")
	if subtleCompareHex(signature, expected) {
		return nil
	}
	return c.Status(fiber.StatusUnauthorized).JSON(errorResponse{Error: "invalid gateway signature"})
}

func gatewayCallbackSecret() string {
	return strings.TrimSpace(os.Getenv("PAYMENT_GATEWAY_WEBHOOK_SECRET"))
}

func gatewayTimestampValid(raw string, maxSkew time.Duration) bool {
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return false
	}
	timestamp := time.Unix(seconds, 0)
	now := time.Now()
	return timestamp.After(now.Add(-maxSkew)) && timestamp.Before(now.Add(maxSkew))
}

func gatewaySignature(secret string, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func subtleCompareHex(actual string, expected string) bool {
	actualBytes, err := hex.DecodeString(strings.TrimSpace(actual))
	if err != nil {
		return false
	}
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil {
		return false
	}
	return hmac.Equal(actualBytes, expectedBytes)
}

func gatewayDepositCallbackFromBody(eventType string, body []byte) (apporders.GatewayDepositCallback, error) {
	payload, err := decodeGatewayPayload(body)
	if err != nil {
		return apporders.GatewayDepositCallback{}, err
	}
	req := apporders.GatewayDepositCallback{
		EventID:       firstPayloadString(payload, "event_id", "eventID", "id"),
		PaymentID:     firstPayloadString(payload, "payment_id", "paymentID", "payment.id", "session.payment_id", "data.payment_id"),
		TrackID:       firstPayloadString(payload, "track_id", "trackID", "session.track_id", "data.track_id"),
		OrderID:       firstPayloadString(payload, "order_id", "orderID", "session.order_id", "data.order_id"),
		UserID:        firstPayloadString(payload, "user_id", "userID", "customer_id", "customerID", "session.user_id", "payment.user_id", "data.user_id"),
		Asset:         firstPayloadString(payload, "asset", "symbol", "currency_symbol", "token_symbol", "selected_asset", "payment.selected_asset", "session.selected_asset", "data.asset", "data.symbol", "data.selected_asset"),
		Symbol:        firstPayloadString(payload, "symbol"),
		SelectedAsset: firstPayloadString(payload, "selected_asset", "payment.selected_asset", "session.selected_asset", "data.selected_asset"),
		Amount:        firstPayloadString(payload, "crypto_amount", "paid_amount", "received_amount", "asset_amount", "token_amount", "payment.crypto_amount", "session.crypto_amount", "data.crypto_amount", "data.paid_amount", "data.received_amount"),
		AmountRaw:     firstPayloadString(payload, "expected_amount_raw", "amount_raw", "raw_amount", "value", "payment.expected_amount_raw", "payment.amount_raw", "transaction.amount_raw", "data.expected_amount_raw", "data.amount_raw"),
		Status:        statusForGatewayEvent(eventType, firstPayloadString(payload, "status", "payment.status", "session.status", "data.status")),
		ChainKey:      firstPayloadString(payload, "chain_key", "chain", "network", "selected_chain", "payment.selected_chain", "session.selected_chain", "data.chain_key", "data.chain", "data.network", "data.selected_chain"),
		Chain:         firstPayloadString(payload, "chain", "network", "data.chain", "data.network"),
		SelectedChain: firstPayloadString(payload, "selected_chain", "payment.selected_chain", "session.selected_chain", "data.selected_chain"),
		TxHash:        firstPayloadString(payload, "tx_hash", "txHash", "transaction_hash", "hash", "transaction.tx_hash", "data.tx_hash"),
	}
	if req.EventID == "" {
		req.EventID = eventReference(eventType, req.PaymentID, req.TrackID, req.OrderID, req.TxHash)
	}
	if req.PaymentID == "" && strings.HasPrefix(eventType, "payment_") {
		req.PaymentID = req.EventID
	}
	if decimals, ok := firstPayloadInt(payload, "decimals", "token_decimals", "asset_decimals", "data.decimals"); ok {
		req.Decimals = decimals
	}
	if req.Asset == "" {
		req.Asset = req.SelectedAsset
	}
	return req, nil
}

func gatewayEventTypeFromBody(body []byte) string {
	payload, err := decodeGatewayPayload(body)
	if err != nil {
		return ""
	}
	return strings.ToLower(firstPayloadString(payload, "event_type", "eventType", "type"))
}

func gatewayWithdrawalCallbackFromBody(eventType string, body []byte) (apporders.GatewayWithdrawalCallback, error) {
	payload, err := decodeGatewayPayload(body)
	if err != nil {
		return apporders.GatewayWithdrawalCallback{}, err
	}
	return apporders.GatewayWithdrawalCallback{
		EventID:      firstPayloadString(payload, "event_id", "eventID", "id"),
		WithdrawalID: firstPayloadString(payload, "withdrawal_id", "withdrawalID", "exchange_withdrawal_id", "metadata.withdrawal_id"),
		PayoutID:     firstPayloadString(payload, "payout_id", "payoutID", "id", "data.payout_id"),
		ID:           firstPayloadString(payload, "id"),
		Status:       statusForGatewayEvent(eventType, firstPayloadString(payload, "status", "data.status")),
		TxHash:       firstPayloadString(payload, "tx_hash", "txHash", "transaction_hash", "hash", "data.tx_hash"),
	}, nil
}

func decodeGatewayPayload(body []byte) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, fmt.Errorf("gateway callback payload must be an object")
	}
	return payload, nil
}

func statusForGatewayEvent(eventType string, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "payment_succeeded", "deposit_confirmed", "manual_test_deposit", "payout_completed", "payout_succeeded", "withdrawal_completed":
		return "completed"
	case "payment_failed", "payment_expired", "payout_failed", "payout_canceled", "withdrawal_failed", "withdrawal_canceled":
		return "failed"
	case "native_transfer", "token_transfer", "erc20_transfer", "spl_transfer", "deposit_detected":
		if strings.TrimSpace(fallback) == "" {
			return "pending"
		}
	}
	return fallback
}

func eventReference(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			clean = append(clean, part)
		}
	}
	return strings.Join(clean, ":")
}

func firstPayloadString(payload map[string]any, paths ...string) string {
	for _, path := range paths {
		if value, ok := payloadValue(payload, path); ok {
			switch typed := value.(type) {
			case string:
				if out := strings.TrimSpace(typed); out != "" {
					return out
				}
			case float64:
				return strconv.FormatFloat(typed, 'f', -1, 64)
			case json.Number:
				return typed.String()
			}
		}
	}
	return ""
}

func firstPayloadInt(payload map[string]any, paths ...string) (int, bool) {
	for _, path := range paths {
		if value, ok := payloadValue(payload, path); ok {
			switch typed := value.(type) {
			case float64:
				return int(typed), true
			case json.Number:
				parsed, err := strconv.Atoi(typed.String())
				return parsed, err == nil
			case string:
				parsed, err := strconv.Atoi(strings.TrimSpace(typed))
				return parsed, err == nil
			}
		}
	}
	return 0, false
}

func payloadValue(payload map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var current any = payload
	for _, part := range parts {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := object[part]
		if !ok {
			value, ok = caseInsensitiveValue(object, part)
			if !ok {
				return nil, false
			}
		}
		current = value
	}
	return current, true
}

func caseInsensitiveValue(object map[string]any, key string) (any, bool) {
	for candidate, value := range object {
		if strings.EqualFold(candidate, key) {
			return value, true
		}
	}
	return nil, false
}
