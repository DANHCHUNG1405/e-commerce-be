package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
)

func TestVerify(t *testing.T) {
	cfg := Config{Bank: "TestBank", Account: "test-account", WebhookSecret: strings.Repeat("x", 32)}
	now := time.Unix(1800000000, 0)
	body := []byte(`{"id":42}`)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(cfg.WebhookSecret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	for _, tc := range []struct {
		name       string
		body       []byte
		stamp, sig string
		now        time.Time
		want       bool
	}{
		{"valid", body, timestamp, signature, now, true},
		{"tampered", []byte(`{"id":43}`), timestamp, signature, now, false},
		{"stale", body, timestamp, signature, now.Add(301 * time.Second), false},
		{"future", body, timestamp, signature, now.Add(-301 * time.Second), false},
		{"missing signature", body, timestamp, "", now, false},
		{"invalid timestamp", body, "bad", signature, now, false},
		{"overflow timestamp", body, "9223372036854775808", signature, now, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cfg.Verify(tc.body, tc.stamp, tc.sig, tc.now); got != tc.want {
				t.Fatalf("Verify = %v", got)
			}
		})
	}
	if (Config{}).Verify(body, timestamp, signature, now) {
		t.Fatal("disabled config accepted")
	}
}

func TestPaymentCode(t *testing.T) {
	code := Code(uuid.New())
	other := Code(uuid.New())
	for _, tc := range []struct {
		content string
		code    *string
		want    string
	}{
		{"transfer " + code, nil, code}, {strings.ToLower(code), nil, code}, {"", &code, code},
		{"wrong", nil, ""}, {code + "A", nil, ""}, {code + " " + other, nil, ""}, {other, &code, ""},
	} {
		if got := (Webhook{Content: tc.content, Code: tc.code}).PaymentCode(); got != tc.want {
			t.Fatalf("unexpected code %q", got)
		}
	}
}

func TestQRURL(t *testing.T) {
	u, err := url.Parse(QRURL("Test Bank", "test-account", "EC123", 1000))
	if err != nil || u.Host != "qr.sepay.vn" || u.Query().Get("bank") != "Test Bank" || u.Query().Get("amount") != "1000" || u.Query().Get("des") != "EC123" {
		t.Fatal("invalid QR URL")
	}
}

func TestReceiveValidation(t *testing.T) {
	s := New(nil, Config{Bank: "TestBank", Account: "test-account", WebhookSecret: strings.Repeat("x", 32)})
	for _, w := range []Webhook{{}, {ID: 1, Gateway: "TestBank", AccountNumber: "test-account", TransferType: "in", TransferAmount: -1}, {ID: 1, Gateway: "TestBank", AccountNumber: "test-account", TransferType: "unknown"}} {
		if err := s.Receive(context.Background(), w); !errors.Is(err, shared.ErrInvalid) {
			t.Fatal("invalid payload accepted")
		}
	}
	if err := New(nil, Config{}).Receive(context.Background(), Webhook{}); !errors.Is(err, shared.ErrUnavailable) {
		t.Fatal("disabled service accepted request")
	}
}
