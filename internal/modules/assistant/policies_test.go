package assistant

import (
	"reflect"
	"testing"
)

func TestPolicyTerms(t *testing.T) {
	got := policyTerms("Tôi muốn hỏi chính sách đổi trả sản phẩm, đổi trả bao lâu?")
	want := []string{"đổi", "trả", "lâu"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("policyTerms() = %#v, want %#v", got, want)
	}
}

func TestPolicyTermsEmptyAndPunctuation(t *testing.T) {
	if got := policyTerms("tôi, mình, hỏi về?"); len(got) != 0 {
		t.Fatalf("policyTerms() = %#v, want no terms", got)
	}
}
