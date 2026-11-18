package catalog

import (
	"testing"

	"github.com/google/uuid"
)

func TestValidImageKitProductURL(t *testing.T) {
	product := uuid.New()
	endpoint := "https://ik.imagekit.io/demo"
	cases := []struct {
		name, raw string
		valid     bool
	}{
		{"matching product folder", endpoint + "/products/" + product.String() + "/photo.webp", true},
		{"other host", "https://example.com/products/" + product.String() + "/photo.webp", false},
		{"other product", endpoint + "/products/" + uuid.NewString() + "/photo.webp", false},
		{"query string", endpoint + "/products/" + product.String() + "/photo.webp?x=1", false},
		{"nested path", endpoint + "/products/" + product.String() + "/folder/photo.webp", false},
		{"http", "http://ik.imagekit.io/demo/products/" + product.String() + "/photo.webp", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validImageKitProductURL(tc.raw, endpoint, product); got != tc.valid {
				t.Fatalf("validImageKitProductURL() = %v, want %v", got, tc.valid)
			}
		})
	}
}
