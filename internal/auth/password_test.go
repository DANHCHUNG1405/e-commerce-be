package auth

import "testing"

func TestPasswordHash(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if VerifyPassword(hash, "correct horse battery staple") != nil {
		t.Fatal("hashed password did not verify")
	}
	if VerifyPassword(hash, "wrong password") == nil {
		t.Fatal("wrong password was accepted")
	}
}
