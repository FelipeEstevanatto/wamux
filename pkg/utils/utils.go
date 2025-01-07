package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gomessguii/logger"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
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

	// Limpa o número primeiro
	number := arg
	number = strings.ReplaceAll(number, " ", "")
	number = strings.ReplaceAll(number, "(", "")
	number = strings.ReplaceAll(number, ")", "")

	// Remove o + inicial se existir
	if number[0] == '+' {
		number = number[1:]
	}

	// Verifica se é um grupo pelo formato original
	containsHyphen := strings.Contains(number, "-")
	endsWithTag := strings.HasSuffix(number, "g.us")

	if containsHyphen && endsWithTag {
		recipient, _ := whatsmeow_types.ParseJID(number)
		return recipient, true
	}

	// Verifica formatos completos de JID
	// if strings.Contains(number, "@g.us") || strings.Contains(number, "@s.whatsapp.net") ||
	// 	strings.Contains(number, "@lid") || strings.Contains(number, "@broadcast") {
	if strings.Contains(number, "@g.us") ||
		strings.Contains(number, "@lid") ||
		strings.Contains(number, "@broadcast") {
		recipient, err := whatsmeow_types.ParseJID(number)
		if err != nil {
			logger.LogWarn("Invalid jid: %s", number)
			return recipient, false
		}
		return recipient, true
	}

	if strings.Contains(number, "@s.whatsapp.net") && !strings.Contains(number, ":") {
		number = strings.Split(number, "@")[0]
	} else if strings.Contains(number, "@s.whatsapp.net") && strings.Contains(number, ":") {
		recipient, err := whatsmeow_types.ParseJID(number)
		if err != nil {
			logger.LogWarn("Invalid jid: %s", number)
			return recipient, false
		}
		return recipient, true
	}

	// Limpa o número para processamento
	number = strings.Split(number, ":")[0]
	number = strings.Split(number, "@")[0]

	// Verifica se é um grupo pelo tamanho
	if strings.Contains(number, "-") && len(number) >= 24 {
		groupID := strings.Map(func(r rune) rune {
			if (r >= '0' && r <= '9') || r == '-' {
				return r
			}
			return -1
		}, number)
		return whatsmeow_types.NewJID(groupID, "g.us"), true
	}

	// Remove todos os caracteres não numéricos
	number = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, number)

	// Verifica se é um número válido
	if number == "" {
		logger.LogWarn("Bad jid format, return empty")
		recipient, _ := whatsmeow_types.ParseJID("")
		return recipient, false
	}

	if len(number) == 13 && strings.HasPrefix(number, "55") {
		// Extrai o DDD
		ddd := number[2:4]

		// Converte o DDD para inteiro
		dddNum, err := strconv.Atoi(ddd)
		if err != nil {
			// Retorna número inválido em caso de erro na conversão
			return whatsmeow_types.NewJID(number, whatsmeow_types.DefaultUserServer), false
		}

		// Verifica se o DDD é menor que 31
		if dddNum < 31 {
			// Retorna o número como válido
			return whatsmeow_types.NewJID(number, whatsmeow_types.DefaultUserServer), true
		}

		// Extrai partes do número (DDI e restante)
		ddi := number[0:2]
		numberEnd := number[5:]

		// Remove o '9' adicional após o DDD
		number = ddi + ddd + numberEnd
	}

	// Retorna o JID formatado
	if !strings.ContainsRune(number, '@') {
		return whatsmeow_types.NewJID(number, whatsmeow_types.DefaultUserServer), true
	} else {
		recipient, err := whatsmeow_types.ParseJID(number)
		if err != nil {
			logger.LogWarn("Invalid jid: %s", number)
			return recipient, false
		} else if recipient.User == "" {
			logger.LogError("Invalid jid. No user specified: %s", number)
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

func WhatsAppGetUserOS() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	default:
		return "Linux"
	}
}

func WhatsAppGetUserAgent(agentType string) waCompanionReg.DeviceProps_PlatformType {
	switch strings.ToLower(agentType) {
	case "desktop":
		return waCompanionReg.DeviceProps_DESKTOP
	case "mac":
		return waCompanionReg.DeviceProps_CATALINA
	case "android":
		return waCompanionReg.DeviceProps_ANDROID_AMBIGUOUS
	case "android-phone":
		return waCompanionReg.DeviceProps_ANDROID_PHONE
	case "andorid-tablet":
		return waCompanionReg.DeviceProps_ANDROID_TABLET
	case "ios-phone":
		return waCompanionReg.DeviceProps_IOS_PHONE
	case "ios-catalyst":
		return waCompanionReg.DeviceProps_IOS_CATALYST
	case "ipad":
		return waCompanionReg.DeviceProps_IPAD
	case "wearos":
		return waCompanionReg.DeviceProps_WEAR_OS
	case "ie":
		return waCompanionReg.DeviceProps_IE
	case "edge":
		return waCompanionReg.DeviceProps_EDGE
	case "chrome":
		return waCompanionReg.DeviceProps_CHROME
	case "safari":
		return waCompanionReg.DeviceProps_SAFARI
	case "firefox":
		return waCompanionReg.DeviceProps_FIREFOX
	case "opera":
		return waCompanionReg.DeviceProps_OPERA
	case "uwp":
		return waCompanionReg.DeviceProps_UWP
	case "aloha":
		return waCompanionReg.DeviceProps_ALOHA
	case "tv-tcl":
		return waCompanionReg.DeviceProps_TCL_TV
	default:
		return waCompanionReg.DeviceProps_UNKNOWN
	}
}
