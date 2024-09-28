package utils

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gomessguii/logger"
	whatsmeow_types "go.mau.fi/whatsmeow/types"
	"golang.org/x/net/proxy"
)

type Values struct {
	m map[string]string
}

type VCardStruct struct {
	FullName     string `json:"fullName"`
	Organization string `json:"organization"`
	Phone        string `json:"phone"`
}

func Find(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

func ParseJID(arg string) (whatsmeow_types.JID, bool) {
	if arg == "" {
		return whatsmeow_types.NewJID("", whatsmeow_types.DefaultUserServer), false
	}
	if arg[0] == '+' {
		arg = arg[1:]
	}

	containsHyphen := strings.Contains(arg, "-")
	endsWithTag := strings.HasSuffix(arg, "g.us")

	var recipient whatsmeow_types.JID
	if containsHyphen && endsWithTag {
		recipient, _ = whatsmeow_types.ParseJID(arg)
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
		recipient, _ := whatsmeow_types.ParseJID("")
		return recipient, false
	}

	if !strings.ContainsRune(arg, '@') {
		return whatsmeow_types.NewJID(arg, whatsmeow_types.DefaultUserServer), true
	} else {
		recipient, err := whatsmeow_types.ParseJID(arg)
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
		host := req.URL.Host
		if !strings.Contains(host, ":") {
			host = fmt.Sprintf("%s:443", host) // Adiciona porta padrão 443 se não especificada
		}
		conn, err := dialer.Dial("tcp", host)
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

func GenerateVC(data VCardStruct) string {
	result := `
BEGIN:VCARD
VERSION:3.0
FN:` + data.FullName + `
ORG:` + data.Organization + `;
TEL;type=CELL;type=VOICE;waid=` + data.Phone + `:` + data.Phone + `
END:VCARD`

	return result
}
