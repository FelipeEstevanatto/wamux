package instance_service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/evolution-foundation/evolution-go/pkg/safemap"
	"github.com/evolution-foundation/evolution-go/pkg/tokencrypt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	minio_storage "github.com/evolution-foundation/evolution-go/pkg/storage/minio"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"github.com/evolution-foundation/evolution-go/pkg/walimits"
	"github.com/evolution-foundation/evolution-go/pkg/webhooksign"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
	"github.com/google/uuid"
	"github.com/patrickmn/go-cache"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type InstanceService interface {
	Create(data *CreateStruct) (*instance_model.Instance, error)
	// EncryptExistingTokens backfills any instance still storing its token in
	// plaintext (called once at startup after an upgrade).
	EncryptExistingTokens()
	Connect(data *ConnectStruct, instance *instance_model.Instance) (*instance_model.Instance, string, string, error)
	Reconnect(instance *instance_model.Instance) error
	Disconnect(instance *instance_model.Instance) (*instance_model.Instance, error)
	Logout(instance *instance_model.Instance) (*instance_model.Instance, error)
	Status(instance *instance_model.Instance) (*StatusStruct, error)
	GetQr(instance *instance_model.Instance) (*QrcodeStruct, error)
	Pair(data *PairStruct, instance *instance_model.Instance) (*PairReturnStruct, error)
	GetAll() ([]*instance_model.Instance, error)
	Info(instanceId string) (*instance_model.Instance, error)
	Rename(instanceId string, name string) (*instance_model.Instance, error)
	Delete(id string) error
	SetProxy(id string, proxyConfig *ProxyConfig) error
	SetProxyFromStruct(id string, data *SetProxyStruct) error
	GetProxy(id string) (*ProxyConfig, error)
	TestProxy(cfg *ProxyConfig) (*ProxyTestResult, error)
	ReconnectProxy(id string) error
	RemoveProxy(id string) error
	GetLimits(instanceId string) (*LimitsStruct, error)
	ForceReconnect(instanceId string, number string) error
	GetInstanceByToken(token string) (*instance_model.Instance, error)
	GetLogs(instanceId string, startDate, endDate time.Time, level string, limit int) ([]logger_wrapper.LogEntry, error)
	GetAdvancedSettings(instanceId string) (*instance_model.AdvancedSettings, error)
	UpdateAdvancedSettings(instanceId string, settings *instance_model.AdvancedSettings) error

	// Webhook HMAC signing. The key is validated, encrypted and stored; the
	// plaintext never leaves the process after the request returns.
	SetWebhookHmacKey(instanceId string, key string) (*HmacConfigStatus, error)
	ClearWebhookHmacKey(instanceId string) error
	WebhookHmacStatus(instanceId string) (*HmacConfigStatus, error)

	// Per-instance S3 media storage. The secret key is encrypted at rest and
	// never returned.
	SetS3Config(instanceId string, data *S3ConfigStruct) (*S3ConfigStatus, error)
	GetS3Config(instanceId string) (*S3ConfigStatus, error)
	DeleteS3Config(instanceId string) error
	TestS3Connection(instanceId string, data *S3ConfigStruct) (*S3TestResult, error)
}

// S3ConfigStruct is the request body for the per-instance S3 endpoints.
type S3ConfigStruct struct {
	Enabled       bool   `json:"enabled" example:"true"`
	Endpoint      string `json:"endpoint" example:"https://s3.amazonaws.com"`
	Region        string `json:"region" example:"us-east-1"`
	Bucket        string `json:"bucket" example:"my-whatsapp-media"`
	AccessKey     string `json:"accessKey" example:"AKIA..."`
	SecretKey     string `json:"secretKey" example:"wJalr..."`
	PathStyle     bool   `json:"pathStyle" example:"false"`
	PublicURL     string `json:"publicUrl" example:"https://cdn.example.com"`
	MediaDelivery string `json:"mediaDelivery" example:"both"`
}

// S3ConfigStatus is the non-sensitive view of the config. The secret itself is
// never returned, only whether one is stored.
type S3ConfigStatus struct {
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint"`
	Region        string `json:"region"`
	Bucket        string `json:"bucket"`
	AccessKey     string `json:"accessKey"`
	SecretKeySet  bool   `json:"secretKeySet"`
	PathStyle     bool   `json:"pathStyle"`
	PublicURL     string `json:"publicUrl"`
	MediaDelivery string `json:"mediaDelivery"`
}

// S3TestResult is returned by the S3 connection test on success.
type S3TestResult struct {
	Bucket string `json:"bucket"`
	Region string `json:"region"`
}

// HmacConfigStatus is the (non-sensitive) HMAC configuration summary returned by
// the instance endpoints. It never includes the key itself.
type HmacConfigStatus struct {
	// Configured is true when the instance has its own signing key.
	Configured bool `json:"configured"`
	// GlobalFallback is true when a process-global key would be used instead.
	GlobalFallback bool `json:"globalFallback"`
}

type instances struct {
	instanceRepository instance_repository.InstanceRepository
	config             *config.Config
	killChannel        *safemap.Map[chan bool]
	clientPointer      *safemap.Map[*whatsmeow.Client]
	whatsmeowService   whatsmeow_service.WhatsmeowService
	loggerWrapper      *logger_wrapper.LoggerManager

	// authCache memoizes token -> instance for the auth middleware, which runs
	// on every authenticated request. The lookup is indexed and cheap, but it is
	// still a Postgres round trip on the hottest path, so a short TTL removes
	// almost all of them. Every write that can change an instance row flushes it
	// (see invalidateAuthCache), so the only staleness is a sub-second window on
	// a concurrent write, never a deleted token.
	authCache *cache.Cache

	// tokenCodec encrypts instance tokens at rest and computes the deterministic
	// lookup hash used by authentication. Nil only if no key could be derived
	// (then lookups fall back to the legacy plaintext column).
	tokenCodec *tokencrypt.Codec
}

// authCacheTTL bounds how long a token lookup may be served from memory.
const authCacheTTL = 10 * time.Second

// ReachoutTimelockStruct reports whether the account is barred from messaging new
// contacts, and until when. Behind WhatsApp error 463.
type ReachoutTimelockStruct struct {
	IsActive            bool   `json:"isActive"`
	TimeEnforcementEnds int64  `json:"timeEnforcementEnds"` // unix seconds
	EnforcementType     string `json:"enforcementType"`
}

// NewChatCappingStruct is the account's quota for starting brand-new chats.
type NewChatCappingStruct struct {
	CappingStatus string `json:"cappingStatus"`
	TotalQuota    int    `json:"totalQuota"`
	UsedQuota     int    `json:"usedQuota"`
	CycleEnds     int64  `json:"cycleEnds"` // unix seconds
}

// LimitsStruct aggregates WhatsApp's account-level messaging limits for an instance.
type LimitsStruct struct {
	ReachoutTimelock *ReachoutTimelockStruct `json:"reachoutTimelock"`
	NewChatCapping   *NewChatCappingStruct   `json:"newChatCapping"`
}

type ProxyConfig struct {
	Protocol string `json:"protocol,omitempty"`
	Port     string `json:"port"`
	Password string `json:"password"`
	Username string `json:"username"`
	Host     string `json:"host"`
}

type CreateStruct struct {
	InstanceId       string                           `json:"instanceId"`
	Name             string                           `json:"name"`
	Token            string                           `json:"token"`
	Proxy            *ProxyConfig                     `json:"proxy"`
	AdvancedSettings *instance_model.AdvancedSettings `json:"advancedSettings"`
}

type ConnectStruct struct {
	WebhookUrl      string   `json:"webhookUrl"`
	Subscribe       []string `json:"subscribe"`
	Immediate       bool     `json:"immediate"`
	Phone           string   `json:"phone"`
	RabbitmqEnable  string   `json:"rabbitmqEnable"`
	WebSocketEnable string   `json:"websocketEnable"`
	NatsEnable      string   `json:"natsEnable"`
}

type StatusStruct struct {
	Connected bool
	LoggedIn  bool
	myJid     *types.JID
	Name      string
}

type QrcodeStruct struct {
	Qrcode string `json:"qrcode"`
	Code   string `json:"code"`
	// Passkey ceremony fields. Populated when the account requires a WebAuthn
	// passkey to finish linking (no QR to scan at that point). The manager uses
	// PasskeyStage to switch its UI and PasskeyOpenUrl for the
	// "Abrir WhatsApp Web" button that launches the passkey ceremony.
	PasskeyStage   string `json:"passkeyStage,omitempty"`
	PasskeyOpenURL string `json:"passkeyOpenUrl,omitempty"`
	PasskeyCode    string `json:"passkeyCode,omitempty"`
}

type PairStruct struct {
	Subscribe []string `json:"subscribe"`
	Phone     string   `json:"phone"`
}

type PairReturnStruct struct {
	PairingCode string
}

type SetProxyStruct struct {
	Protocol string `json:"protocol,omitempty"`
	Host     string `json:"host" validate:"required"`
	Port     string `json:"port" validate:"required"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type ForceReconnectStruct struct {
	Number string `json:"number"`
}

func (i *instances) ensureClientConnected(instanceId string) (*whatsmeow.Client, error) {
	logger := i.loggerWrapper.GetLogger(instanceId)
	client := i.clientPointer.Get(instanceId)
	logger.LogInfo("[%s] Checking client connection status - Client exists: %v", instanceId, client != nil)

	if client == nil {
		logger.LogInfo("[%s] No client found, attempting to start new instance", instanceId)
		err := i.whatsmeowService.StartInstance(instanceId)
		if err != nil {
			logger.LogError("[%s] Failed to start instance: %v", instanceId, err)
			return nil, errors.New("no active session found")
		}

		logger.LogInfo("[%s] Instance started, waiting for the connection...", instanceId)
		client = i.waitForClient(instanceId, 10*time.Second)

		logger.LogInfo("[%s] Checking new client - Exists: %v, Connected: %v",
			instanceId,
			client != nil,
			client != nil && client.IsConnected())

		if client == nil || !client.IsConnected() {
			logger.LogError("[%s] New client validation failed - Exists: %v, Connected: %v",
				instanceId,
				client != nil,
				client != nil && client.IsConnected())
			return nil, errors.New("no active session found")
		}
	} else if !client.IsConnected() {
		logger.LogError("[%s] Existing client is disconnected - Connected status: %v",
			instanceId,
			client.IsConnected())
		return nil, errors.New("client disconnected")
	}

	logger.LogInfo("[%s] Client successfully validated - Connected: %v", instanceId, client.IsConnected())
	return client, nil
}

func (i instances) Create(data *CreateStruct) (*instance_model.Instance, error) {
	// Trim the name before the duplicate check and the insert. A name stored with
	// leading/trailing spaces cannot be matched later (routes and lookups trim),
	// so it would be effectively unreachable.
	data.Name = strings.TrimSpace(data.Name)
	if data.Name == "" {
		return nil, errors.New("name is required")
	}

	if data.Proxy != nil {
		data.Proxy.Protocol = utils.NormalizeProxyProtocol(data.Proxy.Protocol, data.Proxy.Port)
	}

	proxyJson, err := json.Marshal(data.Proxy)
	if err != nil {
		return nil, err
	}

	findInstance, _ := i.instanceRepository.GetInstanceByName(data.Name)

	if findInstance != nil {
		return nil, fmt.Errorf("instance already exists")
	}

	// Deployment-wide cap so a single tenant (or a runaway script) cannot create
	// unbounded instances — each costs a WhatsApp socket and memory. 0 disables.
	if i.config.MaxInstances > 0 {
		existing, err := i.instanceRepository.GetAll(i.config.ClientName)
		if err != nil {
			return nil, fmt.Errorf("could not check the instance limit: %w", err)
		}
		if len(existing) >= i.config.MaxInstances {
			return nil, fmt.Errorf("instance limit reached (%d)", i.config.MaxInstances)
		}
	}

	instance := instance_model.Instance{
		Id:         data.InstanceId,
		Name:       data.Name,
		Token:      data.Token,
		OsName:     i.config.OsName,
		Proxy:      string(proxyJson),
		Connected:  false,
		ClientName: i.config.ClientName,
	}

	// Set advanced settings if provided (nil pointers are left as defaults).
	if data.AdvancedSettings != nil {
		if data.AdvancedSettings.AlwaysOnline != nil {
			instance.AlwaysOnline = *data.AdvancedSettings.AlwaysOnline
		}
		if data.AdvancedSettings.RejectCall != nil {
			instance.RejectCall = *data.AdvancedSettings.RejectCall
		}
		instance.MsgRejectCall = data.AdvancedSettings.MsgRejectCall
		if data.AdvancedSettings.ReadMessages != nil {
			instance.ReadMessages = *data.AdvancedSettings.ReadMessages
		}
		if data.AdvancedSettings.IgnoreGroups != nil {
			instance.IgnoreGroups = *data.AdvancedSettings.IgnoreGroups
		}
		if data.AdvancedSettings.IgnoreStatus != nil {
			instance.IgnoreStatus = *data.AdvancedSettings.IgnoreStatus
		}
	}

	// Encrypt the token before persisting, so the plaintext credential is never
	// written. The plaintext column is left empty for new rows.
	if i.tokenCodec != nil {
		instance.TokenHash = i.tokenCodec.Hash(data.Token)
		enc, encErr := i.tokenCodec.Encrypt(data.Token)
		if encErr != nil {
			return nil, encErr
		}
		instance.TokenEnc = enc
		instance.Token = ""
	}

	createdInstance, err := i.instanceRepository.Create(instance)
	if err != nil {
		return nil, err
	}

	i.invalidateAuthCache()
	// The plaintext token was never persisted; put it back on the returned object
	// so the API response still shows it to the caller who just created it.
	if createdInstance != nil && createdInstance.Token == "" {
		createdInstance.Token = data.Token
	}
	return createdInstance, nil
}

func (i instances) Connect(data *ConnectStruct, instance *instance_model.Instance) (*instance_model.Instance, string, string, error) {
	i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Processing subscribe events: %v", instance.Id, data.Subscribe)

	oldEvents := instance.Events
	oldRabbitmq := instance.RabbitmqEnable

	updates := applyConnectSettings(instance, data)

	if instance.Events != oldEvents {
		i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] events changed: %q -> %q", instance.Id, oldEvents, instance.Events)
	}
	if instance.RabbitmqEnable != oldRabbitmq {
		i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] rabbitmqEnable changed: %q -> %q", instance.Id, oldRabbitmq, instance.RabbitmqEnable)
	}

	if len(updates) > 0 {
		err := i.instanceRepository.UpdateConnectSettings(instance.Id, updates)
		if err != nil {
			i.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error updating instance: %s", instance.Id, err)
			return nil, "", "", err
		}
		// events/rabbitmq/nats changed: the auth-cached row is now stale.
		i.invalidateAuthCache()
	}

	subscribedEvents := splitSubscribedEvents(instance.Events)
	eventString := instance.Events

	// Verifica se a instância já está rodando
	isInstanceRunning := i.clientPointer.Get(instance.Id) != nil

	// Sincroniza as configurações na instância em execução (se já estiver conectada)
	err := i.whatsmeowService.UpdateInstanceSettings(instance.Id)
	if err != nil {
		i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Instance not in runtime yet, will be updated when connected", instance.Id)
		isInstanceRunning = false
	} else {
		i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Instance settings updated successfully in runtime", instance.Id)
		isInstanceRunning = true
	}

	// Se a instância não estiver rodando, inicia uma nova
	if !isInstanceRunning {
		i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Starting new client instance", instance.Id)

		i.killChannel.Set(instance.Id, make(chan bool))

		clientData := &whatsmeow_service.ClientData{
			Instance:      instance,
			Subscriptions: subscribedEvents,
			Phone:         data.Phone,
			IsProxy:       false,
		}

		if instance.Proxy != "" || i.config.ProxyHost != "" {
			var proxyConfig ProxyConfig
			err := json.Unmarshal([]byte(instance.Proxy), &proxyConfig)
			if err != nil {
				i.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error unmarshalling proxy config: %v", instance.Id, err)
				return nil, "", "", err
			}

			if proxyConfig.Host != "" || i.config.ProxyHost != "" {
				clientData.IsProxy = true
			}
		}

		go i.whatsmeowService.StartClient(clientData)
	} else {
		i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Instance already running, settings updated without restarting client", instance.Id)
	}

	return instance, instance.Jid, eventString, nil
}

func (i instances) Reconnect(instance *instance_model.Instance) error {
	_, err := i.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}

	return i.whatsmeowService.ReconnectClient(instance.Id)
}

func (i instances) Disconnect(instance *instance_model.Instance) (*instance_model.Instance, error) {
	client, err := i.ensureClientConnected(instance.Id)
	if err != nil {
		return instance, err
	}

	if client.IsConnected() {
		if client.IsLoggedIn() {
			i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Disconnection successful", instance.Id)
			i.killChannel.Get(instance.Id) <- true

			// Do not clear instance.Events on disconnect. Wiping subscriptions
			// leaves the instance "connected" after reconnect with an empty
			// events string, and CallWebhook then drops every webhook (Go's
			// strings.Split("", ",") yields [""], which fails IsEventType).
			instance.Connected = false
			instance.DisconnectReason = "Disconnected by API"
			if err := i.instanceRepository.UpdateConnected(instance.Id, false, instance.DisconnectReason); err != nil {
				return instance, err
			}
			i.invalidateAuthCache()

			return instance, nil
		}
	}

	i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Ignoring disconnect as it was not connected", instance.Id)
	return instance, nil
}

func (i instances) Logout(instance *instance_model.Instance) (*instance_model.Instance, error) {
	client, err := i.ensureClientConnected(instance.Id)
	if err != nil {
		return instance, err
	}

	if client.IsLoggedIn() && client.IsConnected() {
		err := client.Logout(context.Background())
		if err != nil {
			return instance, err
		}

		instance.Connected = false
		err = i.instanceRepository.Update(instance)
		if err != nil {
			return instance, err
		}
		i.invalidateAuthCache()

		select {
		case i.killChannel.Get(instance.Id) <- true:
		case <-time.After(5 * time.Second):
		}

		i.clientPointer.Delete(instance.Id)
		i.killChannel.Delete(instance.Id)

		i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Logout successful", instance.Id)
		return instance, nil
	}

	if client.IsConnected() {
		client.Disconnect()

		select {
		case i.killChannel.Get(instance.Id) <- true:
		case <-time.After(5 * time.Second):
		}

		i.clientPointer.Delete(instance.Id)
		i.killChannel.Delete(instance.Id)

		i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Disconnection successful", instance.Id)
		return instance, nil
	}

	i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Ignoring logout as it was not connected", instance.Id)
	return instance, fmt.Errorf("ignoring logout as it was not connected")
}

func (i instances) Status(instance *instance_model.Instance) (*StatusStruct, error) {
	client := i.clientPointer.Get(instance.Id)

	if client == nil {
		return &StatusStruct{
			Connected: false,
			LoggedIn:  false,
		}, nil
	}

	isConnected := client.IsConnected()
	isLoggedIn := client.IsLoggedIn()

	var myJid *types.JID
	var name string
	if isLoggedIn {
		myJid = client.Store.ID
		name = client.Store.PushName
	}

	return &StatusStruct{
		Connected: isConnected,
		LoggedIn:  isLoggedIn,
		myJid:     myJid,
		Name:      name,
	}, nil
}

func (i instances) GetQr(instance *instance_model.Instance) (*QrcodeStruct, error) {
	logger := i.loggerWrapper.GetLogger(instance.Id)
	client := i.clientPointer.Get(instance.Id)

	// Se não há cliente ou o cliente está logado, precisamos iniciar um novo cliente
	if client == nil || client.IsLoggedIn() {
		if client != nil && client.IsLoggedIn() {
			logger.LogInfo("[%s] Client is logged in, starting new instance for QR code", instance.Id)
		} else {
			logger.LogInfo("[%s] No client found, starting new instance for QR code", instance.Id)
		}

		// Iniciar nova instância para gerar QR code
		err := i.whatsmeowService.StartInstance(instance.Id)
		if err != nil {
			logger.LogError("[%s] Failed to start instance: %v", instance.Id, err)
			return nil, fmt.Errorf("failed to start instance: %w", err)
		}

		// Aguardar um pouco para o cliente iniciar e gerar QR code
		logger.LogInfo("[%s] Waiting for QR code generation...", instance.Id)
		time.Sleep(3 * time.Second)

		// Verificar novamente se há cliente
		client = i.clientPointer.Get(instance.Id)
		if client != nil && client.IsLoggedIn() {
			return nil, fmt.Errorf("session already logged in")
		}
	} else if !client.IsConnected() {
		// Se o cliente existe mas não está conectado, pode estar aguardando QR code
		logger.LogInfo("[%s] Client exists but not connected, checking for existing QR code", instance.Id)
	}

	// Buscar instância atualizada do banco para pegar o QR code mais recente
	instance, err := i.instanceRepository.GetInstanceByID(instance.Id)
	if err != nil {
		return nil, err
	}

	// If a passkey ceremony is in progress, there is no QR to scan — return the
	// passkey stage + the #wapk openUrl so the manager can render the
	// "Abrir WhatsApp Web" button. Checked before the empty-QR branch because
	// during a passkey ceremony instance.Qrcode is empty.
	if store := i.whatsmeowService.PasskeyCeremonyStore(); store != nil {
		if token, state, ok := store.StateByInstance(instance.Id); ok {
			logger.LogInfo("[%s] Passkey ceremony active (stage=%s) — returning passkey info instead of QR", instance.Id, state.Stage)
			return &QrcodeStruct{
				PasskeyStage:   state.Stage,
				PasskeyCode:    state.Code,
				PasskeyOpenURL: buildPasskeyOpenURL(token),
			}, nil
		}
	}

	code := instance.Qrcode
	if code == "" {
		// Se não há QR code ainda, aguardar um pouco mais e tentar novamente
		logger.LogInfo("[%s] No QR code available yet, waiting a bit more...", instance.Id)
		time.Sleep(2 * time.Second)

		instance, err = i.instanceRepository.GetInstanceByID(instance.Id)
		if err != nil {
			return nil, err
		}

		code = instance.Qrcode
		if code == "" {
			return nil, fmt.Errorf("no QR code available. Please wait a moment and try again")
		}
	}

	parts := strings.Split(code, "|")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid QR code format")
	}

	qr := &QrcodeStruct{
		Qrcode: parts[0],
		Code:   parts[1],
	}

	return qr, nil
}

// buildPasskeyOpenURL builds the URL the manager opens to start the passkey
// ceremony: https://web.whatsapp.com/#wapk=<base64url({t:token,b:publicBase})>.
// publicBase must be the PUBLICLY reachable API base the browser can hit; set it
// via PASSKEY_PUBLIC_URL. Kept in sync with the event handler in whatsmeow.go.
func buildPasskeyOpenURL(token string) string {
	publicBase := os.Getenv("PASSKEY_PUBLIC_URL")
	if publicBase == "" {
		publicBase = "<SET_PASSKEY_PUBLIC_URL>"
	}
	payload := fmt.Sprintf(`{"t":%q,"b":%q}`, token, publicBase)
	wapk := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return "https://web.whatsapp.com/#wapk=" + wapk
}

func (i instances) Pair(data *PairStruct, instance *instance_model.Instance) (*PairReturnStruct, error) {
	logger := i.loggerWrapper.GetLogger(instance.Id)
	client := i.clientPointer.Get(instance.Id)

	if client == nil || !client.IsConnected() {
		if client != nil && client.IsLoggedIn() {
			return nil, fmt.Errorf("instance is already authenticated")
		}
		logger.LogInfo("[%s] No active connection, starting instance for phone pairing", instance.Id)
		if err := i.whatsmeowService.StartInstance(instance.Id); err != nil {
			logger.LogError("[%s] Failed to start instance for pairing: %v", instance.Id, err)
			return nil, fmt.Errorf("failed to start instance: %w", err)
		}
		// Wait for the WA websocket connection and initial QR generation to establish.
		// PairPhone must be called after the QR event is received per whatsmeow docs.
		time.Sleep(3 * time.Second)
		client = i.clientPointer.Get(instance.Id)
		if client == nil {
			return nil, fmt.Errorf("failed to initialize client for pairing")
		}
	}

	if client.IsLoggedIn() {
		return nil, fmt.Errorf("instance is already authenticated")
	}

	code, err := client.PairPhone(context.Background(), data.Phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		logger.LogError("[%s] PairPhone failed: %v", instance.Id, err)
		return nil, fmt.Errorf("pairing failed: %w", err)
	}

	return &PairReturnStruct{PairingCode: code}, nil
}

func (i instances) GetAll() ([]*instance_model.Instance, error) {
	instances, err := i.instanceRepository.GetAll(i.config.ClientName)
	if err != nil {
		return nil, err
	}

	for _, instance := range instances {
		if client := i.clientPointer.Get(instance.Id); client != nil {
			instance.Connected = client.IsLoggedIn()
		} else {
			instance.Connected = false
		}

		instance.Proxy = ""
	}

	return instances, nil
}

func (i instances) Info(instanceId string) (*instance_model.Instance, error) {
	instance, err := i.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		return nil, err
	}

	// Atualiza o status connected com base no estado real do cliente
	if client := i.clientPointer.Get(instance.Id); client != nil {
		instance.Connected = client.IsLoggedIn()
	} else {
		instance.Connected = false
	}

	instance.Proxy = ""

	return instance, nil
}

type RenameStruct struct {
	Name string `json:"name"`
}

// Rename changes the instance label. The id and token stay the same, so no
// integration needs reconfiguring; the runtime is synced so webhooks carry the
// new instanceName immediately.
func (i instances) Rename(instanceId string, name string) (*instance_model.Instance, error) {
	instance, err := i.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		return nil, err
	}

	if existing, _ := i.instanceRepository.GetInstanceByName(name); existing != nil && existing.Id != instance.Id {
		return nil, fmt.Errorf("an instance with this name already exists")
	}

	if err := i.instanceRepository.UpdateName(instance.Id, name); err != nil {
		return nil, err
	}
	instance.Name = name
	i.invalidateAuthCache()

	if err := i.whatsmeowService.UpdateInstanceSettings(instance.Id); err != nil {
		// A disconnected instance has no runtime to update; do not fail the rename.
		i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Error syncing renamed instance to runtime: %v", instance.Id, err)
	}

	instance.Proxy = ""

	return instance, nil
}

func (i instances) Delete(id string) error {
	instance, err := i.instanceRepository.GetInstanceByID(id)
	if err != nil {
		return err
	}

	if i.clientPointer.Get(instance.Id) != nil && i.clientPointer.Get(instance.Id).IsConnected() {
		if i.clientPointer.Get(instance.Id).IsLoggedIn() {
			i.clientPointer.Get(instance.Id).Logout(context.Background())
		}
		i.clientPointer.Get(instance.Id).Disconnect()
	}

	// Limpar todos os recursos da instância antes de deletar
	i.clientPointer.Delete(instance.Id)
	if i.killChannel.Get(instance.Id) != nil {
		close(i.killChannel.Get(instance.Id))
		i.killChannel.Delete(instance.Id)
	}

	// Limpar cache via whatsmeow service
	err = i.whatsmeowService.ClearInstanceCache(instance.Id, instance.Token)
	if err != nil {
		i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Failed to clear instance cache: %v", instance.Id, err)
	}

	// Remove the device (sessions, keys, contacts) from the whatsmeow store, so
	// deleting an instance does not leave credentials behind.
	if err := i.whatsmeowService.DeleteInstanceDevice(instance.Id, instance.Jid); err != nil {
		i.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Failed to delete device store: %v", instance.Id, err)
	}

	err = i.instanceRepository.Delete(id)
	if err != nil {
		return err
	}

	// The token must stop authenticating immediately, not after authCacheTTL.
	i.invalidateAuthCache()
	return nil
}

// GetProxy returns the proxy configuration saved for an instance, or nil when
// none is set. Admin-only route — credentials are included so the manager can
// pre-fill the form.
func (i instances) GetProxy(id string) (*ProxyConfig, error) {
	instance, err := i.instanceRepository.GetInstanceByID(id)
	if err != nil {
		return nil, err
	}

	if instance.Proxy == "" {
		return nil, nil
	}

	var proxyConfig ProxyConfig
	if err := json.Unmarshal([]byte(instance.Proxy), &proxyConfig); err != nil {
		i.loggerWrapper.GetLogger(id).LogError("[%s] Failed to unmarshal stored proxy config: %v", id, err)
		return nil, fmt.Errorf("stored proxy configuration is invalid: %w", err)
	}

	// Older rows may hold the literal "null" or an object without a host, which
	// unmarshals into a zero struct. Report those as "no proxy configured".
	if proxyConfig.Host == "" {
		return nil, nil
	}

	return &proxyConfig, nil
}

// ReconnectProxy re-establishes the WhatsApp connection using the proxy already
// saved for the instance, without changing the stored configuration. Useful when
// the proxy dropped and the socket needs to be rebuilt through it.
func (i instances) ReconnectProxy(id string) error {
	proxyConfig, err := i.GetProxy(id)
	if err != nil {
		return err
	}

	if proxyConfig == nil || proxyConfig.Host == "" {
		return fmt.Errorf("no proxy configured for this instance")
	}

	i.loggerWrapper.GetLogger(id).LogInfo(
		"[%s] Reconnecting through proxy %s://%s:%s",
		id, proxyConfig.Protocol, proxyConfig.Host, proxyConfig.Port,
	)

	// Operator-triggered — give the instance a fresh automatic retry budget.
	whatsmeow_service.ClearReconnectBackoff(id)

	return i.whatsmeowService.ReconnectClient(id)
}

// GetLimits returns WhatsApp's reachout timelock and new-chat messaging quota for
// an instance — the account-level limits behind error 463. It serves the value
// cached on connect (the MEX queries are slow/rate-limited); on a cache miss it
// does a live query with a short timeout so the HTTP request never hangs.
func (i instances) GetLimits(instanceId string) (*LimitsStruct, error) {
	if e, ok := whatsmeow_service.GetCachedAccountLimits(instanceId); ok {
		result := &LimitsStruct{
			ReachoutTimelock: &ReachoutTimelockStruct{
				IsActive:            e.ReachoutActive,
				TimeEnforcementEnds: e.ReachoutEnds,
				EnforcementType:     e.ReachoutType,
			},
		}
		if e.CappingStatus != "" {
			result.NewChatCapping = &NewChatCappingStruct{
				CappingStatus: e.CappingStatus,
				TotalQuota:    e.TotalQuota,
				UsedQuota:     e.UsedQuota,
				CycleEnds:     e.CycleEnds,
			}
		}
		return result, nil
	}

	client, err := i.ensureClientConnected(instanceId)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	result := &LimitsStruct{}

	if tl, err := walimits.GetAccountReachoutTimelock(ctx, client); err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Failed to fetch reachout timelock: %v", instanceId, err)
	} else if tl != nil {
		var ends int64
		if tl.IsActive {
			ends = tl.TimeEnforcementEnds.Unix()
		}
		result.ReachoutTimelock = &ReachoutTimelockStruct{
			IsActive:            tl.IsActive,
			TimeEnforcementEnds: ends,
			EnforcementType:     string(tl.EnforcementType),
		}
	}

	if capping, err := walimits.GetNewChatMessageCappingInfo(ctx, client); err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Failed to fetch new-chat capping info: %v", instanceId, err)
	} else if capping != nil {
		result.NewChatCapping = &NewChatCappingStruct{
			CappingStatus: string(capping.CappingStatus),
			TotalQuota:    capping.TotalQuota,
			UsedQuota:     capping.UsedQuota,
			CycleEnds:     capping.CycleEndTimestamp.Unix(),
		}
	}

	return result, nil
}

func (i instances) SetProxy(id string, proxyConfig *ProxyConfig) error {
	instance, err := i.instanceRepository.GetInstanceByID(id)
	if err != nil {
		return err
	}

	// Validate proxy configuration
	if proxyConfig == nil {
		return fmt.Errorf("proxy configuration cannot be nil")
	}

	if proxyConfig.Host == "" {
		return fmt.Errorf("proxy host is required")
	}

	if proxyConfig.Port == "" {
		return fmt.Errorf("proxy port is required")
	}

	proxyConfig.Protocol = utils.NormalizeProxyProtocol(proxyConfig.Protocol, proxyConfig.Port)

	// An empty password means "keep the current one": GET /instance/proxy no
	// longer returns the stored password, so the UI cannot resend it.
	if proxyConfig.Password == "" && instance.Proxy != "" {
		var existing ProxyConfig
		if err := json.Unmarshal([]byte(instance.Proxy), &existing); err == nil && existing.Host != "" {
			proxyConfig.Password = existing.Password
		}
	}

	// Convert proxy config to JSON
	proxyJSON, err := json.Marshal(proxyConfig)
	if err != nil {
		i.loggerWrapper.GetLogger(id).LogError("[%s] Failed to marshal proxy config: %v", id, err)
		return fmt.Errorf("failed to marshal proxy configuration: %v", err)
	}

	instance.Proxy = string(proxyJSON)

	// Update instance in database
	err = i.instanceRepository.Update(instance)
	if err != nil {
		i.loggerWrapper.GetLogger(id).LogError("[%s] Failed to update instance with proxy: %v", id, err)
		return err
	}

	i.loggerWrapper.GetLogger(id).LogInfo("[%s] Proxy configuration updated: %s://%s:%s", id, proxyConfig.Protocol, proxyConfig.Host, proxyConfig.Port)

	i.invalidateAuthCache()

	// Reconnect to apply proxy changes
	go i.Reconnect(instance)

	return nil
}

func (i instances) SetProxyFromStruct(id string, data *SetProxyStruct) error {
	if data == nil {
		return fmt.Errorf("proxy data cannot be nil")
	}

	proxyConfig := &ProxyConfig{
		Protocol: data.Protocol,
		Host:     data.Host,
		Port:     data.Port,
		Username: data.Username,
		Password: data.Password,
	}

	return i.SetProxy(id, proxyConfig)
}

func (i instances) RemoveProxy(id string) error {
	instance, err := i.instanceRepository.GetInstanceByID(id)
	if err != nil {
		return err
	}

	instance.Proxy = ""

	err = i.instanceRepository.Update(instance)
	if err != nil {
		return err
	}

	i.loggerWrapper.GetLogger(id).LogInfo("[%s] Proxy configuration removed", id)

	i.invalidateAuthCache()

	go i.Reconnect(instance)

	return nil
}

func (i instances) ForceReconnect(instanceId string, number string) error {
	// The client is often absent here (that is the state this endpoint exists to
	// recover from), so Get may return nil — never dereference it blindly.
	if client := i.clientPointer.Get(instanceId); client != nil && client.IsConnected() && client.IsLoggedIn() {
		return fmt.Errorf("client already connected")
	}

	err := i.whatsmeowService.ForceUpdateJid(instanceId, number)
	if err != nil {
		return err
	}

	instance, err := i.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		return err
	}

	subscribedEvents := strings.Split(instance.Events, ",")

	i.killChannel.Set(instance.Id, make(chan bool))

	clientData := &whatsmeow_service.ClientData{
		Instance:      instance,
		Subscriptions: subscribedEvents,
		Phone:         "",
		IsProxy:       false,
	}

	if instance.Proxy != "" || i.config.ProxyHost != "" {
		var proxyConfig ProxyConfig
		err := json.Unmarshal([]byte(instance.Proxy), &proxyConfig)
		if err != nil {
			i.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error unmarshalling proxy config: %v", instance.Id, err)
			return err
		}

		if proxyConfig.Host != "" || i.config.ProxyHost != "" {
			clientData.IsProxy = true
		}
	}

	if i.clientPointer.Get(instance.Id) != nil {
		client := i.clientPointer.Get(instance.Id)
		client.Disconnect()

		select {
		case i.killChannel.Get(instance.Id) <- true:
		case <-time.After(5 * time.Second):
		}

		i.clientPointer.Delete(instance.Id)
		i.killChannel.Delete(instance.Id)
	}

	go i.whatsmeowService.StartClient(clientData)

	// Wait for the client to actually connect and log in instead of guessing
	// with a fixed sleep. StartClient runs asynchronously, so this first waits
	// for the client object to appear, then for the socket to be ready.
	client := i.waitForClient(instance.Id, 10*time.Second)
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("failed to connect")
	}
	if !client.IsLoggedIn() {
		return fmt.Errorf("failed to login")
	}

	return nil
}

// waitForClient waits for the instance's client object to exist and for its
// socket to be connected and logged in, up to timeout. It replaces the fixed
// sleeps that used to guess how long a connection takes — whatsmeow's
// WaitForConnection is the supported way to wait, and returns as soon as the
// socket is ready (or immediately on an expected disconnect).
func (i instances) waitForClient(instanceId string, timeout time.Duration) *whatsmeow.Client {
	deadline := time.Now().Add(timeout)
	for {
		if client := i.clientPointer.Get(instanceId); client != nil {
			client.WaitForConnection(time.Until(deadline))
			return client
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (i instances) GetInstanceByToken(token string) (*instance_model.Instance, error) {
	if token != "" && i.authCache != nil {
		if v, ok := i.authCache.Get(token); ok {
			if cached, ok := v.(instance_model.Instance); ok {
				// Hand back a copy: the cached value is shared across requests and
				// handlers must not be able to mutate it.
				instance := cached
				return &instance, nil
			}
		}
	}

	// Prefer the encrypted path: look the instance up by the deterministic HMAC of
	// the presented token, so the plaintext credential is never stored or queried.
	// Fall back to the legacy plaintext column only when no codec is configured.
	var instance *instance_model.Instance
	var err error
	if i.tokenCodec != nil {
		instance, err = i.instanceRepository.GetInstanceByTokenHash(i.tokenCodec.Hash(token))
		if err != nil || instance == nil {
			// Rows created before encryption existed have no hash yet; try the
			// legacy column and backfill, so the upgrade is transparent.
			if legacy, legacyErr := i.instanceRepository.GetInstanceByToken(token); legacyErr == nil && legacy != nil {
				if hashErr := i.backfillToken(legacy, token); hashErr != nil {
					i.loggerWrapper.GetLogger(legacy.Id).LogWarn("[%s] Could not encrypt token on the fly: %v", legacy.Id, hashErr)
				}
				return legacy, nil
			}
			return nil, err
		}
	} else {
		instance, err = i.instanceRepository.GetInstanceByToken(token)
		if err != nil {
			return nil, err
		}
	}

	if token != "" && i.authCache != nil && instance != nil {
		i.authCache.Set(token, *instance, cache.DefaultExpiration)
	}

	return instance, nil
}

// backfillToken encrypts a legacy plaintext token in place: it stores the hash
// and ciphertext and clears the plaintext column, so the credential is no longer
// readable in a database dump. Best-effort: a failure leaves the row usable via
// the legacy column.
func (i instances) backfillToken(instance *instance_model.Instance, plaintext string) error {
	if i.tokenCodec == nil || instance == nil || plaintext == "" {
		return nil
	}
	hash := i.tokenCodec.Hash(plaintext)
	enc, err := i.tokenCodec.Encrypt(plaintext)
	if err != nil {
		return err
	}
	// Write hash + ciphertext AND blank the plaintext column in one update, so
	// there is never a moment where the token is stored only in the clear.
	if err := i.instanceRepository.UpdateToken(instance.Id, hash, enc); err != nil {
		return err
	}
	instance.TokenHash = hash
	instance.TokenEnc = enc
	instance.Token = ""
	i.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Instance token encrypted at rest", instance.Id)
	return nil
}

// EncryptExistingTokens backfills every instance whose token is still plaintext.
// Called once at startup so an upgraded deployment stops storing credentials in
// the clear without waiting for each instance to authenticate again.
func (i instances) EncryptExistingTokens() {
	if i.tokenCodec == nil {
		return
	}
	all, err := i.instanceRepository.GetAll(i.config.ClientName)
	if err != nil {
		i.loggerWrapper.GetLogger("startup").LogWarn("[TOKEN] Could not list instances for token encryption: %v", err)
		return
	}
	migrated := 0
	for _, inst := range all {
		if inst == nil || inst.Token == "" {
			continue // already encrypted (plaintext cleared) or nothing to do
		}
		if err := i.backfillToken(inst, inst.Token); err != nil {
			i.loggerWrapper.GetLogger(inst.Id).LogWarn("[%s] Failed to encrypt token: %v", inst.Id, err)
			continue
		}
		migrated++
	}
	if migrated > 0 {
		i.invalidateAuthCache()
		i.loggerWrapper.GetLogger("startup").LogInfo("[TOKEN] Encrypted %d instance token(s) at rest", migrated)
	}
}

// that can change an instance row, so a rename, settings change or delete is
// visible to the very next request rather than after authCacheTTL.
func (i instances) invalidateAuthCache() {
	if i.authCache != nil {
		i.authCache.Flush()
	}
}

func (i instances) GetLogs(instanceId string, startDate, endDate time.Time, level string, limit int) ([]logger_wrapper.LogEntry, error) {
	// Inicializa o slice vazio para garantir que nunca retorne null
	logs := make([]logger_wrapper.LogEntry, 0)

	// The id is used as a path segment below, so reject anything that is not a
	// UUID: without this, "../.." would escape the log directory.
	if _, err := uuid.Parse(instanceId); err != nil {
		return logs, fmt.Errorf("invalid instance id")
	}

	// Per-instance logs are written asynchronously, so make sure everything
	// already emitted for this instance has reached the file before reading it.
	i.loggerWrapper.Flush(instanceId)

	// Define valores padrão
	if limit <= 0 {
		limit = 100 // Limite padrão de 100 registros
	}

	// Se não foi fornecida data inicial, usa 7 dias atrás
	if startDate.IsZero() {
		startDate = time.Now().AddDate(0, 0, -7)
	}

	// Se não foi fornecida data final, usa data atual
	if endDate.IsZero() {
		endDate = time.Now()
	}

	// Ajusta as datas para início e fim do dia
	startDate = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.UTC)
	endDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, time.UTC)

	// Garante que a data inicial não seja posterior à data final
	if startDate.After(endDate) {
		return logs, fmt.Errorf("data inicial não pode ser posterior à data final")
	}

	// Níveis de log válidos
	validLevels := map[string]bool{
		"INFO":  true,
		"ERROR": true,
		"WARN":  true,
		"DEBUG": true,
	}

	var levelArray []string
	if level == "" {
		// Se nenhum nível foi especificado, usa todos
		levelArray = []string{"INFO", "ERROR", "WARN", "DEBUG"}
	} else {
		// Divide e normaliza os níveis fornecidos
		for _, l := range strings.Split(level, ",") {
			l = strings.TrimSpace(strings.ToUpper(l))
			if !validLevels[l] {
				return logs, fmt.Errorf("nível de log inválido: %s", l)
			}
			levelArray = append(levelArray, l)
		}
	}

	// Lê os logs do arquivo
	logPath := filepath.Join(i.config.LogDirectory, instanceId, "instance.log")
	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return logs, nil // Retorna array vazio se arquivo não existir
		}
		return logs, fmt.Errorf("erro ao abrir arquivo de log: %v", err)
	}
	defer file.Close()

	// Read the newest matching entries from the end of the file. The log file can
	// grow to hundreds of MB; scanning it from the start to collect the first
	// `limit` matches both read the whole file and returned the OLDEST entries.
	size, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return logs, fmt.Errorf("erro ao posicionar no arquivo de log: %v", err)
	}

	return readLogTail(file, size, startDate, endDate, levelArray, limit)
}

// readLogTail reads up to limit entries matching the date/level filters, newest
// first, by scanning the file backwards in chunks. It returns as soon as it has
// collected limit entries or reached the start of the file, so a dashboard
// request for the latest logs never reads the whole file.
func readLogTail(file *os.File, size int64, startDate, endDate time.Time, levelArray []string, limit int) ([]logger_wrapper.LogEntry, error) {
	logs := make([]logger_wrapper.LogEntry, 0, limit)

	const chunkSize = 64 * 1024
	var carry []byte // a partial line carried over from the previously read chunk
	offset := size

	for offset > 0 && len(logs) < limit {
		readSize := int64(chunkSize)
		if offset < readSize {
			readSize = offset
		}
		offset -= readSize

		buf := make([]byte, readSize)
		if _, err := file.ReadAt(buf, offset); err != nil && err != io.EOF {
			return logs, err
		}

		// The carry is the tail of the line that started in this chunk and
		// continued into the chunk read before it.
		data := append(buf, carry...)
		lines := bytes.Split(data, []byte{'\n'})

		if offset > 0 {
			// The first piece has no leading newline: it is a partial line.
			carry = append([]byte(nil), lines[0]...)
			lines = lines[1:]
		} else {
			carry = nil
		}

		// Lines are in file order inside the chunk; walk them backwards.
		for idx := len(lines) - 1; idx >= 0; idx-- {
			line := bytes.TrimSpace(lines[idx])
			if len(line) == 0 {
				continue
			}

			var entry logger_wrapper.LogEntry
			if err := json.Unmarshal(line, &entry); err != nil {
				continue // Ignora linhas inválidas
			}

			entry.Timestamp = entry.Timestamp.UTC()
			if entry.Timestamp.Before(startDate) || entry.Timestamp.After(endDate) {
				continue
			}
			if !slices.Contains(levelArray, entry.Level) {
				continue
			}

			logs = append(logs, entry)
			if len(logs) >= limit {
				break
			}
		}
	}

	return logs, nil
}

func (i instances) GetAdvancedSettings(instanceId string) (*instance_model.AdvancedSettings, error) {
	i.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Getting advanced settings", instanceId)

	settings, err := i.instanceRepository.GetAdvancedSettings(instanceId)
	if err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error getting advanced settings: %v", instanceId, err)
		return nil, err
	}

	return settings, nil
}

func (i instances) UpdateAdvancedSettings(instanceId string, settings *instance_model.AdvancedSettings) error {
	i.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Updating advanced settings", instanceId)

	err := i.instanceRepository.UpdateAdvancedSettings(instanceId, settings)
	if err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error updating advanced settings: %v", instanceId, err)
		return err
	}
	i.invalidateAuthCache()

	// Sincroniza as configurações na instância em execução
	err = i.whatsmeowService.UpdateInstanceAdvancedSettings(instanceId)
	if err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Error syncing advanced settings to runtime: %v", instanceId, err)
		// Não falha a operação, apenas loga o warning
	}

	i.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Advanced settings updated successfully", instanceId)
	return nil
}

// SetWebhookHmacKey validates, encrypts and stores an instance's webhook
// signing key, and pushes the plaintext to the running client so the next
// delivery is signed. The stored value is never returned.
func (i instances) SetWebhookHmacKey(instanceId string, key string) (*HmacConfigStatus, error) {
	key = strings.TrimSpace(key)
	if len(key) < webhooksign.MinKeyLength {
		return nil, fmt.Errorf("hmac key must be at least %d characters", webhooksign.MinKeyLength)
	}

	if _, err := i.instanceRepository.GetInstanceByID(instanceId); err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Cannot set webhook HMAC key: %v", instanceId, err)
		return nil, err
	}

	encrypted, err := webhooksign.Encrypt(i.config.WebhookHmacEncryptionKey, key)
	if err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to encrypt webhook HMAC key: %v", instanceId, err)
		return nil, err
	}

	if err := i.instanceRepository.UpdateHmacKey(instanceId, encrypted); err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to store webhook HMAC key: %v", instanceId, err)
		return nil, err
	}

	// Update the running client and the auth cache so the key is used at once.
	i.whatsmeowService.SetWebhookHmacKey(instanceId, key)
	i.invalidateAuthCache()

	i.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Webhook HMAC key configured", instanceId)
	return i.WebhookHmacStatus(instanceId)
}

// ClearWebhookHmacKey removes an instance's signing key, reverting it to the
// global key (if any) or unsigned deliveries.
func (i instances) ClearWebhookHmacKey(instanceId string) error {
	if _, err := i.instanceRepository.GetInstanceByID(instanceId); err != nil {
		return err
	}
	if err := i.instanceRepository.UpdateHmacKey(instanceId, ""); err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to clear webhook HMAC key: %v", instanceId, err)
		return err
	}
	i.whatsmeowService.SetWebhookHmacKey(instanceId, "")
	i.invalidateAuthCache()
	i.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Webhook HMAC key cleared", instanceId)
	return nil
}

// WebhookHmacStatus reports whether an instance has its own key and whether a
// global fallback exists, without revealing either.
func (i instances) WebhookHmacStatus(instanceId string) (*HmacConfigStatus, error) {
	instance, err := i.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		return nil, err
	}
	return &HmacConfigStatus{
		Configured:     instance.HmacKey != "",
		GlobalFallback: i.config.WebhookHmacGlobalKey != "",
	}, nil
}

// normalizeMediaDelivery validates the per-instance media delivery mode.
// Empty defaults to "base64" (the pre-existing behaviour).
func normalizeMediaDelivery(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return "base64", nil
	case "base64":
		return "base64", nil
	case "s3":
		return "s3", nil
	case "both":
		return "both", nil
	default:
		return "", fmt.Errorf("invalid mediaDelivery %q (valid: base64, s3, both)", mode)
	}
}

func s3StatusFromInstance(inst *instance_model.Instance) *S3ConfigStatus {
	return &S3ConfigStatus{
		Enabled:       inst.S3Enabled,
		Endpoint:      inst.S3Endpoint,
		Region:        inst.S3Region,
		Bucket:        inst.S3Bucket,
		AccessKey:     inst.S3AccessKey,
		SecretKeySet:  inst.S3SecretKey != "",
		PathStyle:     inst.S3PathStyle,
		PublicURL:     inst.S3PublicURL,
		MediaDelivery: inst.S3MediaDelivery,
	}
}

// SetS3Config validates, encrypts (the secret) and stores an instance's S3
// config, then invalidates the runtime media-storage cache so the change takes
// effect on the next media message.
func (i instances) SetS3Config(instanceId string, data *S3ConfigStruct) (*S3ConfigStatus, error) {
	if data == nil {
		return nil, errors.New("invalid request")
	}
	instance, err := i.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Cannot set S3 config: %v", instanceId, err)
		return nil, err
	}

	delivery, err := normalizeMediaDelivery(data.MediaDelivery)
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimSpace(data.Endpoint)
	bucket := strings.TrimSpace(data.Bucket)
	accessKey := strings.TrimSpace(data.AccessKey)

	if data.Enabled {
		if endpoint == "" || bucket == "" || accessKey == "" {
			return nil, errors.New("endpoint, bucket and accessKey are required when S3 is enabled")
		}
		if strings.TrimSpace(data.SecretKey) == "" && instance.S3SecretKey == "" {
			return nil, errors.New("secretKey is required when S3 is enabled")
		}
	}

	updates := map[string]interface{}{
		"s3_enabled":        data.Enabled,
		"s3_endpoint":       endpoint,
		"s3_region":         strings.TrimSpace(data.Region),
		"s3_bucket":         bucket,
		"s3_access_key":     accessKey,
		"s3_path_style":     data.PathStyle,
		"s3_public_url":     strings.TrimSpace(data.PublicURL),
		"s3_media_delivery": delivery,
	}

	// An empty secret on update keeps the stored one (so operators can change
	// other fields without re-sending it).
	if secret := strings.TrimSpace(data.SecretKey); secret != "" {
		encrypted, err := webhooksign.Encrypt(i.config.DataEncryptionKey, secret)
		if err != nil {
			i.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to encrypt S3 secret: %v", instanceId, err)
			return nil, err
		}
		updates["s3_secret_key"] = encrypted
	}

	if err := i.instanceRepository.UpdateS3Config(instanceId, updates); err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to store S3 config: %v", instanceId, err)
		return nil, err
	}

	i.invalidateAuthCache()
	i.whatsmeowService.InvalidateMediaStorage(instanceId)
	i.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] S3 config updated (enabled=%v, delivery=%s)", instanceId, data.Enabled, delivery)
	return i.GetS3Config(instanceId)
}

// DeleteS3Config removes the per-instance S3 config, reverting the instance to
// the global MinIO config (or base64).
func (i instances) DeleteS3Config(instanceId string) error {
	if _, err := i.instanceRepository.GetInstanceByID(instanceId); err != nil {
		return err
	}
	updates := map[string]interface{}{
		"s3_enabled":        false,
		"s3_endpoint":       "",
		"s3_region":         "",
		"s3_bucket":         "",
		"s3_access_key":     "",
		"s3_secret_key":     "",
		"s3_path_style":     false,
		"s3_public_url":     "",
		"s3_media_delivery": "",
	}
	if err := i.instanceRepository.UpdateS3Config(instanceId, updates); err != nil {
		i.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to clear S3 config: %v", instanceId, err)
		return err
	}
	i.invalidateAuthCache()
	i.whatsmeowService.InvalidateMediaStorage(instanceId)
	i.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] S3 config cleared", instanceId)
	return nil
}

// GetS3Config returns the stored config with the secret masked.
func (i instances) GetS3Config(instanceId string) (*S3ConfigStatus, error) {
	instance, err := i.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		return nil, err
	}
	return s3StatusFromInstance(instance), nil
}

// TestS3Connection probes the bucket (no writes). Fields omitted from the
// request fall back to the stored config, so it can test an existing config
// without re-sending the secret.
func (i instances) TestS3Connection(instanceId string, data *S3ConfigStruct) (*S3TestResult, error) {
	instance, err := i.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		return nil, err
	}
	if data == nil {
		data = &S3ConfigStruct{}
	}

	pick := func(request, stored string) string {
		if strings.TrimSpace(request) != "" {
			return strings.TrimSpace(request)
		}
		return strings.TrimSpace(stored)
	}
	endpoint := pick(data.Endpoint, instance.S3Endpoint)
	bucket := pick(data.Bucket, instance.S3Bucket)
	accessKey := pick(data.AccessKey, instance.S3AccessKey)
	region := pick(data.Region, instance.S3Region)

	secret := strings.TrimSpace(data.SecretKey)
	if secret == "" && instance.S3SecretKey != "" {
		secret, err = webhooksign.Decrypt(i.config.DataEncryptionKey, instance.S3SecretKey)
		if err != nil {
			return nil, fmt.Errorf("could not decrypt the stored secret; re-send it in the request")
		}
	}
	if endpoint == "" || bucket == "" || accessKey == "" || secret == "" {
		return nil, errors.New("endpoint, bucket, accessKey and secretKey are required")
	}

	host, useSSL := minio_storage.NormalizeEndpoint(endpoint)
	if err := minio_storage.TestConnection(minio_storage.Options{
		Endpoint:  host,
		AccessKey: accessKey,
		SecretKey: secret,
		Bucket:    bucket,
		Region:    region,
		UseSSL:    useSSL,
		PathStyle: data.PathStyle,
	}); err != nil {
		return nil, err
	}
	return &S3TestResult{Bucket: bucket, Region: region}, nil
}

func NewInstanceService(
	instanceRepository instance_repository.InstanceRepository,
	killChannel *safemap.Map[chan bool],
	clientPointer *safemap.Map[*whatsmeow.Client],
	whatsmeowService whatsmeow_service.WhatsmeowService,
	config *config.Config,
	loggerWrapper *logger_wrapper.LoggerManager,
) InstanceService {
	// Encrypt instance tokens at rest. The key follows the same precedence as the
	// other secrets; if none is available the codec is nil and authentication
	// falls back to the legacy plaintext column (so an unconfigured install keeps
	// working, just without encryption).
	var codec *tokencrypt.Codec
	if len(config.DataEncryptionKey) > 0 {
		if c, err := tokencrypt.New(string(config.DataEncryptionKey)); err == nil {
			codec = c
		} else {
			loggerWrapper.GetLogger("config").LogError("[CONFIG] could not build the token codec: %v", err)
		}
	} else {
		loggerWrapper.GetLogger("config").LogWarn("[CONFIG] no encryption key configured; instance tokens will not be encrypted at rest")
	}

	return &instances{
		instanceRepository: instanceRepository,
		killChannel:        killChannel,
		clientPointer:      clientPointer,
		whatsmeowService:   whatsmeowService,
		config:             config,
		loggerWrapper:      loggerWrapper,
		authCache:          cache.New(authCacheTTL, 2*authCacheTTL),
		tokenCodec:         codec,
	}
}
