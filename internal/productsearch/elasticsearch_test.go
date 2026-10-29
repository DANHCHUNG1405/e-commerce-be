package productsearch

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestMappingUsesFoldedVietnameseTextAndNestedVariants(t *testing.T) {
	b, err := json.Marshal(Mapping())
	if err != nil {
		t.Fatal(err)
	}
	raw := string(b)
	for _, want := range []string{"asciifolding", "vi_folded", "vi_prefix", "completion", "\"type\":\"nested\""} {
		if !strings.Contains(raw, want) {
			t.Fatalf("mapping missing %q", want)
		}
	}
}

func TestBuildQueryKeepsPriceBoundsInsideOneVariant(t *testing.T) {
	min, max := int64(100), int64(500)
	category := uuid.New()
	seller := uuid.New()
	b, err := json.Marshal(BuildQuery("áo sơ mi", &seller, &category, &min, &max))
	if err != nil {
		t.Fatal(err)
	}
	q := string(b)
	for _, want := range []string{"\"path\":\"variants\"", "\"variants.price\"", "\"fuzzy\"", "\"sku\"", "\"category_ids\"", "\"seller_id\""} {
		if !strings.Contains(q, want) {
			t.Fatalf("query missing %q: %s", want, q)
		}
	}
	var tree any
	if err := json.Unmarshal(b, &tree); err != nil {
		t.Fatal(err)
	}
	assertFuzzyDoesNotTargetSKU(t, tree)
}

func assertFuzzyDoesNotTargetSKU(t *testing.T, node any) {
	t.Helper()
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			if key == "fuzzy" {
				encoded, _ := json.Marshal(child)
				if strings.Contains(string(encoded), "sku") {
					t.Fatal("SKU must not be fuzzy matched")
				}
			}
			assertFuzzyDoesNotTargetSKU(t, child)
		}
	case []any:
		for _, child := range value {
			assertFuzzyDoesNotTargetSKU(t, child)
		}
	}
}

func TestBuildSort(t *testing.T) {
	for _, kind := range []string{"newest", "price_asc", "price_desc", "best_selling", "rating", "relevance"} {
		if len(BuildSort(kind)) == 0 {
			t.Fatalf("sort %q is empty", kind)
		}
	}
	if got := BuildSort("relevance")[0]; got != "_score" {
		t.Fatalf("relevance first sort = %#v", got)
	}
}
