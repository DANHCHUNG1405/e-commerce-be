package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Config enables bank-transfer webhooks, not the separate SePay Payment Gateway.
type Config struct {
	Bank          string
	Account       string
	WebhookSecret string
}

func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Bank) != "" && strings.TrimSpace(c.Account) != "" && len(c.WebhookSecret) >= 32
}

func Code(id uuid.UUID) string {
	return "EC" + strings.ToUpper(strings.ReplaceAll(id.String(), "-", ""))
}

// Verify authenticates the exact raw bytes and bounds replay to five minutes.
func (c Config) Verify(body []byte, timestamp, signature string, now time.Time) bool {
	t, err := strconv.ParseInt(timestamp, 10, 64)
	if !c.Enabled() || err != nil || t < now.Unix()-300 || t > now.Unix()+300 || !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(c.WebhookSecret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), got)
}

type Webhook struct {
	ID             int64   `json:"id"`
	Gateway        string  `json:"gateway"`
	AccountNumber  string  `json:"accountNumber"`
	SubAccount     *string `json:"subAccount"`
	Code           *string `json:"code"`
	Content        string  `json:"content"`
	TransferType   string  `json:"transferType"`
	TransferAmount int64   `json:"transferAmount"`
}

var paymentCodePattern = regexp.MustCompile(`(?i)\bEC[0-9A-F]{32}\b`)

func (w Webhook) PaymentCode() string {
	codes := paymentCodePattern.FindAllString(strings.ToUpper(w.Content), -1)
	if w.Code != nil && *w.Code != "" {
		code := strings.ToUpper(*w.Code)
		if len(code) != 34 || paymentCodePattern.FindString(code) != code {
			return ""
		}
		codes = append(codes, code)
	}
	if len(codes) == 0 {
		return ""
	}
	for _, code := range codes {
		if code != codes[0] {
			return ""
		}
	}
	return codes[0]
}

func QRURL(bank, account, code string, amount int64) string {
	q := url.Values{"bank": {bank}, "acc": {account}, "amount": {strconv.FormatInt(amount, 10)}, "des": {code}}
	return "https://qr.sepay.vn/img?" + q.Encode()
}
