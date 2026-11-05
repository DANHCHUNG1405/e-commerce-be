package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/example/e-commerce-be/internal/modules/assistant"
	"github.com/example/e-commerce-be/internal/modules/catalog"
	"github.com/example/e-commerce-be/internal/modules/order"
	"github.com/example/e-commerce-be/internal/modules/shared"
	"github.com/google/uuid"
)

type supportLookup struct {
	catalog  *catalog.Service
	orders   *order.Service
	sellers  shared.SellerDirectory
	policies *assistant.PolicyRepository
}

var _ assistant.Lookup = (*supportLookup)(nil)

func (l *supportLookup) SearchPolicies(ctx context.Context, query string) ([]assistant.PolicyChunk, error) {
	return l.policies.Search(ctx, query)
}

func (l *supportLookup) SearchProducts(ctx context.Context, query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 200 {
		return "Bạn hãy cho mình biết tên hoặc từ khóa sản phẩm cần tìm.", nil
	}
	ctx = shared.WithSellerDirectory(ctx, l.sellers)
	products, err := l.catalog.Search(ctx, catalog.Filter{Query: query}, 1, 5)
	if err != nil {
		return "", err
	}
	if len(products) == 0 {
		return "Mình chưa tìm thấy sản phẩm phù hợp với từ khóa đó. Bạn thử tên hoặc từ khóa khác nhé.", nil
	}
	var b strings.Builder
	b.WriteString("Sản phẩm phù hợp trong cửa hàng:\n")
	for _, product := range products {
		detail, err := l.catalog.Detail(ctx, product.ID)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "\n- %s", product.Name)
		if product.Description != "" {
			fmt.Fprintf(&b, ": %s", trimText(product.Description, 350))
		}
		for i, variant := range detail.Variants {
			if i == 5 {
				break
			}
			fmt.Fprintf(&b, "\n  • %s | %s VND | còn %d", variant.Name, formatVND(variant.Price), variant.Stock)
		}
	}
	if b.String() == "Sản phẩm phù hợp trong cửa hàng:\n" {
		return "Mình chưa tìm thấy thông tin sản phẩm có thể hiển thị. Bạn thử từ khóa khác nhé.", nil
	}
	return b.String(), nil
}

func (l *supportLookup) ListMyOrders(ctx context.Context, user uuid.UUID) (string, error) {
	orders, err := l.orders.List(ctx, user, 1, 5)
	if err != nil {
		return "", err
	}
	if len(orders) == 0 {
		return "Tài khoản của bạn chưa có đơn hàng.", nil
	}
	var b strings.Builder
	b.WriteString("Các đơn hàng gần đây của bạn:")
	for _, item := range orders {
		fmt.Fprintf(&b, "\n- Mã đơn %s | trạng thái %s | %s VND | ngày %s", item.ID, item.Status, formatVND(item.Total), item.CreatedAt.UTC().Format("2006-01-02"))
	}
	return b.String(), nil
}

func (l *supportLookup) GetMyOrder(ctx context.Context, user, id uuid.UUID) (string, error) {
	detail, err := l.orders.Detail(ctx, user, id)
	if err != nil {
		return "Không tìm thấy đơn hàng thuộc tài khoản của bạn với mã đó.", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Đơn hàng %s\nTrạng thái: %s\nTổng tiền: %s VND\nThanh toán: %s (%s)", detail.Order.ID, detail.Order.Status, formatVND(detail.Order.Total), detail.Payment.Method, detail.Payment.Status)
	for i, sellerOrder := range detail.Sellers {
		if i == 20 {
			break
		}
		fmt.Fprintf(&b, "\nTrạng thái xử lý của shop %s: %s", sellerOrder.SellerID, sellerOrder.Status)
	}
	if len(detail.Items) > 0 {
		b.WriteString("\nSản phẩm:")
		for i, item := range detail.Items {
			if i == 20 {
				break
			}
			fmt.Fprintf(&b, "\n- %s / %s × %d", item.ProductName, item.VariantName, item.Quantity)
		}
	}
	return b.String(), nil
}

func formatVND(amount int64) string { return fmt.Sprintf("%d", amount) }
func trimText(value string, limit int) string {
	r := []rune(strings.TrimSpace(value))
	if len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return string(r)
}
