package lib

import (
	"errors"
	"testing"
)

func TestStaticTokenVerifier(t *testing.T) {
	tests := []struct {
		name      string
		configure string
		token     string
		wantErr   bool
	}{
		{"matching token", "secret-token", "secret-token", false},
		{"matching token with surrounding space", "  secret-token  ", "secret-token", false},
		{"wrong token", "secret-token", "other-token", true},
		{"empty request token", "secret-token", "", true},
		{"empty configured token", "", "secret-token", true},
		{"both empty", "", "", true},
		{"prefix", "secret-token", "secret", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			verifier := NewStaticTokenVerifier(tc.configure)
			err := verifier.Verify(tc.token)
			if tc.wantErr {
				if !errors.Is(err, ErrUnauthenticated) {
					t.Fatalf("error = %v, want ErrUnauthenticated", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
		})
	}
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		header string
		want   string
		ok     bool
	}{
		{"Bearer abc", "abc", true},
		{"bearer abc", "abc", true},
		{"  Bearer   abc  ", "abc", true},
		{"Bearer", "", false},
		{"Basic abc", "", false},
		{"abc", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.header, func(t *testing.T) {
			got, ok := bearerToken(tc.header)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("bearerToken(%q) = (%q, %v), want (%q, %v)", tc.header, got, ok, tc.want, tc.ok)
			}
		})
	}
}
