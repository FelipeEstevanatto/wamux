package utils

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gomessguii/logger"
	"go.mau.fi/whatsmeow/types"
	"golang.org/x/net/proxy"
)

var messageTypes = map[string]bool{
	"MESSAGE":       true,
	"READ_RECEIPT":  true,
	"PRESENCE":      true,
	"HISTORY_SYNC":  true,
	"CHAT_PRESENCE": true,
	"CALL":          true,
	"ALL":           true,
}

type Values struct {
	m map[string]string
}

func ValidateEvent(event string) bool {
	return messageTypes[event]
}

func Find(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

func ParseJID(arg string) (types.JID, bool) {
	if arg == "" {
		return types.NewJID("", types.DefaultUserServer), false
	}
	if arg[0] == '+' {
		arg = arg[1:]
	}

	containsHyphen := strings.Contains(arg, "-")
	endsWithTag := strings.HasSuffix(arg, "g.us")

	var recipient types.JID
	if containsHyphen && endsWithTag {
		recipient, _ = types.ParseJID(arg)
		return recipient, true
	}

	// Basic only digit check for recipient phone number, we want to remove @server and .session
	phonenumber := ""
	phonenumber = strings.Split(arg, "@")[0]
	phonenumber = strings.Split(phonenumber, ":")[0]
	b := true
	for _, c := range phonenumber {
		if c < '0' || c > '9' {
			b = false
			break
		}
	}
	if !b {
		logger.LogWarn("Bad jid format, return empty")
		recipient, _ := types.ParseJID("")
		return recipient, false
	}

	if !strings.ContainsRune(arg, '@') {
		return types.NewJID(arg, types.DefaultUserServer), true
	} else {
		recipient, err := types.ParseJID(arg)
		if err != nil {
			logger.LogWarn("Invalid jid: %s", arg)
			return recipient, false
		} else if recipient.User == "" {
			logger.LogError("Invalid jid. No user specified: %s", arg)
			return recipient, false
		}
		return recipient, true
	}
}

func CreateSocks5Proxy(socks5Host, socks5Port, user, password string) (func(*http.Request) (*url.URL, error), error) {
	auth := &proxy.Auth{
		User:     user,
		Password: password,
	}

	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("%s:%s", socks5Host, socks5Port), auth, proxy.Direct)
	if err != nil {
		return nil, err
	}

	return func(req *http.Request) (*url.URL, error) {
		conn, err := dialer.Dial("tcp", req.URL.Host)
		if err != nil {
			return nil, err
		}
		defer conn.Close()

		return nil, nil
	}, nil
}

func UpdateUserInfo(values interface{}, field string, value string) interface{} {
	logger.LogDebug("User info updated field: %s value: %s", field, value)
	values.(Values).m[field] = value
	return values
}

func TimestampToUnixInt(timestamp string) (int64, error) {
	layout := "2006-01-02 15:04:05"

	t, err := time.Parse(layout, timestamp)
	if err != nil {
		return 0, err
	}

	unixTimestamp := t.Unix()

	return unixTimestamp, nil
}
