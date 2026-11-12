package lib

import (
	"testing"
	"time"
)

func TestSessionCodecRoundTrip(t *testing.T) {
	codec := NewSessionCodec("test-secret")
	value := codec.Encode("did:privy:merchant-1", time.Now().Add(time.Hour))
	userID, err := codec.Decode(value, time.Now())
	if err != nil {
		t.Fatalf("valid session rejected: %v", err)
	}
	if userID != "did:privy:merchant-1" {
		t.Fatalf("userID = %q, want did:privy:merchant-1", userID)
	}
}

func TestSessionCodecRejectsForgedValue(t *testing.T) {
	codec := NewSessionCodec("test-secret")
	value := codec.Encode("did:privy:merchant-1", time.Now().Add(time.Hour))
	// Flip the last signature character to simulate tampering.
	forged := value[:len(value)-1] + string(flipChar(value[len(value)-1]))
	if _, err := codec.Decode(forged, time.Now()); err == nil {
		t.Fatal("forged session was accepted")
	}
}

func TestSessionCodecRejectsExpiredSession(t *testing.T) {
	codec := NewSessionCodec("test-secret")
	value := codec.Encode("did:privy:merchant-1", time.Now().Add(-time.Minute))
	if _, err := codec.Decode(value, time.Now()); err == nil {
		t.Fatal("expired session was accepted")
	}
}

func TestSessionCodecRejectsOtherSecret(t *testing.T) {
	value := NewSessionCodec("secret-a").Encode("did:privy:merchant-1", time.Now().Add(time.Hour))
	if _, err := NewSessionCodec("secret-b").Decode(value, time.Now()); err == nil {
		t.Fatal("session signed with another secret was accepted")
	}
}

func TestSessionCodecRejectsNonPrivySubject(t *testing.T) {
	// A correctly signed cookie whose subject is not a Privy DID must be
	// rejected: only the passwordless flow may mint a subject.
	codec := NewSessionCodec("test-secret")
	value := codec.Encode("someone-else", time.Now().Add(time.Hour))
	if _, err := codec.Decode(value, time.Now()); err == nil {
		t.Fatal("session with a non-Privy subject was accepted")
	}
}

func TestSessionCodecRejectsMissingValueAndNilCodec(t *testing.T) {
	codec := NewSessionCodec("test-secret")
	if _, err := codec.Decode("", time.Now()); err == nil {
		t.Fatal("empty value was accepted")
	}
	if _, err := (*SessionCodec)(nil).Decode("anything", time.Now()); err == nil {
		t.Fatal("nil codec was accepted")
	}
	if codec := NewSessionCodec(""); codec != nil {
		t.Fatal("blank secret should disable sessions")
	}
}

func flipChar(c byte) byte {
	if c == 'A' {
		return 'B'
	}
	return 'A'
}
