package main

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // RFC 6238 authenticator codes are defined over SHA-1.
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	totpPeriod = 30 // seconds
	// maxTOTPWait stays below the API server's 60s request timeout.
	maxTOTPWait = 40 * time.Second
)

// totpCode computes the RFC 6238 six-digit code for a base32 secret and counter.
func totpCode(secret string, counter uint64) (string, error) {
	normalized := strings.TrimRight(strings.ToUpper(strings.Join(strings.Fields(secret), "")), "=")
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(normalized)
	if err != nil {
		return "", fmt.Errorf("TOTP secret is not valid base32: %w", err)
	}

	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1_000_000), nil
}

// reserveTOTPCounter claims the next unused TOTP period, because INWX rejects a
// code that was already used. It returns how long to wait for that period.
//
// shortcut: one code per 30s per process across all accounts, so replicas can collide (the retry recovers); share or key this state if several MFA accounts or replicas become common.
func (s *inwxDNSSolver) reserveTOTPCounter(now time.Time) (uint64, time.Duration, error) {
	s.totpMu.Lock()
	defer s.totpMu.Unlock()

	counter := max(uint64(now.Unix())/totpPeriod, s.lastTOTPCounter+1)
	wait := time.Unix(int64(counter*totpPeriod), 0).Sub(now)
	if wait > maxTOTPWait {
		return 0, 0, errors.New("too many concurrent INWX 2FA logins, will be retried")
	}
	s.lastTOTPCounter = counter
	return counter, wait, nil
}

// unlock completes the login of an account protected by two-factor authentication.
func (s *inwxDNSSolver) unlock(client inwxClient, secret string) error {
	if secret == "" {
		return fmt.Errorf("INWX account requires 2FA but the credentials secret has no TOTP secret (key %q unless totpSecretKey is set)", defaultTOTPSecretKey)
	}
	counter, wait, err := s.reserveTOTPCounter(time.Now())
	if err != nil {
		return err
	}
	tan, err := totpCode(secret, counter)
	if err != nil {
		return err
	}
	time.Sleep(wait)
	if err := client.Unlock(tan); err != nil {
		return fmt.Errorf("failed to unlock INWX account with TOTP code: %w", err)
	}
	return nil
}
