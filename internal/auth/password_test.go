package auth

import "testing"

func TestPasswordHashAndVerify(t *testing.T) {
	t.Parallel()
	const password = "a correct long password"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == password {
		t.Fatal("password was stored in plaintext")
	}
	valid, err := VerifyPassword(hash, password)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !valid {
		t.Fatal("correct password did not verify")
	}
	valid, err = VerifyPassword(hash, "a different long password")
	if err != nil {
		t.Fatalf("VerifyPassword wrong password: %v", err)
	}
	if valid {
		t.Fatal("wrong password verified")
	}
}

func TestSessionTokenHashDoesNotReturnRawToken(t *testing.T) {
	t.Parallel()
	const token = "raw-secret-session-token"
	hash := HashSessionToken(token)
	if hash == token {
		t.Fatal("session hash equals raw token")
	}
	if len(hash) != 64 {
		t.Fatalf("expected SHA-256 hex hash length 64, got %d", len(hash))
	}
}
