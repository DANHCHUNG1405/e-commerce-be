package payment

import (
	"context"
	"strconv"
	"strings"

	"github.com/example/e-commerce-be/internal/models"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
)

type Service struct {
	repo   *Repository
	config Config
}

func New(repo *Repository, config Config) *Service { return &Service{repo: repo, config: config} }

type Instructions struct {
	PaymentID       uuid.UUID `json:"paymentId"`
	Status          string    `json:"status"`
	Amount          int64     `json:"amount"`
	Currency        string    `json:"currency"`
	Bank            string    `json:"bank"`
	AccountNumber   string    `json:"accountNumber"`
	TransferContent string    `json:"transferContent"`
	QRURL           string    `json:"qrUrl,omitempty"`
}

func (s *Service) Instructions(ctx context.Context, user, order uuid.UUID) (Instructions, error) {
	if !s.config.Enabled() {
		return Instructions{}, shared.ErrUnavailable
	}
	p, err := s.repo.OwnPayment(ctx, user, order)
	if err != nil {
		return Instructions{}, err
	}
	if p.Method != "sepay" {
		return Instructions{}, shared.ErrConflict
	}
	v := Instructions{PaymentID: p.ID, Status: p.Status, Amount: p.Amount, Currency: "VND", Bank: p.Bank, AccountNumber: p.AccountNumber}
	if p.Code != nil {
		v.TransferContent = *p.Code
	}
	if p.Status == "pending" {
		v.QRURL = QRURL(p.Bank, p.AccountNumber, v.TransferContent, p.Amount)
	}
	return v, nil
}

// Receive is called only after signature verification. Reconciliation exceptions
// are durably recorded rather than marking underpaid/cancelled orders as paid.
func (s *Service) Receive(ctx context.Context, w Webhook) error {
	if !s.config.Enabled() {
		return shared.ErrUnavailable
	}
	if w.ID <= 0 || w.TransferAmount < 0 || w.AccountNumber == "" || w.Gateway == "" || (w.TransferType != "in" && w.TransferType != "out") {
		return shared.ErrInvalid
	}
	return s.repo.Within(ctx, func(tx *Repository) error {
		rec := receipt(w)
		inserted, err := tx.InsertReceipt(&rec)
		if err != nil {
			return err
		}
		if !inserted {
			old, err := tx.ExistingReceipt(rec.Provider, rec.ProviderTransactionID)
			if err != nil {
				return err
			}
			if old.Code != rec.Code || old.Amount != rec.Amount || old.Bank != rec.Bank || old.AccountNumber != rec.AccountNumber || old.Direction != rec.Direction {
				return shared.ErrConflict
			}
			return nil
		}
		if rec.Status != "ignored" && rec.Code != "" {
			p, found, err := tx.PaymentByCode(rec.Code)
			if err != nil {
				return err
			}
			if found {
				o, err := tx.LockOrderPayment(&p)
				if err != nil {
					return err
				}
				rec.PaymentID = &p.ID
				rec.Status = "requires_review"
				if canApply(rec, p, o) {
					rec.Status = "applied"
					if err := tx.MarkPaid(&p); err != nil {
						return err
					}
					if err := tx.Enqueue("payment", p.ID, "payment_succeeded", map[string]any{"orderId": o.ID, "amount": p.Amount, "providerTransactionId": rec.ProviderTransactionID}); err != nil {
						return err
					}
				}
				if err := tx.RecordTransaction(&models.PaymentTransaction{PaymentID: p.ID, Provider: rec.Provider, ProviderTransactionID: rec.ProviderTransactionID, Status: rec.Status, Amount: rec.Amount}); err != nil {
					return err
				}
			}
		}
		if rec.Status == "requires_review" || rec.Status == "unmatched" {
			if err := tx.Enqueue("payment_webhook", rec.ID, "payment_reconciliation_required", map[string]any{"receiptId": rec.ID, "status": rec.Status}); err != nil {
				return err
			}
		}
		return tx.SaveReceipt(&rec)
	})
}

func canApply(rec models.PaymentWebhookReceipt, p models.Payment, o models.Order) bool {
	return rec.Direction == "in" && strings.EqualFold(rec.Bank, p.Bank) && rec.AccountNumber == p.AccountNumber && rec.Amount > 0 && rec.Amount == p.Amount && p.Method == "sepay" && p.Status == "pending" && o.Status == "pending" && o.Currency == "VND" && o.DeletedAt == nil && p.DeletedAt == nil
}

func receipt(w Webhook) models.PaymentWebhookReceipt {
	status := "unmatched"
	if w.TransferType != "in" {
		status = "ignored"
	}
	return models.PaymentWebhookReceipt{Provider: "sepay", ProviderTransactionID: strconv.FormatInt(w.ID, 10), Code: w.PaymentCode(), Bank: strings.TrimSpace(w.Gateway), AccountNumber: w.AccountNumber, Direction: w.TransferType, Amount: w.TransferAmount, Status: status}
}

func (s *Service) Receipts(ctx context.Context, user uuid.UUID, status string, page, limit int) ([]models.PaymentWebhookReceipt, error) {
	if page < 1 || page > 100000 || limit < 1 || limit > 100 {
		return nil, shared.ErrInvalid
	}
	switch status {
	case "", "unmatched", "ignored", "requires_review", "applied":
	default:
		return nil, shared.ErrInvalid
	}
	if err := s.repo.Admin(ctx, user); err != nil {
		return nil, err
	}
	return s.repo.Receipts(ctx, status, page, limit)
}
