package payment

import (
	"github.com/example/e-commerce-be/internal/models"
	"testing"
)

func TestCanApply(t *testing.T) {
	for _, name := range []string{"valid", "underpaid", "overpaid", "outgoing", "wrong bank", "wrong account", "paid", "cancelled", "processing", "currency", "cod", "zero"} {
		t.Run(name, func(t *testing.T) {
			r := models.PaymentWebhookReceipt{Direction: "in", Bank: "TestBank", AccountNumber: "account", Amount: 100}
			p := models.Payment{Method: "sepay", Status: "pending", Bank: "TestBank", AccountNumber: "account", Amount: 100}
			o := models.Order{Status: "pending", Currency: "VND"}
			switch name {
			case "underpaid":
				r.Amount = 99
			case "overpaid":
				r.Amount = 101
			case "outgoing":
				r.Direction = "out"
			case "wrong bank":
				r.Bank = "other"
			case "wrong account":
				r.AccountNumber = "other"
			case "paid":
				p.Status = "paid"
			case "cancelled":
				o.Status = "cancelled"
			case "processing":
				o.Status = "processing"
			case "currency":
				o.Currency = "USD"
			case "cod":
				p.Method = "cod"
			case "zero":
				p.Amount = 0
				r.Amount = 0
			}
			if got := canApply(r, p, o); got != (name == "valid") {
				t.Fatalf("canApply=%v", got)
			}
		})
	}
}
