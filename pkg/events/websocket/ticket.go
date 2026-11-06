package websocket_producer

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

// TicketTTL is how long a /ws ticket works. A browser cannot set headers on a
// WebSocket, so the credential has to go in the URL, and URLs reach proxy logs.
// A ticket found there has expired by the time anyone reads it.
const TicketTTL = 30 * time.Second

// IssueTicket returns a short-lived /ws credential. It is signed rather than
// stored, so any node behind a load balancer can accept it. It is not single use:
// it can open more than one socket until it expires.
func IssueTicket(secret string, now time.Time) (string, error) {
	if secret == "" {
		return "", errors.New("no key to sign the ticket with")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	body := strconv.FormatInt(now.Add(TicketTTL).Unix(), 10) + "." + hex.EncodeToString(nonce)
	return body + "." + signTicket(secret, body), nil
}

// ValidTicket reports whether ticket was signed with secret and has not expired.
func ValidTicket(secret, ticket string, now time.Time) bool {
	if secret == "" {
		return false
	}
	cut := strings.LastIndexByte(ticket, '.')
	if cut < 0 {
		return false
	}
	body, signature := ticket[:cut], ticket[cut+1:]
	if !hmac.Equal([]byte(signature), []byte(signTicket(secret, body))) {
		return false
	}
	expiry, _, found := strings.Cut(body, ".")
	if !found {
		return false
	}
	expires, err := strconv.ParseInt(expiry, 10, 64)
	return err == nil && now.Unix() < expires
}

func signTicket(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("wamux-ws-ticket|" + body))
	return hex.EncodeToString(mac.Sum(nil))
}
