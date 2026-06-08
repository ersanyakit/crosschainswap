package rest

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func TestGatewaySignature(t *testing.T) {
	body := []byte(`{"event_id":"e1"}`)
	got := gatewaySignature("secret", "1710000000", body)
	if !subtleCompareHex(got, got) {
		t.Fatal("expected signature to compare equal to itself")
	}
	if subtleCompareHex("00", got) {
		t.Fatal("expected invalid signature to fail")
	}
}

func TestGatewayCallbackSecretPreference(t *testing.T) {
	t.Setenv("PAYMENT_GATEWAY_SECRET", "legacy")
	t.Setenv("PAYMENT_GATEWAY_API_SECRET", "api")
	t.Setenv("PAYMENT_GATEWAY_WEBHOOK_SECRET", "webhook")
	if got := gatewayCallbackSecret(); got != "webhook" {
		t.Fatalf("gatewayCallbackSecret = %q, want webhook", got)
	}
}

func TestGatewayDepositCallbackFromPaymentEvent(t *testing.T) {
	body := []byte(`{
		"payment_id": "pay_1",
		"event_type": "payment_succeeded",
		"user_id": "user-a",
		"symbol": "USDC",
		"amount": "25.50",
		"expected_amount_raw": "25500000",
		"decimals": 6,
		"chain_id": 8453
	}`)
	req, err := gatewayDepositCallbackFromBody(gatewayEventTypeFromBody(body), body)
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != "completed" || req.UserID != "user-a" || req.Asset != "USDC" || req.AmountRaw != "25500000" || req.Decimals != 6 {
		t.Fatalf("unexpected payment callback mapping: %#v", req)
	}
}

func TestGatewayDepositCallbackFromRawTransferEvent(t *testing.T) {
	body := []byte(`{
		"data": {
			"user_id": "user-a",
			"symbol": "PEPPER",
			"amount_raw": "12345000",
			"decimals": 3,
			"chain": "solana",
			"tx_hash": "tx_1"
		}
	}`)
	req, err := gatewayDepositCallbackFromBody("native_transfer", body)
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != "pending" || req.UserID != "user-a" || req.Asset != "PEPPER" || req.AmountRaw != "12345000" || req.Decimals != 3 {
		t.Fatalf("unexpected raw transfer callback mapping: %#v", req)
	}
}

func TestGatewayDepositCallbackFromManualTestDepositEvent(t *testing.T) {
	body := []byte(`{
		"event_id": "test_1",
		"user_id": "user-a",
		"symbol": "BTC",
		"amount_raw": "25000000",
		"decimals": 8,
		"hash": "manual-tx"
	}`)
	req, err := gatewayDepositCallbackFromBody("manual_test_deposit", body)
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != "completed" || req.UserID != "user-a" || req.Asset != "BTC" || req.AmountRaw != "25000000" || req.Decimals != 8 {
		t.Fatalf("unexpected manual test deposit mapping: %#v", req)
	}
}

func TestGatewayUnifiedCallbackSuccessResetsHTTPStatus(t *testing.T) {
	t.Setenv("PAYMENT_GATEWAY_WEBHOOK_SECRET", "webhook")
	body := `{"event_type":"unknown_event","event_id":"evt_1"}`
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := gatewaySignature("webhook", timestamp, []byte(body))

	app := fiber.New()
	server := &Server{}
	app.Post("/callback", func(c fiber.Ctx) error {
		c.Status(fiber.StatusServiceUnavailable)
		return server.gatewayUnifiedCallback(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/callback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gateway-Event", "unknown_event")
	req.Header.Set("X-Gateway-Timestamp", timestamp)
	req.Header.Set("X-Gateway-Signature", "sha256="+signature)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
