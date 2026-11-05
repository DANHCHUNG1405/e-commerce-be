package assistant

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestProductFactsJSONOmitsInternalIdentifiers(t *testing.T) {
	product := ProductContext{
		ID: uuid.New(), SellerID: uuid.New(), Name: "Tai nghe",
		Description: "Tai nghe Bluetooth",
	}
	data, err := productFactsJSON(product)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), product.ID.String()) || strings.Contains(string(data), product.SellerID.String()) {
		t.Fatalf("product facts contain internal identifiers: %s", data)
	}
	var facts map[string]any
	if err := json.Unmarshal(data, &facts); err != nil {
		t.Fatal(err)
	}
	if facts["name"] != product.Name {
		t.Fatalf("product facts name = %v, want %q", facts["name"], product.Name)
	}
}

func TestProductFactsJSONBoundsLargeSpecifications(t *testing.T) {
	product := ProductContext{
		Name:           "Tai nghe",
		Description:    strings.Repeat("Mô tả ", 1000),
		Specifications: map[string]any{"long": strings.Repeat("x", 20000)},
		Variants:       []ProductVariantContext{{Name: "Đen", Attributes: map[string]any{"color": "đen"}}},
	}
	data, err := productFactsJSON(product)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 16000 {
		t.Fatalf("product facts payload is too large: %d bytes", len(data))
	}
	if strings.Contains(string(data), "specifications") || strings.Contains(string(data), "attributes") {
		t.Fatalf("oversized product metadata was not removed: %s", data)
	}
}

func TestSellerChatAction(t *testing.T) {
	sellerID := uuid.New()
	action := sellerChatAction(ProductContext{SellerID: sellerID, Name: "Tai nghe"})
	if action == nil || action.Type != "message_seller" || action.SellerID != sellerID {
		t.Fatalf("unexpected seller chat action: %#v", action)
	}
	if action.SuggestedMessage == "" {
		t.Fatal("seller chat action should contain a suggested message")
	}
}
