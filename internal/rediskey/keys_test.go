package rediskey

import "testing"

func TestKey(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  string
	}{
		{"refresh token", []string{"auth", "refresh", "token-hash"}, "ecommerce:auth:refresh:token-hash"},
		{"catalog cache", []string{"catalog", "product", "product-id"}, "ecommerce:catalog:product:product-id"},
		{"namespace", nil, "ecommerce:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Key(tt.parts...); got != tt.want {
				t.Fatalf("Key() = %q, want %q", got, tt.want)
			}
		})
	}
}
