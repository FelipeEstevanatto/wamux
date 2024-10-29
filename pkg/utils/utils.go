package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gomessguii/logger"
	whatsmeow_types "go.mau.fi/whatsmeow/types"
	"golang.org/x/exp/rand"
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

func GenerateRandomString(length int) string {
	characters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = characters[rand.Intn(len(characters))]
	}
	return string(b)
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
	v, ok := values.(Values)
	if !ok {
		logger.LogError("Failed to cast values to Values type")
		return values
	}

	logger.LogDebug("User info updated field: %s value: %s", field, value)
	v.m[field] = value
	return v
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

func GetObject(message []byte, keyFind string) string {
	var messageMap map[string]interface{}
	err := json.Unmarshal(message, &messageMap)
	if err != nil {
		logger.LogError("failed to unmarshal message: %s", err)
		return ""
	}
	for key, value := range messageMap {
		if key == keyFind {
			if captionStr, ok := value.(string); ok {
				return captionStr
			}
		}

		if nestedMap, ok := value.(map[string]interface{}); ok {
			nestedMapBytes, err := json.Marshal(nestedMap)
			if err != nil {
				logger.LogError("failed to marshal nestedMap: %s", err)
				continue
			}
			if caption := GetObject(nestedMapBytes, keyFind); caption != "" {
				return caption
			}
		}
	}
	return ""
}
