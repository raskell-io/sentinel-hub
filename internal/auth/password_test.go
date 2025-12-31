package auth

import (
	"testing"
)

func TestHashPassword(t *testing.T) {
	password := "TestPassword123"
	hash, err := HashPassword(password, DefaultBcryptCost)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == "" {
		t.Fatal("HashPassword returned empty hash")
	}

	// Hash should be different from password
	if hash == password {
		t.Fatal("Hash should not equal password")
	}
}

func TestVerifyPassword(t *testing.T) {
	password := "TestPassword123"
	hash, err := HashPassword(password, DefaultBcryptCost)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	// Correct password should verify
	err = VerifyPassword(hash, password)
	if err != nil {
		t.Errorf("VerifyPassword failed for correct password: %v", err)
	}

	// Wrong password should fail
	err = VerifyPassword(hash, "WrongPassword123")
	if err == nil {
		t.Error("VerifyPassword should fail for wrong password")
	}
}

func TestValidatePasswordStrength(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{
			name:     "valid password",
			password: "TestPassword123",
			wantErr:  nil,
		},
		{
			name:     "too short",
			password: "Test1",
			wantErr:  ErrPasswordTooShort,
		},
		{
			name:     "no uppercase",
			password: "testpassword123",
			wantErr:  ErrPasswordNoUppercase,
		},
		{
			name:     "no lowercase",
			password: "TESTPASSWORD123",
			wantErr:  ErrPasswordNoLowercase,
		},
		{
			name:     "no digit",
			password: "TestPassword",
			wantErr:  ErrPasswordNoDigit,
		},
		{
			name:     "exactly 8 characters",
			password: "Test123a",
			wantErr:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePasswordStrength(tt.password)
			if err != tt.wantErr {
				t.Errorf("ValidatePasswordStrength(%q) = %v, want %v", tt.password, err, tt.wantErr)
			}
		})
	}
}

func TestGenerateRandomToken(t *testing.T) {
	token1, err := GenerateRandomToken(32)
	if err != nil {
		t.Fatalf("GenerateRandomToken failed: %v", err)
	}

	if len(token1) == 0 {
		t.Fatal("GenerateRandomToken returned empty token")
	}

	// Generate another token - should be different
	token2, err := GenerateRandomToken(32)
	if err != nil {
		t.Fatalf("GenerateRandomToken failed: %v", err)
	}

	if token1 == token2 {
		t.Error("GenerateRandomToken returned same token twice")
	}
}

func TestHashToken(t *testing.T) {
	token := "test-token-123"
	hash := HashToken(token)

	if hash == "" {
		t.Fatal("HashToken returned empty hash")
	}

	if hash == token {
		t.Fatal("HashToken should not return the same value as input")
	}

	// Same input should produce same hash
	hash2 := HashToken(token)
	if hash != hash2 {
		t.Error("HashToken should be deterministic")
	}

	// Different input should produce different hash
	hash3 := HashToken("different-token")
	if hash == hash3 {
		t.Error("Different tokens should produce different hashes")
	}
}
