package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTokenServiceAccessAndRefresh(t *testing.T) {
	service := NewTokenService("test-secret")
	id := uuid.New()

	access, err := service.Generate(id, "customer", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	accessClaims, err := service.Parse(access)
	if err != nil {
		t.Fatal(err)
	}
	if accessClaims.Subject != id.String() || accessClaims.Type != "access" {
		t.Fatalf("unexpected access claims: %+v", accessClaims)
	}

	refresh, err := service.GenerateRefresh(id, "customer", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	refreshClaims, err := service.Parse(refresh)
	if err != nil {
		t.Fatal(err)
	}
	if refreshClaims.Subject != id.String() || refreshClaims.Type != "refresh" {
		t.Fatalf("unexpected refresh claims: %+v", refreshClaims)
	}
}
