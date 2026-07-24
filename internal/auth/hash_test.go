package auth

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestBcryptHasher_ProducesHashAtCost12(t *testing.T) {
	got, err := BcryptHasher{}.Hash("hunter2!")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(got))
	if err != nil {
		t.Fatalf("bcrypt.Cost: %v", err)
	}
	if cost != 12 {
		t.Errorf("cost = %d, want 12", cost)
	}
}

func TestBcryptHasher_SaltingProducesDifferentHashes(t *testing.T) {
	h := BcryptHasher{}
	a, err := h.Hash("hunter2!")
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.Hash("hunter2!")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two hashes of the same password are identical; salting is broken")
	}
}

func TestBcryptHasher_HashVerifiesRoundTrip(t *testing.T) {
	got, err := BcryptHasher{}.Hash("hunter2!")
	if err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(got), []byte("hunter2!")); err != nil {
		t.Errorf("correct password rejected: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(got), []byte("wrong")); err == nil {
		t.Error("wrong password accepted")
	}
}

func TestBcryptHasher_LengthBoundaries(t *testing.T) {
	h := BcryptHasher{}

	cases := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"empty", "", false},
		{"single byte", "x", false},
		{"71 bytes", strings.Repeat("a", 71), false},
		{"72 bytes (bcrypt max)", strings.Repeat("a", 72), false},
		{"73 bytes (over bcrypt limit)", strings.Repeat("a", 73), true},
		{"1000 bytes", strings.Repeat("a", 1000), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.Hash(tc.password)
			if (err != nil) != tc.wantErr {
				t.Errorf("Hash(%d bytes): err = %v, wantErr = %v",
					len(tc.password), err, tc.wantErr)
			}
		})
	}
}
