package main

import (
	"testing"
	"time"
)

func TestTOTPCode_RFC6238Vectors(t *testing.T) {
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" // base32 of "12345678901234567890"
	tests := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1234567890, "005924"},
		{2000000000, "279037"},
	}
	for _, tc := range tests {
		got, err := totpCode(secret, uint64(tc.unix)/totpPeriod)
		if err != nil {
			t.Fatalf("totpCode(%d): %v", tc.unix, err)
		}
		if got != tc.want {
			t.Errorf("totpCode(%d) = %q, want %q", tc.unix, got, tc.want)
		}
	}
}

func TestTOTPCode_AcceptsAuthenticatorFormatting(t *testing.T) {
	want, _ := totpCode("GEZDGNBVGY3TQOJQ", 1)
	got, err := totpCode(" gezd gnbv gy3t qojq==\n", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTOTPCode_InvalidSecret(t *testing.T) {
	if _, err := totpCode("not-base32!", 1); err == nil {
		t.Fatal("expected error for invalid base32")
	}
}

func TestReserveTOTPCounter_NeverReusesAPeriod(t *testing.T) {
	s := &inwxDNSSolver{}
	now := time.Unix(300, 0)

	counter, wait, err := s.reserveTOTPCounter(now)
	if err != nil || counter != 10 || wait != 0 {
		t.Fatalf("first: counter=%d wait=%v err=%v", counter, wait, err)
	}
	counter, wait, err = s.reserveTOTPCounter(now)
	if err != nil || counter != 11 || wait != 30*time.Second {
		t.Fatalf("second: counter=%d wait=%v err=%v", counter, wait, err)
	}
	if _, _, err = s.reserveTOTPCounter(now); err == nil {
		t.Fatal("third: expected error when the wait exceeds the limit")
	}
	// The refused call must not consume a period.
	counter, _, err = s.reserveTOTPCounter(now.Add(30 * time.Second))
	if err != nil || counter != 12 {
		t.Fatalf("after refusal: counter=%d err=%v", counter, err)
	}
}
