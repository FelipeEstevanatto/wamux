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

// TicketScope is what a validated ticket is allowed to subscribe to.
type TicketScope struct {
	// InstanceID is the only instance the socket may receive. Empty means global.
	InstanceID string
	// Global tickets may choose any instance (or all of them) at connect time.
	Global bool
}

// IssueTicket returns a short-lived global /ws credential. It is signed rather
// than stored, so any node behind a load balancer can accept it. It is not single
// use: it can open more than one socket until it expires.
func IssueTicket(secret string, now time.Time) (string, error) {
	return issueTicket(secret, "", now)
}

// IssueInstanceTicket returns a ticket bound to one instance. Even if the socket
// asks for another instanceId, ServeWs uses the bound one.
func IssueInstanceTicket(secret, instanceID string, now time.Time) (string, error) {
	if strings.TrimSpace(instanceID) == "" {
		return "", errors.New("instance id is required")
	}
	return issueTicket(secret, instanceID, now)
}

func issueTicket(secret, instanceID string, now time.Time) (string, error) {
	if secret == "" {
		return "", errors.New("no key to sign the ticket with")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	body := strconv.FormatInt(now.Add(TicketTTL).Unix(), 10) + "." + hex.EncodeToString(nonce)
	if instanceID != "" {
		// Hex keeps the UUID out of the dot-separated body. An attacker cannot
		// change it without the signature.
		body += "." + hex.EncodeToString([]byte(instanceID))
	}
	return body + "." + signTicket(secret, body), nil
}

// ParseTicket validates a ticket and returns its scope.
func ParseTicket(secret, ticket string, now time.Time) (TicketScope, bool) {
	if secret == "" {
		return TicketScope{}, false
	}
	cut := strings.LastIndexByte(ticket, '.')
	if cut < 0 {
		return TicketScope{}, false
	}
	body, signature := ticket[:cut], ticket[cut+1:]
	if !hmac.Equal([]byte(signature), []byte(signTicket(secret, body))) {
		return TicketScope{}, false
	}
	parts := strings.Split(body, ".")
	if len(parts) != 2 && len(parts) != 3 {
		return TicketScope{}, false
	}
	expires, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || now.Unix() >= expires {
		return TicketScope{}, false
	}
	if len(parts) == 2 {
		return TicketScope{Global: true}, true
	}
	instance, err := hex.DecodeString(parts[2])
	if err != nil || len(instance) == 0 {
		return TicketScope{}, false
	}
	return TicketScope{InstanceID: string(instance)}, true
}

// ValidTicket reports whether ticket was signed with secret and has not expired.
// It accepts both global and instance-scoped tickets; callers that need the scope
// should use ParseTicket.
func ValidTicket(secret, ticket string, now time.Time) bool {
	_, ok := ParseTicket(secret, ticket, now)
	return ok
}

func signTicket(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("wamux-ws-ticket|" + body))
	return hex.EncodeToString(mac.Sum(nil))
}
