package whatsmeow_service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/evolution-foundation/evolution-go/pkg/safemap"
	"image/png"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/webp"
	"google.golang.org/protobuf/proto"

	_ "github.com/lib/pq"
	"github.com/patrickmn/go-cache"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waMmsRetry"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	producer_interfaces "github.com/evolution-foundation/evolution-go/pkg/events/interfaces"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
	"github.com/evolution-foundation/evolution-go/pkg/internal/event_types"
	label_model "github.com/evolution-foundation/evolution-go/pkg/label/model"
	label_repository "github.com/evolution-foundation/evolution-go/pkg/label/repository"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	message_repository "github.com/evolution-foundation/evolution-go/pkg/message/repository"
	"github.com/evolution-foundation/evolution-go/pkg/passkey/ceremony"
	poll_service "github.com/evolution-foundation/evolution-go/pkg/poll/service"
	storage_interfaces "github.com/evolution-foundation/evolution-go/pkg/storage/interfaces"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"github.com/evolution-foundation/evolution-go/pkg/walimits"
)

type WhatsmeowService interface {
	StartClient(clientData *ClientData)
	ConnectOnStartup(clientName string)
	StartInstance(instanceId string) error

	// Typebot integration. The processor is injected (not imported) because
	// typebot_service depends on sendMessage_service, which depends on this
	// package — importing it directly would close a cycle.
	SetTypebotService(processor TypebotProcessor)
	DispatchToTypebot(instance *instance_model.Instance, remoteJid, pushName, content string, fromMe bool)
	// SendOperationalEvent publishes an operational event (today only the
	// Typebot auto-pause) WITHOUT going through CallWebhook's subscription
	// filter: a protection alert must not depend on the instance having
	// subscribed to the right event.
	SendOperationalEvent(instance *instance_model.Instance, event string, data map[string]any)

	ReconnectClient(instanceId string) error
	ClearInstanceCache(instanceId string, token string) error
	// DeleteInstanceDevice removes the instance's device (and, by cascade, its
	// sessions/keys/contacts) from the whatsmeow store.
	DeleteInstanceDevice(instanceId, jid string) error

	// Media retry: ask the sender's phone to re-upload media whose download
	// failed (403/404/410), and serve the refreshed bytes on a later request.
	RequestMediaRetry(instanceId string, info *types.MessageInfo, mediaKey []byte, media whatsmeow.DownloadableMessage) error
	GetRetriedMedia(instanceId, messageID string) ([]byte, bool)
	HandleMediaRetry(instanceId string, evt *events.MediaRetry)
	CallWebhook(instance *instance_model.Instance, queueName string, jsonData []byte)
	SendToGlobalQueues(event string, jsonData []byte, userId string)
	ForceUpdateJid(instanceId string, number string) error
	UpdateInstanceSettings(instanceId string) error
	UpdateInstanceAdvancedSettings(instanceId string) error
	GetPollService() poll_service.PollService // NOVO: Acesso ao serviço de polls

	// Passkey (WebAuthn) pairing bridge — read by the public ceremony endpoint,
	// written by the whatsmeow event goroutine.
	PasskeyCeremonyStore() *ceremony.Store
	SubmitPasskeyResponse(instanceId string, resp *types.WebAuthnResponse) error
	ConfirmPasskey(instanceId string) error

	// GetInstanceOverview returns the connected instance's own profile picture,
	// push name and local contact count, for the dashboard.
	GetInstanceOverview(instanceId string) (*InstanceOverview, error)

	// ResolveChats turns bare message sources into display names (and the phone
	// behind them) for the dashboard's "most active conversations" list.
	ResolveChats(users []string) map[string]ChatIdentity
}

// InstanceOverview is the per-instance summary the self-hosted dashboard shows
// for each instance. It is deliberately cheap: the contact count comes from the
// local whatsmeow store and the picture is a preview fetch, both best-effort.
type InstanceOverview struct {
	Connected     bool   `json:"connected"`
	Platform      string `json:"platform,omitempty"`
	BusinessName  string `json:"businessName,omitempty"`
	ProfileName   string `json:"profileName,omitempty"`
	ProfilePicURL string `json:"profilePicUrl,omitempty"`
	ContactsCount int    `json:"contactsCount"`
	ChatsCount    int64  `json:"chatsCount"`
	MessagesCount int64  `json:"messagesCount"`
}

// TypebotProcessor is the slice of the Typebot service this package consumes.
// Declaring the interface here, at the consumer, avoids the import cycle that
// referencing typebot_service directly would create.
type TypebotProcessor interface {
	ProcessMessage(instance *instance_model.Instance, remoteJid, pushName, content string, fromMe bool)
}

type clientVersion struct {
	Major int
	Minor int
	Patch int
}

type whatsmeowService struct {
	instanceRepository instance_repository.InstanceRepository
	authDB             *sql.DB
	typebotProcessor   TypebotProcessor
	messageRepository  message_repository.MessageRepository
	labelRepository    label_repository.LabelRepository
	pollService        poll_service.PollService // NOVO: Serviço de enquetes
	config             *config.Config
	killChannel        *safemap.Map[chan bool]
	userInfoCache      *cache.Cache
	chatNameCache      *cache.Cache
	groupInfoCache     *cache.Cache
	clientPointer      *safemap.Map[*whatsmeow.Client]
	myClientPointer    *safemap.Map[*MyClient]
	rabbitmqProducer   producer_interfaces.Producer
	webhookProducer    producer_interfaces.Producer
	websocketProducer  producer_interfaces.Producer
	sqliteDB           *sql.DB
	exPath             string
	mediaStorage       storage_interfaces.MediaStorage
	processedMessages  *cache.Cache
	natsProducer       producer_interfaces.Producer
	loggerWrapper      *logger_wrapper.LoggerManager
	passkeyCeremony    *ceremony.Store
	persistPool        *persistPool
	// Media-retry state: pending requests (to decrypt the response) and the
	// refreshed bytes to serve on the next download request.
	mediaRetryPending *cache.Cache
	mediaRetryBytes   *cache.Cache
	// authStore is heap-allocated so sync.Once works with value-receiver methods like StartClient.
	authStore *sharedSQLStore
}

// sharedSQLStore holds the process-wide whatsmeow sqlstore.Container for PostgresAuthDB
// (or the sqlite fallback). One Upgrade per process; never Close on instance disconnect.
//
// A mutex rather than sync.Once: a transient database failure must not be cached
// permanently, otherwise every instance stays unable to start until the process
// restarts. A successful container is memoized and reused.
type sharedSQLStore struct {
	mu        sync.Mutex
	container *sqlstore.Container
	err       error
	sqliteDB  *sql.DB // kept alive when not using PostgresAuthDB
}
type MyClient struct {
	service            WhatsmeowService
	WAClient           *whatsmeow.Client
	eventHandlerID     uint32
	userID             string
	Instance           *instance_model.Instance
	token              string
	subscriptions      []string
	webhookUrl         string
	rabbitmqEnable     string
	natsEnable         string
	websocketEnable    string
	instanceRepository instance_repository.InstanceRepository
	messageRepository  message_repository.MessageRepository
	labelRepository    label_repository.LabelRepository
	pollService        poll_service.PollService // NOVO: Serviço de enquetes
	clientPointer      *safemap.Map[*whatsmeow.Client]
	myClientPointer    *safemap.Map[*MyClient]
	killChannel        *safemap.Map[chan bool]
	userInfoCache      *cache.Cache
	config             *config.Config
	groupInfoCache     *cache.Cache
	historySyncID      int32
	rabbitmqProducer   producer_interfaces.Producer
	webhookProducer    producer_interfaces.Producer
	websocketProducer  producer_interfaces.Producer
	mediaStorage       storage_interfaces.MediaStorage
	processedMessages  *cache.Cache
	natsProducer       producer_interfaces.Producer
	loggerWrapper      *logger_wrapper.LoggerManager
	qrcodeCount        int
	passkeyCeremony    *ceremony.Store
	persistPool        *persistPool
	appStateRecoveryMu sync.Mutex
	appStateRecovery   map[appstate.WAPatchName]appStateRecoveryAttempt
	nctSaltSyncMu      sync.Mutex
	nctSaltSyncAt      time.Time
}

type appStateRecoveryAttempt struct {
	fullSyncAt        time.Time
	recoveryRequestAt time.Time
}

const appStateRecoveryCooldown = 15 * time.Minute

func (mycli *MyClient) reserveAppStateRecovery(name appstate.WAPatchName, recoveryRequest bool) bool {
	mycli.appStateRecoveryMu.Lock()
	defer mycli.appStateRecoveryMu.Unlock()

	if mycli.appStateRecovery == nil {
		mycli.appStateRecovery = make(map[appstate.WAPatchName]appStateRecoveryAttempt)
	}

	now := time.Now()
	attempt := mycli.appStateRecovery[name]
	lastAttempt := attempt.fullSyncAt
	if recoveryRequest {
		lastAttempt = attempt.recoveryRequestAt
	}
	if !lastAttempt.IsZero() && now.Sub(lastAttempt) < appStateRecoveryCooldown {
		return false
	}

	if recoveryRequest {
		attempt.recoveryRequestAt = now
	} else {
		attempt.fullSyncAt = now
	}
	mycli.appStateRecovery[name] = attempt
	return true
}

// nctSaltSyncCooldown bounds how often the forced regular_high bootstrap below
// can run for an instance that still has no NCT salt. Long enough not to hammer
// the server for accounts the server genuinely provisions no salt for, short
// enough that a transient failure self-heals without a process restart.
const nctSaltSyncCooldown = 6 * time.Hour

// reserveNctSaltSync reports whether the forced NCT-salt bootstrap may run now.
// Mirrors reserveAppStateRecovery: the timestamp is written before the attempt so
// concurrent/repeated Connected events cannot stampede, and it is not cleared on
// failure so a permanently-unsalted account stays bounded to one try per cooldown.
func (mycli *MyClient) reserveNctSaltSync() bool {
	mycli.nctSaltSyncMu.Lock()
	defer mycli.nctSaltSyncMu.Unlock()

	now := time.Now()
	if !mycli.nctSaltSyncAt.IsZero() && now.Sub(mycli.nctSaltSyncAt) < nctSaltSyncCooldown {
		return false
	}
	mycli.nctSaltSyncAt = now
	return true
}

// ensureNctSaltSynced backfills the NCT salt for instances that never received it.
//
// The salt is normally learned from the one-time HistorySync payload sent right
// after pairing, and thereafter from a regular_high `nct_salt_sync` app-state
// mutation. Instances paired before this fork switched to upstream whatsmeow
// (v0.7.2) went through their one-time sync with a fork that had no concept of
// the salt, so they must learn it from app state — but whatsmeow's
// handleAppStateSyncKeyShare calls FetchAppState with onlyIfNotSynced=true, which
// skips categories already marked as synced. Those instances therefore never get
// a salt and every cold (first-contact) 1:1 send fails with error 463
// (NackCallerReachoutTimelocked), forever and silently. See issue #124.
//
// Forcing fullSync=true, onlyIfNotSynced=false on regular_high re-reads the
// category from scratch so the salt mutation is applied. It is a no-op when a
// salt is already stored, and rate-limited per instance (see nctSaltSyncCooldown).
func (mycli *MyClient) ensureNctSaltSynced() {
	if mycli == nil || mycli.WAClient == nil || mycli.WAClient.Store == nil || mycli.WAClient.Store.NCTSalt == nil {
		return
	}

	// Quick check first: a normal pairing/history sync may already have supplied
	// the salt, in which case there is nothing to bootstrap.
	checkCtx, cancelCheck := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelCheck()
	if salt, err := mycli.WAClient.Store.NCTSalt.GetNCTSalt(checkCtx); err == nil && len(salt) > 0 {
		return
	}

	if !mycli.reserveNctSaltSync() {
		return
	}

	logger := mycli.loggerWrapper.GetLogger(mycli.userID)
	logger.LogInfo("[%s] No NCT salt stored; forcing a regular_high app-state sync to backfill it (issue #124)", mycli.userID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := mycli.WAClient.FetchAppState(ctx, appstate.WAPatchRegularHigh, true, false); err != nil {
		logger.LogWarn("[%s] Forced regular_high sync for NCT salt failed: %v", mycli.userID, err)
		return
	}

	if salt, err := mycli.WAClient.Store.NCTSalt.GetNCTSalt(ctx); err == nil && len(salt) > 0 {
		logger.LogInfo("[%s] NCT salt backfilled from regular_high", mycli.userID)
	} else {
		logger.LogInfo("[%s] regular_high sync completed; no NCT salt is provisioned for this account", mycli.userID)
	}
}

func (mycli *MyClient) handleAppStateSyncError(evt *events.AppStateSyncError) {
	if evt == nil || mycli.WAClient == nil {
		return
	}

	// A failed incremental sync is retried once as a full sync. If the full
	// snapshot also fails verification, ask the primary phone for a recovery
	// snapshot. This follows the recovery sequence documented by whatsmeow and
	// deliberately avoids the fatal recovery notification, which unlinks every
	// companion device and would require a new login/QR.
	recoveryRequest := evt.FullSync
	if !mycli.reserveAppStateRecovery(evt.Name, recoveryRequest) {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo(
			"[%s] App-state recovery already attempted recently for %s (fullSync=%t)",
			mycli.userID, evt.Name, evt.FullSync,
		)
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if !recoveryRequest {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn(
				"[%s] App-state incremental sync failed for %s; starting controlled full sync",
				mycli.userID, evt.Name,
			)
			if err := mycli.WAClient.FetchAppState(ctx, evt.Name, true, false); err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn(
					"[%s] App-state full sync did not complete for %s; recovery request will be used if the full-sync error event is emitted: %v",
					mycli.userID, evt.Name, err,
				)
			}
			return
		}

		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn(
			"[%s] App-state full sync failed for %s; requesting recovery snapshot from primary device",
			mycli.userID, evt.Name,
		)
		if _, err := mycli.WAClient.SendPeerMessage(ctx, whatsmeow.BuildAppStateRecoveryRequest(evt.Name)); err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError(
				"[%s] Failed to send app-state recovery request for %s: %v",
				mycli.userID, evt.Name, err,
			)
			return
		}
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo(
			"[%s] App-state recovery request sent for %s",
			mycli.userID, evt.Name,
		)
	}()
}

func (mycli *MyClient) persistMessageAsync(message message_model.Message) {
	if mycli == nil || mycli.messageRepository == nil {
		return
	}

	job := persistJob{
		repo:       mycli.messageRepository,
		logger:     mycli.loggerWrapper,
		instanceID: mycli.userID,
		message:    message,
	}
	if mycli.persistPool != nil && mycli.persistPool.submit(job) {
		return
	}

	// No pool (a bare MyClient in a test) or the pool is saturated: persist
	// inline so the message is never dropped.
	if err := mycli.messageRepository.InsertMessage(message); err != nil && mycli.loggerWrapper != nil {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to persist message %s: %v", mycli.userID, message.MessageID, err)
	}
}

// Message persistence used to spawn one goroutine (and one DB upsert) per
// message with no bound, so a history sync or a message burst could create
// thousands of goroutines and put unbounded pressure on the connection pool.
// A fixed-size worker pool with a bounded queue smooths that out; when the queue
// is full the caller persists inline, which applies backpressure instead of
// letting memory and goroutines grow without limit.
const (
	persistWorkers   = 8
	persistQueueSize = 4096
)

type persistJob struct {
	repo       message_repository.MessageRepository
	logger     *logger_wrapper.LoggerManager
	instanceID string
	message    message_model.Message
}

type persistPool struct {
	queue chan persistJob
}

// newPersistPool starts workers goroutines draining a queue of size size.
func newPersistPool(workers, size int) *persistPool {
	if workers < 1 {
		workers = 1
	}
	if size < 1 {
		size = 1
	}
	p := &persistPool{queue: make(chan persistJob, size)}
	for i := 0; i < workers; i++ {
		go func() {
			for job := range p.queue {
				if err := job.repo.InsertMessage(job.message); err != nil && job.logger != nil {
					job.logger.GetLogger(job.instanceID).LogError("[%s] Failed to persist message %s: %v", job.instanceID, job.message.MessageID, err)
				}
			}
		}()
	}
	return p
}

// submit enqueues a job without blocking, reporting false when the queue is full.
func (p *persistPool) submit(job persistJob) bool {
	select {
	case p.queue <- job:
		return true
	default:
		return false
	}
}

// getGroupInfoCached resolves group metadata, reusing a recent result. Every
// group message used to trigger a live IQ inside the event handler; group
// metadata changes rarely, so a short cache removes that call from the hot path.
func (mycli *MyClient) getGroupInfoCached(jid types.JID) (*types.GroupInfo, error) {
	key := mycli.userID + "|" + jid.String()
	if mycli.groupInfoCache != nil {
		if v, ok := mycli.groupInfoCache.Get(key); ok {
			if info, ok := v.(*types.GroupInfo); ok {
				return info, nil
			}
		}
	}

	info, err := mycli.WAClient.GetGroupInfo(context.Background(), jid)
	if err != nil {
		return nil, err
	}
	if mycli.groupInfoCache != nil {
		mycli.groupInfoCache.Set(key, info, cache.DefaultExpiration)
	}
	return info, nil
}

// stripMessageThumbnails clears the media thumbnails on an incoming message.
// The webhook payload deletes them anyway, so clearing them before the event is
// converted to a map avoids base64-encoding (and then decoding) a potentially
// large thumbnail only to discard it. Media download uses the media keys, not the
// thumbnail, so stored media is unaffected. Only the fields the payload already
// drops are cleared, so the emitted JSON is unchanged.
func stripMessageThumbnails(m *waE2E.Message) {
	if m == nil {
		return
	}
	if img := m.GetImageMessage(); img != nil {
		img.JPEGThumbnail = nil
	}
	if vid := m.GetVideoMessage(); vid != nil {
		vid.JPEGThumbnail = nil
	}
	if doc := m.GetDocumentMessage(); doc != nil {
		doc.JPEGThumbnail = nil
	}
}

type ClientData struct {
	Instance      *instance_model.Instance
	Subscriptions []string
	Phone         string
	IsProxy       bool
}

type Values struct {
	m map[string]string
}

func (v Values) Get(key string) string {
	return v.m[key]
}

type UserCollection struct {
	Users map[types.JID]types.UserInfo
}

type ProxyConfig struct {
	Protocol string `json:"protocol,omitempty"`
	Host     string `json:"host"`
	Password string `json:"password"`
	Port     string `json:"port"`
	Username string `json:"username"`
}

func (w whatsmeowService) ReconnectClient(instanceId string) error {
	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Starting reconnection process - simulating restart", instanceId)

	// Passo 1: Limpar conexão existente se houver
	if client, exists := w.clientPointer.Lookup(instanceId); exists {
		w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Disconnecting existing client", instanceId)

		// Desconectar o cliente WebSocket
		if client.IsConnected() {
			client.Disconnect()
			w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] WebSocket disconnected", instanceId)
		}

		// Remover event handler se existir
		if mycli, ok := w.myClientPointer.Lookup(instanceId); ok {
			if mycli.eventHandlerID != 0 {
				client.RemoveEventHandler(mycli.eventHandlerID)
				w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Event handler removed", instanceId)
			}
		}
	}

	// Passo 2: Limpar todos os recursos da instância
	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Cleaning up resources", instanceId)

	// Enviar sinal de kill se o canal existir
	if killChan, exists := w.killChannel.Lookup(instanceId); exists {
		select {
		case killChan <- true:
			w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Kill signal sent", instanceId)
		default:
			// Canal pode estar bloqueado, continua
		}
	}

	// Remover das estruturas
	w.clientPointer.Delete(instanceId)
	w.myClientPointer.Delete(instanceId)
	w.killChannel.Delete(instanceId)

	// Limpar cache de userInfo para esta instância
	if instance, err := w.instanceRepository.GetInstanceByID(instanceId); err == nil {
		w.userInfoCache.Delete(instance.Token)
		w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] UserInfo cache cleared for token: %s", instanceId, instance.Token)
	}

	// Passo 3: Atualizar status no banco
	instance, err := w.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		return fmt.Errorf("failed to get instance: %v", err)
	}

	instance.Connected = false
	instance.DisconnectReason = "Reconnecting"
	err = w.instanceRepository.UpdateConnected(instanceId, false, "Reconnecting")
	if err != nil {
		w.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Failed to update disconnect status: %v", instanceId, err)
	}

	// Passo 4: Aguardar um pouco para garantir limpeza completa
	time.Sleep(2 * time.Second)

	// Passo 5: Iniciar nova instância como se fosse a primeira vez
	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Starting fresh instance", instanceId)
	return w.StartInstance(instanceId)
}

func (w whatsmeowService) ForceUpdateJid(instanceId string, number string) error {
	instance, err := w.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error getting instance: %v", instanceId, err)
		return err
	}

	if instance.Jid == "" && number != "" {
		// Parameterised: `number` comes from the API body, so it must never be
		// interpolated into SQL (it previously was, which allowed injection).
		const sqlDeviceSearch = `SELECT jid FROM whatsmeow_device WHERE jid LIKE '%' || $1 || '%'`
		rows, err := w.authDB.Query(sqlDeviceSearch, number)
		if err != nil {
			w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error getting device: %v", instanceId, err)
			return err
		}

		defer rows.Close()

		var latestJid string
		var latestSession int

		for rows.Next() {
			type deviceStruct struct {
				Jid string `json:"jid"`
			}
			var device deviceStruct
			err := rows.Scan(&device.Jid)
			if err != nil {
				w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error getting device: %v", instanceId, err)
				return err
			}

			// Extrair o número da sessão do JID
			parts := strings.Split(device.Jid, ":")
			if len(parts) == 2 {
				sessionPart := strings.Split(parts[1], "@")[0]
				session, err := strconv.Atoi(sessionPart)
				if err != nil {
					w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error parsing session number: %v", instanceId, err)
					return err
				}

				// Atualizar se for a sessão mais recente
				if session > latestSession {
					latestSession = session
					latestJid = device.Jid
				}
			}
		}

		// Atualizar a instância com o JID mais recente
		if latestJid != "" {
			instance.Jid = latestJid
			err = w.instanceRepository.UpdateJid(instanceId, latestJid)
			if err != nil {
				w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error updating instance: %v", instanceId, err)
			}
			w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Updated instance with latest JID: %s (session: %d)", instanceId, latestJid, latestSession)
		}
	}

	return nil
}

// getSharedSQLStoreContainer returns the process-wide whatsmeow sqlstore.Container.
// PostgresAuthDB reuses the pooled authDB from initPostgresAuthDB (Upgrade once).
// The Users/GORM database is intentionally separate and never passed here.
func (w whatsmeowService) getSharedSQLStoreContainer() (*sqlstore.Container, error) {
	h := w.authStore
	if h == nil {
		return nil, fmt.Errorf("shared sqlstore not initialized")
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.container != nil {
		return h.container, nil
	}
	h.err = nil

	var dbLog waLog.Logger
	if w.config.WaDebug != "" {
		dbLog = waLog.Stdout("Database", w.config.WaDebug, true)
	}
	ctx := context.Background()
	if w.config.PostgresAuthDB != "" {
		if w.authDB == nil {
			h.err = fmt.Errorf("postgres auth DB handle is nil")
			return nil, h.err
		}
		container := sqlstore.NewWithDB(w.authDB, "postgres", dbLog)
		if err := container.Upgrade(ctx); err != nil {
			// Do not Close authDB — owned by main.
			h.err = fmt.Errorf("failed to upgrade database: %w", err)
			return nil, h.err
		}
		h.container = container
		return h.container, nil
	}

	dsn := fmt.Sprintf("file:%s/dbdata/main.db?_pragma=foreign_keys(1)&_busy_timeout=5000&cache=shared&mode=rwc&_journal_mode=WAL", w.exPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		h.err = fmt.Errorf("failed to open sqlite auth store: %w", err)
		return nil, h.err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)
	container := sqlstore.NewWithDB(db, "sqlite", dbLog)
	if err := container.Upgrade(ctx); err != nil {
		_ = db.Close()
		h.err = fmt.Errorf("failed to upgrade database: %w", err)
		return nil, h.err
	}
	h.sqliteDB = db
	h.container = container
	return h.container, nil
}

// ============================================================================
// Backoff for the reconnect loop.
//
// THE LOOP: an instance whose device was logged out from the phone spins
// forever. Disconnected -> ReconnectClient -> the instance comes up with no
// session -> emits a QR -> nobody scans -> max QR count -> forced logout ->
// Disconnected again. Measured in production: 110 reconnects and 1135 QR codes
// in 50 minutes from a single instance, and it only stopped when a human looked.
//
// Nothing in the current code counts those restarts, because from
// ReconnectClient's point of view every turn SUCCEEDS — the instance really does
// come up. What fails afterwards is the pairing, which nobody was measuring.
//
// THE SHAPE: the first reconnectFreeAttempts restarts inside the window go
// straight through — that is the good case, a healthy instance that lost its
// websocket and must come back within seconds. Past that the loop turns into a
// growing wait, and the instance keeps trying: this is a backoff, not a
// give-up. An instance with a valid session still heals on its own; what is
// lost is the hammering.
//
// NOTE: only one goroutine waits per instance (the scheduled flag). Without it
// every Disconnected arriving during the wait would stack another one, and the
// backoff would become the very loop it was written to stop.
// ============================================================================

const (
	reconnectWindow       = 15 * time.Minute // restarts inside this window count together
	reconnectFreeAttempts = 5                // how many go through with no wait
)

// The ladder of waits. The last step repeats indefinitely.
var reconnectBackoff = []time.Duration{5 * time.Minute, 15 * time.Minute, 30 * time.Minute}

type reconnectState struct {
	restarts  int
	since     time.Time
	step      int
	scheduled bool
}

var (
	reconnectMu    sync.Mutex
	reconnectTrack = map[string]*reconnectState{}
)

// reconnectAllowed reports whether this instance may restart right now.
//
//	(true, 0)   — go ahead, normal path
//	(false, d)  — wait d and then try; this goroutine owns the wait
//	(false, -1) — another goroutine is already waiting for this instance; give up
func reconnectAllowed(instanceID string) (bool, time.Duration) {
	reconnectMu.Lock()
	defer reconnectMu.Unlock()

	now := time.Now()
	st := reconnectTrack[instanceID]
	if st == nil || now.Sub(st.since) > reconnectWindow {
		reconnectTrack[instanceID] = &reconnectState{restarts: 1, since: now}
		return true, 0
	}
	if st.scheduled {
		return false, -1
	}
	st.restarts++
	if st.restarts <= reconnectFreeAttempts {
		return true, 0
	}

	d := reconnectBackoff[len(reconnectBackoff)-1]
	if st.step < len(reconnectBackoff) {
		d = reconnectBackoff[st.step]
		st.step++
	}
	st.scheduled = true
	return false, d
}

// reconnectWaitDone puts the instance back in line once its wait is over.
func reconnectWaitDone(instanceID string) {
	reconnectMu.Lock()
	if st := reconnectTrack[instanceID]; st != nil {
		st.scheduled = false
	}
	reconnectMu.Unlock()
}

// reconnectSucceeded clears the state — called when the instance actually
// connects. Without it the backoff would inherit the count of a problem that is
// already solved.
func reconnectSucceeded(instanceID string) {
	reconnectMu.Lock()
	delete(reconnectTrack, instanceID)
	reconnectMu.Unlock()
}

// ClearReconnectBackoff forgets an instance's reconnect history so the next
// reconnect gets a fresh, unthrottled budget. Used when a human explicitly asks
// for a reconnect (for example after changing the proxy), which is exactly the
// signal that the automatic counter no longer describes the situation.
func ClearReconnectBackoff(instanceID string) {
	reconnectSucceeded(instanceID)
}

// AccountLimitsCacheEntry holds the last successfully fetched WhatsApp account
// limits for an instance. The MEX queries can be slow/rate-limited, so they are
// fetched once on connect and cached here for GET /instance/limits to serve
// instantly.
type AccountLimitsCacheEntry struct {
	ReachoutActive bool
	ReachoutEnds   int64 // unix seconds
	ReachoutType   string
	CappingStatus  string
	TotalQuota     int
	UsedQuota      int
	CycleEnds      int64 // unix seconds
	FetchedAt      time.Time
}

var accountLimitsCache sync.Map // instanceID(string) -> *AccountLimitsCacheEntry

// chatEphemeralCache remembers each chat's disappearing-messages timer (seconds)
// so outgoing messages can carry ContextInfo.Expiration. WhatsApp warns the
// recipient ("This message will not disappear") on any outgoing message that
// does not include the chat's timer. The value is learned from the
// EPHEMERAL_SETTING protocol notification (sent when someone changes the timer)
// and from received ephemeral messages. Key: "instanceID|chatJID".
var chatEphemeralCache sync.Map // string -> uint32

// SetCachedChatEphemeral records the disappearing timer for a chat.
func SetCachedChatEphemeral(instanceID string, chat types.JID, seconds uint32) {
	chatEphemeralCache.Store(instanceID+"|"+chat.ToNonAD().String(), seconds)
}

// GetCachedChatEphemeral returns the last known disappearing timer for a chat.
// known is false when the chat's timer has never been seen.
func GetCachedChatEphemeral(instanceID string, chat types.JID) (seconds uint32, known bool) {
	if v, ok := chatEphemeralCache.Load(instanceID + "|" + chat.ToNonAD().String()); ok {
		return v.(uint32), true
	}
	return 0, false
}

// messageEphemeralExpiration returns the disappearing-messages timer carried by
// a received message, or 0 when none is present. The timer lives on the
// ContextInfo of whichever content type the message uses; an EphemeralMessage
// wrapper carries it on the wrapper's ContextInfo.
func messageEphemeralExpiration(msg *waE2E.Message) uint32 {
	if msg == nil {
		return 0
	}
	if pm := msg.GetProtocolMessage(); pm != nil && pm.GetEphemeralExpiration() > 0 {
		return pm.GetEphemeralExpiration()
	}
	if wrapped := msg.GetEphemeralMessage().GetMessage(); wrapped != nil {
		if exp := messageEphemeralExpiration(wrapped); exp > 0 {
			return exp
		}
	}
	for _, ctx := range []*waE2E.ContextInfo{
		msg.GetExtendedTextMessage().GetContextInfo(),
		msg.GetImageMessage().GetContextInfo(),
		msg.GetVideoMessage().GetContextInfo(),
		msg.GetPtvMessage().GetContextInfo(),
		msg.GetAudioMessage().GetContextInfo(),
		msg.GetDocumentMessage().GetContextInfo(),
		msg.GetStickerMessage().GetContextInfo(),
		msg.GetLocationMessage().GetContextInfo(),
		msg.GetContactMessage().GetContextInfo(),
		msg.GetPollCreationMessage().GetContextInfo(),
	} {
		if ctx != nil && ctx.GetExpiration() > 0 {
			return ctx.GetExpiration()
		}
	}
	return 0
}

// GetCachedAccountLimits returns the last fetched account limits for an instance, if any.
func GetCachedAccountLimits(instanceID string) (*AccountLimitsCacheEntry, bool) {
	v, ok := accountLimitsCache.Load(instanceID)
	if !ok {
		return nil, false
	}
	return v.(*AccountLimitsCacheEntry), true
}

// logAccountLimits queries WhatsApp's MEX endpoints for the account's new-chat
// message capping and reachout timelock state, logs them, and caches the result.
// Error 463 on sends to NEW contacts is caused by these account-level limits, not
// by local code.
func (mycli *MyClient) logAccountLimits() {
	client := mycli.WAClient
	if client == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		entry := &AccountLimitsCacheEntry{FetchedAt: time.Now()}
		got := false

		if capInfo, err := walimits.GetNewChatMessageCappingInfo(ctx, client); err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to fetch new-chat message capping info: %v", mycli.userID, err)
		} else if capInfo != nil {
			entry.CappingStatus = string(capInfo.CappingStatus)
			entry.TotalQuota = capInfo.TotalQuota
			entry.UsedQuota = capInfo.UsedQuota
			entry.CycleEnds = capInfo.CycleEndTimestamp.Unix()
			got = true
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] NEW-CHAT CAPPING: status=%s used=%d/%d cycleEnds=%s",
				mycli.userID, capInfo.CappingStatus, capInfo.UsedQuota, capInfo.TotalQuota, capInfo.CycleEndTimestamp.Time)
		}

		if tl, err := walimits.GetAccountReachoutTimelock(ctx, client); err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to fetch reachout timelock: %v", mycli.userID, err)
		} else if tl != nil {
			entry.ReachoutActive = tl.IsActive
			if tl.IsActive {
				entry.ReachoutEnds = tl.TimeEnforcementEnds.Unix()
			}
			entry.ReachoutType = string(tl.EnforcementType)
			got = true
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] REACHOUT TIMELOCK: active=%t ends=%s type=%s",
				mycli.userID, tl.IsActive, tl.TimeEnforcementEnds.Time, tl.EnforcementType)
		}

		if got {
			accountLimitsCache.Store(mycli.userID, entry)
		}
	}()
}

// History-sync depth requested from the phone when a device links (DeviceProps.HistorySyncConfig).
// Generous defaults so a newly linked device receives the full available history; tune here if
// bandwidth/storage is a concern.
const (
	historyFullSyncDaysLimit   = 3650 // ~10 years
	historyFullSyncSizeMbLimit = 2048 // 2 GB
	historyStorageQuotaMb      = 2048 // 2 GB
)

// maxParallelRetryReceipts caps how many retry receipts whatsmeow handles at
// once. Its default is unlimited; a burst of undecryptable messages (each
// triggering a retry receipt) would otherwise spawn unbounded goroutines.
const maxParallelRetryReceipts = 10

func (w whatsmeowService) StartClient(cd *ClientData) {

	w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("Starting websocket connection to Whatsapp for user '%s'", cd.Instance.Id)

	var deviceStore *store.Device
	var err error

	if w.clientPointer.Get(cd.Instance.Id) != nil {
		if w.clientPointer.Get(cd.Instance.Id).IsConnected() {
			return
		}
	}

	container, err := w.getSharedSQLStoreContainer()
	if err != nil {
		w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Failed to create container: %v", cd.Instance.Id, err)
		return
	}

	if cd.Instance.Jid != "" {
		jid, _ := utils.ParseJID(cd.Instance.Jid)
		w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Jid found. Getting device store for jid: %s", cd.Instance.Id, jid)
		deviceStore, err = container.GetDevice(context.Background(), jid)
		if err != nil {
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Erro ao obter device store: %v", cd.Instance.Id, err)
			return
		}
	} else {
		w.loggerWrapper.GetLogger(cd.Instance.Id).LogWarn("[%s] No jid found. Creating new device", cd.Instance.Id)
		deviceStore = container.NewDevice()
	}

	if deviceStore == nil {
		w.loggerWrapper.GetLogger(cd.Instance.Id).LogWarn("[%s] No store found. Creating new one", cd.Instance.Id)
		deviceStore = container.NewDevice()

		cd.Instance.Connected = false
		err := w.instanceRepository.UpdateConnected(cd.Instance.Id, cd.Instance.Connected, cd.Instance.DisconnectReason)
		if err != nil {
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Error updating instance: %s", cd.Instance.Id, err)
		}
	}

	var version clientVersion

	platformID, ok := waCompanionReg.DeviceProps_PlatformType_value[strings.ToUpper("chrome")]
	if ok {
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_PlatformType(platformID).Enum()
	}
	if cd.Instance.OsName == "" {
		cd.Instance.OsName = utils.WhatsAppGetUserOS()
	}

	store.DeviceProps.Os = &cd.Instance.OsName
	store.DeviceProps.RequireFullSync = proto.Bool(true)
	// RequireFullSync alone still yields a shallow/uneven backfill because the phone falls back to
	// conservative defaults. Explicitly request a deep on-link HistorySync window so newly linked
	// devices receive the full available message history. Values are named constants so deployments
	// can tune history depth / resource usage in one place.
	store.DeviceProps.HistorySyncConfig = &waCompanionReg.DeviceProps_HistorySyncConfig{
		FullSyncDaysLimit:   proto.Uint32(historyFullSyncDaysLimit),
		FullSyncSizeMbLimit: proto.Uint32(historyFullSyncSizeMbLimit),
		StorageQuotaMb:      proto.Uint32(historyStorageQuotaMb),
	}

	if w.config.WhatsappVersionMajor != 0 && w.config.WhatsappVersionMinor != 0 && w.config.WhatsappVersionPatch != 0 {
		w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Setting whatsapp version to %d.%d.%d", cd.Instance.Id, w.config.WhatsappVersionMajor, w.config.WhatsappVersionMinor, w.config.WhatsappVersionPatch)
		version.Major = w.config.WhatsappVersionMajor
		if err == nil {
			store.DeviceProps.Version.Primary = proto.Uint32(uint32(version.Major))
		}
		version.Minor = w.config.WhatsappVersionMinor
		if err == nil {
			store.DeviceProps.Version.Secondary = proto.Uint32(uint32(version.Minor))
		}
		version.Patch = w.config.WhatsappVersionPatch
		if err == nil {
			store.DeviceProps.Version.Tertiary = proto.Uint32(uint32(version.Patch))
		}
	} else {
		// Try to fetch version from WhatsApp Web
		webVersion, err := fetchWhatsAppWebVersion()
		if err != nil {
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Failed to fetch WhatsApp Web version: %v", cd.Instance.Id, err)
		} else {
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Setting whatsapp version from web to %d.%d.%d", cd.Instance.Id, webVersion.Major, webVersion.Minor, webVersion.Patch)
			version = *webVersion
			store.DeviceProps.Version.Primary = proto.Uint32(uint32(version.Major))
			store.DeviceProps.Version.Secondary = proto.Uint32(uint32(version.Minor))
			store.DeviceProps.Version.Tertiary = proto.Uint32(uint32(version.Patch))
		}
	}

	// 🔒 FIX: Sempre criar logger, mesmo que WaDebug esteja vazio
	// Usar "INFO" como nível mínimo para garantir que logs importantes apareçam
	minLevel := w.config.WaDebug
	if minLevel == "" {
		minLevel = "INFO" // Nível mínimo para garantir que logs INFO apareçam
	}
	clientLog := waLog.Stdout("Client", minLevel, true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	// whatsmeow handles retry receipts with unlimited parallelism by default, so
	// a retry storm can spawn unbounded goroutines. Bound it; this must be set
	// before connecting.
	client.SetMaxParallelRetryReceiptHandling(maxParallelRetryReceipts)

	w.clientPointer.Set(cd.Instance.Id, client)

	if cd.IsProxy {
		var proxyConfig ProxyConfig
		err := json.Unmarshal([]byte(cd.Instance.Proxy), &proxyConfig)
		if err != nil {
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] error unmarshalling proxy config", cd.Instance.Id)
			return
		}

		proxyProtocol := proxyConfig.Protocol
		proxyHost := proxyConfig.Host
		proxyPort := proxyConfig.Port
		proxyUsername := proxyConfig.Username
		proxyPassword := proxyConfig.Password

		if proxyConfig.Host == "" {
			proxyHost = w.config.ProxyHost
		}

		if proxyConfig.Port == "" {
			proxyPort = w.config.ProxyPort
		}

		if proxyConfig.Protocol == "" {
			proxyProtocol = w.config.ProxyProtocol
		}

		if proxyConfig.Username == "" {
			proxyUsername = w.config.ProxyUsername
		}

		if proxyConfig.Password == "" {
			proxyPassword = w.config.ProxyPassword
		}

		proxyAddress, err := utils.BuildProxyAddress(proxyProtocol, proxyHost, proxyPort, proxyUsername, proxyPassword)
		if err != nil {
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogWarn("[%s] Proxy error, continuing without proxy: %v", cd.Instance.Id, err)
		} else {
			err = client.SetProxyAddress(proxyAddress)
			if err != nil {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogWarn("[%s] Proxy error, continuing without proxy: %v", cd.Instance.Id, err)
			} else {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Proxy enabled (%s)", cd.Instance.Id, utils.NormalizeProxyProtocol(proxyProtocol, proxyPort))
			}
		}
	}

	client.EnableAutoReconnect = false
	client.AutoTrustIdentity = true
	// Re-request messages that fail to decrypt from the phone instead of dropping them.
	client.AutomaticMessageRerequestFromPhone = w.config.RerequestFromPhone

	mycli := &MyClient{
		service:            &w,
		Instance:           cd.Instance,
		WAClient:           client,
		eventHandlerID:     1,
		userID:             cd.Instance.Id,
		token:              cd.Instance.Token,
		subscriptions:      cd.Subscriptions,
		webhookUrl:         cd.Instance.Webhook,
		rabbitmqEnable:     cd.Instance.RabbitmqEnable,
		natsEnable:         cd.Instance.NatsEnable,
		websocketEnable:    cd.Instance.WebSocketEnable,
		instanceRepository: w.instanceRepository,
		messageRepository:  w.messageRepository,
		labelRepository:    w.labelRepository,
		pollService:        w.pollService, // NOVO: Serviço de enquetes
		userInfoCache:      w.userInfoCache,
		groupInfoCache:     w.groupInfoCache,
		clientPointer:      w.clientPointer,
		myClientPointer:    w.myClientPointer,
		killChannel:        w.killChannel,
		config:             w.config,
		historySyncID:      0,
		rabbitmqProducer:   w.rabbitmqProducer,
		webhookProducer:    w.webhookProducer,
		websocketProducer:  w.websocketProducer,
		mediaStorage:       w.mediaStorage,
		processedMessages:  w.processedMessages,
		natsProducer:       w.natsProducer,
		loggerWrapper:      w.loggerWrapper,
		qrcodeCount:        0,
		passkeyCeremony:    w.passkeyCeremony,
		persistPool:        w.persistPool,
	}

	mycli.eventHandlerID = mycli.WAClient.AddEventHandler(mycli.myEventHandler)

	// Armazena o MyClient no map para permitir atualizações posteriores
	w.myClientPointer.Set(cd.Instance.Id, mycli)

	if client.Store.ID != nil {
		w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Already logged in with JID: %s", cd.Instance.Id, client.Store.ID.String())
		err = client.Connect()
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Erro de conexão WebSocket (EOF). Tentando reconectar em 5 segundos...", cd.Instance.Id)
				time.Sleep(5 * time.Second)
				err = client.Connect()
				if err != nil {
					w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Falha na segunda tentativa de conexão: %v", cd.Instance.Id, err)
					return
				}
			} else if strings.Contains(err.Error(), "username/password authentication failed") {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogWarn("[%s] Proxy authentication failed, attempting to connect without proxy", cd.Instance.Id)

				// Desabilita o proxy
				client.SetProxy(nil)

				// Tenta conectar sem proxy
				err = client.Connect()
				if err != nil {
					w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Failed to connect even without proxy: %v", cd.Instance.Id, err)
					return
				}
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Successfully connected without proxy", cd.Instance.Id)
			} else {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Failed to connect: %v", cd.Instance.Id, err)
				return
			}
		}
	} else {
		// New-device pairing. We intentionally do NOT use client.GetQRChannel:
		// in the installed whatsmeow its qrChannel handler auto-confirms a
		// passkey ceremony (SkipHandoffUX) and Disconnects the socket when the
		// QR codes run out, both of which break passkey pairing (DOC2 §4.3/§4.4).
		// Instead we Connect() directly and consume *events.QR in myEventHandler
		// (see handleQRCodes), which pair.go dispatches to every handler anyway.
		err = client.Connect()
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Erro de conexão WebSocket (EOF). Tentando reconectar em 5 segundos...", cd.Instance.Id)
				time.Sleep(5 * time.Second)
				err = client.Connect()
				if err != nil {
					w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Falha na segunda tentativa de conexão: %v", cd.Instance.Id, err)
					return
				}
			} else if strings.Contains(err.Error(), "username/password authentication failed") {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogWarn("[%s] Proxy authentication failed during QR connection, attempting without proxy", cd.Instance.Id)

				// Desabilita o proxy
				client.SetProxy(nil)

				// Tenta conectar sem proxy
				err = client.Connect()
				if err != nil {
					w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Failed to connect even without proxy: %v", cd.Instance.Id, err)
					return
				}
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Successfully connected without proxy", cd.Instance.Id)
			} else {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Failed to connect: %v", cd.Instance.Id, err)
				return
			}
		}

	}

	// Removed auto-reconnect logic to prevent infinite loops

	for {
		select {
		case <-w.killChannel.Get(cd.Instance.Id):
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("Received kill signal for user '%s'", cd.Instance.Id)
			client.Disconnect()

			w.clientPointer.Delete(cd.Instance.Id)
			w.myClientPointer.Delete(cd.Instance.Id)

			// Limpar cache de userInfo para esta instância
			w.userInfoCache.Delete(cd.Instance.Token)
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] UserInfo cache cleared for token: %s", cd.Instance.Id, cd.Instance.Token)

			cd.Instance.Connected = false

			err := w.instanceRepository.UpdateConnected(cd.Instance.Id, cd.Instance.Connected, cd.Instance.DisconnectReason)
			if err != nil {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Error updating instance: %s", cd.Instance.Id, err)
			}

			postMap := make(map[string]interface{})

			postMap["event"] = "LoggedOut"

			dataMap := make(map[string]interface{})

			dataMap["reason"] = "Logged out"

			postMap["data"] = dataMap

			postMap["instanceToken"] = mycli.token
			postMap["instanceId"] = mycli.userID
			postMap["instanceName"] = cd.Instance.Name

			var queueName string

			if _, ok := postMap["event"]; ok {
				queueName = strings.ToLower(fmt.Sprintf("%s.%s", cd.Instance.Id, postMap["event"]))
			}

			values, err := json.Marshal(postMap)
			if err != nil {
				w.loggerWrapper.GetLogger(cd.Instance.Id).LogError("[%s] Failed to marshal JSON for queue", cd.Instance.Id)
				return
			}

			go w.CallWebhook(cd.Instance, queueName, values)

			if mycli.config.AmqpGlobalEnabled || mycli.config.NatsGlobalEnabled {
				go mycli.service.SendToGlobalQueues(postMap["event"].(string), values, mycli.userID)
			}

			// restart client
			w.loggerWrapper.GetLogger(cd.Instance.Id).LogInfo("[%s] Restarting client", cd.Instance.Id)
			w.StartClient(cd)
			return
		default:
			time.Sleep(1000 * time.Millisecond)
		}
	}
}

func schedulePresenceUpdates(mycli *MyClient) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Verificar se a instância ainda existe
			instance, err := mycli.instanceRepository.GetInstanceByID(mycli.userID)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Instance no longer exists, stopping presence updates", mycli.userID)
				return // Encerra a goroutine se a instância não existir mais
			}

			// This loop is only started for alwaysOnline instances, but the
			// setting can be turned off while it runs. Going available again
			// would re-silence the operator's phone, so stop refreshing presence
			// as soon as the fresh DB row says alwaysOnline is off (issues
			// #70/#54/#55). Read the row instead of mycli.Instance so a runtime
			// toggle is honoured without waiting for a reconnect.
			if !instance.AlwaysOnline {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] alwaysOnline is off, stopping presence updates", mycli.userID)
				return
			}

			processPresenceUpdates(mycli)

			ticker.Stop()
			randomInterval := time.Duration(1+rand.Intn(3)) * time.Hour
			ticker = time.NewTicker(randomInterval)

		case <-mycli.killChannel.Get(mycli.userID):
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Received kill signal, stopping presence updates", mycli.userID)
			return // Encerra a goroutine quando receber sinal de kill
		}
	}
}

func processPresenceUpdates(mycli *MyClient) {
	now := time.Now()
	location, _ := time.LoadLocation("America/Sao_Paulo")
	nowSp := now.In(location)

	if nowSp.Hour() >= 1 && nowSp.Hour() < 24 {
		err := mycli.WAClient.SendPresence(context.Background(), types.PresenceUnavailable)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to set presence as unavailable %v", mycli.userID, err)
		} else {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Marked self as unavailable", mycli.userID)
		}

		time.Sleep(time.Duration(1+rand.Intn(5)) * time.Second)

		err = mycli.WAClient.SendPresence(context.Background(), types.PresenceAvailable)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to set presence as available %v", mycli.userID, err)
		} else {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Marked self as available", mycli.userID)
		}
	}
}

// handleQRCodes forwards a batch of QR codes (events.QR.Codes) to the manager,
// rotating them with whatsmeow's native timing (first code ~60s, the rest ~20s)
// WITHOUT using GetQRChannel. GetQRChannel is deliberately avoided for new-device
// pairing because, in the installed whatsmeow, its qrChannel handler both
// auto-confirms PairPasskeyConfirmation when SkipHandoffUX is set (racing our
// own confirm flow) and Disconnects the socket when codes run out — either of
// which breaks an in-flight passkey ceremony (DOC2 §4.3/§4.4). Consuming
// events.QR here keeps the socket alive for as long as pairing (QR or passkey)
// needs, since events.QR is dispatched to every handler by pair.go regardless.
//
// This preserves the original GetQRChannel-loop behavior byte-for-byte for the
// per-code work (max-count enforcement, PNG encode, DB persist, webhook/queue
// fan-out) and the timeout teardown; only the trigger (batch vs. per-code) and
// the rotation/self-timer are new. Runs in its own goroutine so it never blocks
// the whatsmeow event dispatch.
func (mycli *MyClient) handleQRCodes(codes []string) {
	go func() {
		instanceID := mycli.userID
		for i, code := range codes {
			// A successful pair (Store.ID set) or an in-flight passkey ceremony
			// supersedes QR — stop rotating WITHOUT tearing down. Store.ID stays
			// nil throughout a passkey ceremony (it is only set at PairSuccess),
			// so we must also consult the ceremony store, otherwise a ceremony
			// that outlasts QR rotation would have its socket/client torn down.
			if mycli.WAClient == nil || mycli.WAClient.Store.ID != nil {
				return
			}
			if mycli.passkeyCeremony != nil && mycli.passkeyCeremony.HasActiveByInstance(instanceID) {
				mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] Passkey ceremony in progress — pausing QR rotation, keeping socket alive", instanceID)
				return
			}

			mycli.qrcodeCount++

			if mycli.config.QrcodeMaxCount > 0 {
				mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] QR code generated #%d (max: %d)", instanceID, mycli.qrcodeCount, mycli.config.QrcodeMaxCount)
			} else {
				mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] QR code generated #%d (limit disabled)", instanceID, mycli.qrcodeCount)
			}

			// Max-count reached: force logout + teardown + QRTimeout (0 = disabled).
			// But never tear down while a passkey ceremony is in flight.
			if mycli.config.QrcodeMaxCount > 0 && mycli.qrcodeCount >= mycli.config.QrcodeMaxCount {
				if mycli.passkeyCeremony != nil && mycli.passkeyCeremony.HasActiveByInstance(instanceID) {
					mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] QR max-count reached but passkey ceremony active — not tearing down", instanceID)
					return
				}
				mycli.loggerWrapper.GetLogger(instanceID).LogWarn("[%s] Maximum QR code count reached (%d), forcing logout and QRTimeout", instanceID, mycli.config.QrcodeMaxCount)

				if mycli.WAClient.IsConnected() {
					if err := mycli.WAClient.Logout(context.Background()); err != nil {
						mycli.loggerWrapper.GetLogger(instanceID).LogWarn("[%s] Error during forced logout: %v", instanceID, err)
					}
				}
				mycli.teardownQR(fmt.Sprintf("Maximum QR code count (%d) reached", mycli.config.QrcodeMaxCount), true)
				return
			}

			if mycli.config.LogType != "json" {
				fmt.Println("QR code:\n", code)
			}

			image, _ := qrcode.Encode(code, qrcode.Medium, 256)
			base64qrcode := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)
			base64WithCode := base64qrcode + "|" + code

			if err := mycli.instanceRepository.UpdateQrcode(instanceID, base64WithCode); err != nil {
				mycli.loggerWrapper.GetLogger(instanceID).LogError("[%s] Error updating instance: %s", instanceID, err)
			}

			postMap := map[string]interface{}{
				"event": "QRCode",
				"data": map[string]interface{}{
					"qrcode":   base64qrcode,
					"code":     code,
					"count":    mycli.qrcodeCount,
					"maxCount": mycli.config.QrcodeMaxCount,
				},
				"instanceToken": mycli.token,
				"instanceId":    instanceID,
				"instanceName":  mycli.Instance.Name,
			}
			queueName := strings.ToLower(fmt.Sprintf("%s.%s", instanceID, "QRCode"))
			if values, err := json.Marshal(postMap); err == nil {
				go mycli.service.CallWebhook(mycli.Instance, queueName, values)
				if mycli.config.AmqpGlobalEnabled || mycli.config.NatsGlobalEnabled {
					go mycli.service.SendToGlobalQueues("QRCode", values, instanceID)
				}
			} else {
				mycli.loggerWrapper.GetLogger(instanceID).LogError("[%s] Failed to marshal JSON for queue", instanceID)
			}

			// Rotation timing: first code lives ~60s, subsequent ~20s (whatsmeow native).
			timeout := 20 * time.Second
			if i == 0 {
				timeout = 60 * time.Second
			}
			time.Sleep(timeout)
		}

		// Ran out of codes without a PairSuccess. Treat as QR timeout (mirrors
		// GetQRChannel's "timeout") — UNLESS a passkey ceremony is in flight, in
		// which case the socket must stay alive for the ceremony to complete.
		if mycli.WAClient != nil && mycli.WAClient.Store.ID == nil {
			if mycli.passkeyCeremony != nil && mycli.passkeyCeremony.HasActiveByInstance(instanceID) {
				mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] QR codes exhausted but passkey ceremony active — keeping socket alive", instanceID)
				return
			}
			mycli.teardownQR("", false)
		}
	}()
}

// teardownQR clears the QR state and emits a QRTimeout event, then signals the
// kill channel so StartClient's select loop performs the actual disconnect and
// map cleanup. IMPORTANT: this method must NOT delete from the shared
// clientPointer/myClientPointer/killChannel maps itself — those are unsynchronized
// service-wide maps and this runs in the handleQRCodes goroutine; doing the
// delete()s here (concurrent with other instances' goroutines and the whatsmeow
// dispatch) risks a `fatal error: concurrent map writes`. The kill-channel send
// is blocking (like the original GetQRChannel timeout branch) so the signal is
// never dropped and the socket can't be orphaned. Cleanup happens in the
// StartClient goroutine, the single writer of those maps for this instance.
// If reason is non-empty it is included in the QRTimeout payload (max-count path).
func (mycli *MyClient) teardownQR(reason string, forceLogout bool) {
	instanceID := mycli.userID

	if err := mycli.instanceRepository.UpdateQrcode(instanceID, ""); err != nil {
		mycli.loggerWrapper.GetLogger(instanceID).LogError("[%s] Error updating instance: %s", instanceID, err)
	}

	if reason != "" {
		if err := mycli.instanceRepository.UpdateConnected(instanceID, false, reason); err != nil {
			mycli.loggerWrapper.GetLogger(instanceID).LogError("[%s] Error updating instance status: %v", instanceID, err)
		}
	}

	data := map[string]interface{}{}
	if reason != "" {
		data["reason"] = reason
		data["qrcount"] = mycli.qrcodeCount
		data["maxCount"] = mycli.config.QrcodeMaxCount
		data["forceLogout"] = forceLogout
	}
	postMap := map[string]interface{}{
		"event":         "QRTimeout",
		"data":          data,
		"instanceToken": mycli.token,
		"instanceId":    instanceID,
		"instanceName":  mycli.Instance.Name,
	}
	queueName := strings.ToLower(fmt.Sprintf("%s.%s", instanceID, "QRTimeout"))
	if values, err := json.Marshal(postMap); err == nil {
		go mycli.service.CallWebhook(mycli.Instance, queueName, values)
		if mycli.config.AmqpGlobalEnabled || mycli.config.NatsGlobalEnabled {
			go mycli.service.SendToGlobalQueues("QRTimeout", values, instanceID)
		}
	}

	// Signal StartClient's select loop to disconnect and clean up the shared
	// maps (it is the single writer for this instance). Blocking send mirrors
	// the original timeout branch so the signal is never dropped.
	mycli.loggerWrapper.GetLogger(instanceID).LogWarn("[%s] QR timeout — signaling kill channel", instanceID)
	if killChan, exists := mycli.killChannel.Lookup(instanceID); exists {
		killChan <- true
	}
}

// handlePollVote decrypts a poll vote and stores it.
//
// It MUST be called before myEventHandler's LID/PN JID swap: whatsmeow derives
// the vote's GCM additional data from msg.Info.Sender and resolves the poll's
// message secret by (msg.Info.Chat, original sender, poll message id). Rewriting
// those JIDs first (LID -> phone number) makes a vote cast from a LID-addressed
// contact fail with "cipher: message authentication failed", so only the poll
// author's own vote would ever decrypt.
func (mycli *MyClient) handlePollVote(evt *events.Message) {
	log := mycli.loggerWrapper.GetLogger(mycli.userID)

	client := mycli.clientPointer.Get(mycli.userID)
	if client == nil {
		log.LogWarn("[%s] Poll vote received but no client is available", mycli.userID)
		return
	}

	decrypted, err := client.DecryptPollVote(context.Background(), evt)
	if err != nil {
		log.LogError("[%s] Failed to decrypt vote: %v", mycli.userID, err)
		return
	}

	log.LogInfo("[%s] Decrypted poll vote with %d selected option(s)", mycli.userID, len(decrypted.SelectedOptions))

	if mycli.pollService == nil {
		return
	}

	pollKey := evt.Message.GetPollUpdateMessage().GetPollCreationMessageKey()
	if pollKey == nil {
		log.LogWarn("[%s] PollCreationMessageKey not found", mycli.userID)
		return
	}

	// Snapshot the identifiers we need: the swap later mutates evt.Info, and the
	// stored vote should keep the sending chat as it arrived.
	info := evt.Info
	instanceID := mycli.Instance.Id
	userID := mycli.userID

	// Resolve the voter to the phone-number form the rest of the API uses (the
	// webhook Sender, quoted replies, etc.). The vote arrives LID-addressed and
	// with a device suffix; storing it raw made voterPhone hold the LID number
	// instead of the phone. done here because this is where the client lives —
	// BuildPollVoteFromEvent stays pure.
	voter := info.Sender.ToNonAD()
	if voter.Server == types.HiddenUserServer {
		if mycli.WAClient != nil && mycli.WAClient.Store != nil && mycli.WAClient.Store.LIDs != nil {
			if pn, err := mycli.WAClient.Store.LIDs.GetPNForLID(context.Background(), voter); err == nil && !pn.IsEmpty() {
				log.LogInfo("[%s] Resolved poll voter %s to %s", mycli.userID, voter, pn.ToNonAD())
				voter = pn.ToNonAD()
			}
		}
	}
	info.Sender = voter
	info.SenderAlt = info.SenderAlt.ToNonAD()

	if info.Sender.Server == types.HiddenUserServer {
		// No LID->PN mapping available (yet): keep the LID but make clear that
		// the phone is unknown rather than storing the LID number as if it were one.
		log.LogWarn("[%s] No PN mapping for poll voter %s; storing the LID form", mycli.userID, info.Sender)
	}

	// Saving touches the database, so keep it off the event dispatch path.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				mycli.loggerWrapper.GetLogger(userID).LogError("[%s] Panic ao salvar voto: %v", userID, r)
			}
		}()

		pollInfo := &types.MessageInfo{
			ID: pollKey.GetID(),
			MessageSource: types.MessageSource{
				Chat: info.Chat,
			},
		}

		pollVote := poll_service.BuildPollVoteFromEvent(
			pollInfo,
			&info,
			decrypted,
			"", // CompanyID não disponível no MyClient, será vazio
			instanceID,
		)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := mycli.pollService.SavePollVote(ctx, pollVote); err != nil {
			mycli.loggerWrapper.GetLogger(userID).LogError("[%s] Failed to save poll vote to database: %v", userID, err)
		} else {
			mycli.loggerWrapper.GetLogger(userID).LogInfo("[%s] Poll vote saved to database successfully", userID)
		}
	}()
}

// scheduleReconnect heals a dropped or hung socket. For an already-paired device
// it goes through ReconnectClient (with the reconnect backoff); mid-QR-pairing it
// reconnects the same client in place (no device reset), so it cannot race the
// QR-rotation goroutine over qrcodeCount.
func (mycli *MyClient) scheduleReconnect(reason string) {
	if mycli.WAClient == nil {
		return
	}

	if mycli.WAClient.Store.ID != nil {
		go func(instanceID string) {
			mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] %s detected, restarting instance", instanceID, reason)

			// Backoff: a logged-out device loops Disconnected -> reconnect ->
			// QR -> no scan -> forced logout -> Disconnected forever. The first
			// few restarts go straight through (healthy socket drop); after that
			// we slow the loop down instead of hammering WhatsApp.
			if allowed, wait := reconnectAllowed(instanceID); !allowed {
				if wait < 0 {
					mycli.loggerWrapper.GetLogger(instanceID).LogWarn("[%s] a reconnect is already scheduled — this event will not open another", instanceID)
					return
				}
				mycli.loggerWrapper.GetLogger(instanceID).LogWarn("[%s] reconnect loop detected: more than %d restarts in %s. Next attempt in %s. If the device was logged out from the phone, only a new QR scan will fix it.", instanceID, reconnectFreeAttempts, reconnectWindow, wait)
				if err := mycli.instanceRepository.UpdateConnected(instanceID, false, fmt.Sprintf("Reconnect backing off — next attempt in %s", wait)); err != nil {
					mycli.loggerWrapper.GetLogger(instanceID).LogWarn("[%s] could not record the backoff reason: %v", instanceID, err)
				}
				time.Sleep(wait)
				reconnectWaitDone(instanceID)
			}

			if err := mycli.service.ReconnectClient(instanceID); err != nil {
				mycli.loggerWrapper.GetLogger(instanceID).LogError("[%s] Failed to restart instance: %v", instanceID, err)
			}
		}(mycli.userID)
		return
	}

	// Still mid-QR-pairing: reconnect the SAME client/session in place rather
	// than ReconnectClient's teardown-and-mint-a-new-device path. This touches no
	// shared instance maps or qrcodeCount, so it cannot reintroduce the race the
	// Store.ID gate above removes.
	go func(instanceID string) {
		if mycli.WAClient.IsConnected() {
			return // already recovered by the time this goroutine ran
		}
		if err := mycli.WAClient.Connect(); err != nil {
			mycli.loggerWrapper.GetLogger(instanceID).LogWarn("[%s] Reconnect attempt during QR pairing failed: %v", instanceID, err)
		} else {
			mycli.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] Reconnected during QR pairing (same session, no device reset)", instanceID)
		}
	}(mycli.userID)
}

func (mycli *MyClient) myEventHandler(rawEvt interface{}) {
	userID := mycli.userID
	postMap := make(map[string]interface{})
	postMap["data"] = rawEvt
	doWebhook := false

	switch evt := rawEvt.(type) {
	case *events.QR:
		// New-device pairing emits QR codes here (we connect without GetQRChannel
		// so the socket survives a passkey ceremony). Forward + rotate them.
		mycli.handleQRCodes(evt.Codes)
		return
	case *events.AppStateSyncError:
		mycli.handleAppStateSyncError(evt)
		return
	case *events.AppStateSyncComplete:
		if evt.Recovery {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo(
				"[%s] App-state recovery completed for %s at version %d",
				mycli.userID, evt.Name, evt.Version,
			)
		}
		if len(mycli.WAClient.Store.PushName) > 0 && evt.Name == appstate.WAPatchCriticalBlock {
			err := mycli.WAClient.SendPresence(context.Background(), types.PresenceUnavailable)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to send unavailable presence %v", mycli.userID, err)
			} else {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Marked self as unavailable", mycli.userID)
			}
		}
	case *events.Connected, *events.PushNameSetting:
		// A real connection ends the loop, so the backoff counter dies here.
		// Without this, a drop tomorrow would inherit today's restarts.
		reconnectSucceeded(mycli.userID)
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] events.Connected to Whatsapp for user '%s'", mycli.userID, mycli.WAClient.Store.PushName)
		// Refresh the account-level limits cache (new-chat quota / reachout
		// timelock) in the background so /instance/limits does not have to pay
		// the slow MEX query per request.
		mycli.logAccountLimits()
		// Backfill the NCT salt for instances paired before v0.7.2, otherwise
		// cold 1:1 sends keep failing with error 463 (issue #124). No-op when a
		// salt is already stored; rate-limited per instance internally.
		go mycli.ensureNctSaltSynced()
		if len(mycli.WAClient.Store.PushName) > 0 {
			doWebhook = true
			postMap["event"] = "Connected"

			if postMap["data"] != nil {
				jsonBytes, err := json.Marshal(postMap["data"])
				if err != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
					return
				}

				var dataMap map[string]interface{}
				err = json.Unmarshal(jsonBytes, &dataMap)
				if err != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
					return
				}

				postMap["data"] = dataMap
			} else {
				postMap["data"] = make(map[string]interface{})
			}

			dataMap := postMap["data"].(map[string]interface{})

			dataMap["status"] = "open"
			dataMap["jid"] = mycli.WAClient.Store.ID.String()
			dataMap["pushName"] = mycli.WAClient.Store.PushName

			// jid, ok := utils.ParseJID(mycli.WAClient.Store.ID.ToNonAD().User)
			// if ok {
			// 	profilePicUrl, err := mycli.clientPointer.Get(mycli.userID).GetProfilePictureInfo(jid, &whatsmeow.GetProfilePictureParams{
			// 		Preview: false,
			// 	})
			// 	if err != nil {
			// 		w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to get profile picture info: %v", mycli.userID, err)
			// 	} else {
			// 		dataMap["profilePicUrl"] = profilePicUrl.URL
			// 	}
			// }

			postMap["data"] = dataMap

			// Respect the alwaysOnline instance flag. Previously the device was marked
			// online unconditionally on every connect (and the periodic presence job was
			// started), which kept the linked device permanently "available". WhatsApp then
			// delivers messages to that active session and suppresses push notifications on
			// the user's phone. When alwaysOnline is false we now send Unavailable instead.
			var err error
			if mycli.Instance.AlwaysOnline {
				go schedulePresenceUpdates(mycli)

				err = mycli.WAClient.SendPresence(context.Background(), types.PresenceAvailable)
				if err != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to send available presence %v", mycli.userID, err)
				} else {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Marked self as available", mycli.userID)
				}
			} else {
				err = mycli.WAClient.SendPresence(context.Background(), types.PresenceUnavailable)
				if err != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to send unavailable presence %v", mycli.userID, err)
				} else {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Marked self as unavailable (alwaysOnline=false)", mycli.userID)
				}
			}

			mycli.Instance.Connected = true
			mycli.Instance.DisconnectReason = ""
			err = mycli.instanceRepository.UpdateConnected(mycli.Instance.Id, mycli.Instance.Connected, mycli.Instance.DisconnectReason)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error updating instance: %s", mycli.Instance.Id, err)
			}

			err = mycli.instanceRepository.UpdateQrcode(mycli.Instance.Id, "")
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error updating instance: %s", mycli.Instance.Id, err)
			}
		}
	case *events.PairSuccess:
		doWebhook = true
		postMap["event"] = "PairSuccess"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("QR Pair Success for user '%s' with JID '%s' - '%s'", mycli.userID, evt.ID.String(), mycli.WAClient.Store.ID.String())

		instance, err := mycli.instanceRepository.GetInstanceByID(mycli.userID)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error getting instance: %s", mycli.userID, err)
		}

		instance.Qrcode = ""
		instance.Connected = true
		instance.DisconnectReason = ""
		instance.Jid = mycli.WAClient.Store.ID.String()

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Updating JID: %s in Instance: %s", mycli.userID, mycli.WAClient.Store.ID.String(), instance.Jid)

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Attempting to update instance in DB: %+v", mycli.userID, instance)
		err = mycli.instanceRepository.Update(instance)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error updating instance: %s", mycli.userID, err)
		} else {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Instance successfully updated", mycli.userID)
		}

		myUserInfo, found := mycli.userInfoCache.Get(mycli.token)

		if !found {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] No user info cached on pairing?", mycli.userID)
		} else {
			txtid := myUserInfo.(Values).Get("Id")
			token := myUserInfo.(Values).Get("Token")

			updatedUserInfo := utils.UpdateUserInfo(myUserInfo, "Jid", evt.ID.String())

			mycli.userInfoCache.Set(token, updatedUserInfo, cache.NoExpiration)
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] User information set for user '%s'", mycli.userID, txtid)
		}

		if postMap["data"] != nil {
			jsonBytes, err := json.Marshal(postMap["data"])
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
				return
			}

			var dataMap map[string]interface{}
			err = json.Unmarshal(jsonBytes, &dataMap)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
				return
			}

			postMap["data"] = dataMap
		} else {
			postMap["data"] = make(map[string]interface{})
		}

		dataMap := postMap["data"].(map[string]interface{})

		dataMap["status"] = "open"
		dataMap["jid"] = mycli.WAClient.Store.ID.String()

		if mycli.WAClient.Store.PushName != "" {
			dataMap["pushName"] = mycli.WAClient.Store.PushName
		}

		postMap["data"] = dataMap

		// Pairing succeeded — tear down any pending passkey ceremony for this instance.
		mycli.passkeyCeremony.Clear(mycli.userID)
	case *events.PairError:
		// The server accepted the pairing but finishing it locally failed (e.g.
		// the device identity could not be stored). Without this case the error
		// was only visible as an "Unhandled event" warning, and a passkey
		// ceremony would stay stuck until its TTL.
		doWebhook = true
		postMap["event"] = "PairError"
		msg := "unknown pairing error"
		if evt.Error != nil {
			msg = evt.Error.Error()
		}
		mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Pairing failed after pair-success (platform=%s): %s", mycli.userID, evt.Platform, msg)
		if mycli.passkeyCeremony != nil {
			mycli.passkeyCeremony.SetError(mycli.userID, msg)
		}
		postMap["data"] = map[string]interface{}{
			"error":    msg,
			"platform": evt.Platform,
			"stage":    "error",
		}
	case *events.PairPasskeyRequest:
		// The server demands a WebAuthn passkey to finish linking. We CANNOT
		// produce the assertion here (it needs the user's authenticator on the
		// web.whatsapp.com origin) — we only forward the challenge. The browser
		// extension (tools/passkey-helper) runs navigator.credentials.get() and
		// POSTs the assertion back to /passkey-ceremony/{token}/response, which
		// is where SendPasskeyResponse is actually called.
		doWebhook = true
		postMap["event"] = "PasskeyRequest"

		pkJSON, err := json.Marshal(evt.PublicKey)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal passkey publicKey: %v", mycli.userID, err)
			mycli.passkeyCeremony.SetError(mycli.userID, "failed to encode passkey challenge")
			return
		}

		token := mycli.passkeyCeremony.Start(mycli.userID, pkJSON)

		// Build the #wapk payload the extension consumes: base64url({t,b}).
		// `b` must be the PUBLICLY reachable API base the browser can hit
		// (a tunnel / LAN IP in dev) — set PASSKEY_PUBLIC_URL to that base.
		publicBase := os.Getenv("PASSKEY_PUBLIC_URL")
		if publicBase == "" {
			publicBase = "<SET_PASSKEY_PUBLIC_URL>"
		}
		payload := fmt.Sprintf(`{"t":%q,"b":%q}`, token, publicBase)
		wapk := base64.RawURLEncoding.EncodeToString([]byte(payload))
		openURL := "https://web.whatsapp.com/#wapk=" + wapk

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo(
			"[%s] Passkey required. Open this URL in a browser with the Evolution Passkey Helper extension:\n%s\n(ceremony token=%s, base=%s)",
			mycli.userID, openURL, token, publicBase,
		)

		// Surface the ceremony info to webhooks/queues so the manager UI can
		// render the "Abrir WhatsApp Web" button.
		postMap["data"] = map[string]interface{}{
			"ceremonyToken": token,
			"openUrl":       openURL,
			"stage":         "challenge",
		}
	case *events.PairPasskeyConfirmation:
		// The server returned a confirmation code. Per DOC2 §4.2 we NEVER
		// auto-confirm on SkipHandoffUX — we always force skipHandoffUX=false so
		// the extension shows the manual "Confirmar" button, and the actual
		// SendPasskeyConfirmation happens from /passkey-ceremony/{token}/confirm.
		doWebhook = true
		postMap["event"] = "PasskeyConfirmation"
		mycli.passkeyCeremony.SetConfirmation(mycli.userID, evt.Code, false)
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo(
			"[%s] Passkey confirmation code=%s (skipHandoffUX from server=%v, forced to manual)",
			mycli.userID, evt.Code, evt.SkipHandoffUX,
		)
		postMap["data"] = map[string]interface{}{
			"code":  evt.Code,
			"stage": "confirmation",
		}
	case *events.PairPasskeyError:
		doWebhook = true
		postMap["event"] = "PasskeyError"
		msg := "unknown passkey error"
		if evt.Error != nil {
			msg = evt.Error.Error()
		}
		mycli.passkeyCeremony.SetError(mycli.userID, msg)
		mycli.loggerWrapper.GetLogger(mycli.userID).LogError(
			"[%s] Passkey pairing error (continuation=%v): %s", mycli.userID, evt.Continuation, msg,
		)
		postMap["data"] = map[string]interface{}{
			"error": msg,
			"stage": "error",
		}
	case *events.MediaRetry:
		// The sender's phone answered a media-retry request: decrypt it, refresh
		// the direct path and cache the bytes for the next download request.
		mycli.service.HandleMediaRetry(mycli.userID, evt)
		return
	case *events.StreamReplaced:
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Received StreamReplaced event", mycli.userID)
		// The socket was replaced, so any in-flight passkey ceremony's pairing
		// context is gone. Clear it rather than leave the extension polling a
		// dead ceremony (issue #107).
		if mycli.passkeyCeremony.Clear(mycli.userID) {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Cleared in-flight passkey ceremony after stream replacement (issue #107)", mycli.userID)
		}
		return
	case *events.TemporaryBan:
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] User received temporary ban for %s", mycli.userID, evt.Code.String())
		doWebhook = true
		postMap["event"] = "TemporaryBan"

		if postMap["data"] != nil {
			jsonBytes, err := json.Marshal(postMap["data"])
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
				return
			}

			var dataMap map[string]interface{}
			err = json.Unmarshal(jsonBytes, &dataMap)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
				return
			}

			postMap["data"] = dataMap
		} else {
			postMap["data"] = make(map[string]interface{})
		}

		dataMap := postMap["data"].(map[string]interface{})

		dataMap["reason"] = evt.Code.String()
		dataMap["expire"] = evt.Expire

		postMap["data"] = dataMap
	case *events.Message:
		doWebhook = true
		postMap["event"] = "Message"
		// Message received

		// Learn/refresh the chat's disappearing-messages timer so outgoing
		// messages to this chat can carry it (see chatEphemeralCache).
		if msg := evt.Message; msg != nil {
			if pm := msg.GetProtocolMessage(); pm != nil && pm.GetType() == waE2E.ProtocolMessage_EPHEMERAL_SETTING {
				SetCachedChatEphemeral(mycli.userID, evt.Info.Chat, pm.GetEphemeralExpiration())
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Disappearing timer changed for %s: %ds", mycli.userID, evt.Info.Chat.String(), pm.GetEphemeralExpiration())
			} else if exp := messageEphemeralExpiration(msg); exp > 0 {
				SetCachedChatEphemeral(mycli.userID, evt.Info.Chat, exp)
			}
		}

		// Log message arrival with detailed info
		messageSize := "unknown"
		if evt.Message.GetDocumentMessage() != nil && evt.Message.GetDocumentMessage().FileLength != nil {
			messageSize = fmt.Sprintf("%d bytes", *evt.Message.GetDocumentMessage().FileLength)
		} else if evt.Message.GetVideoMessage() != nil && evt.Message.GetVideoMessage().FileLength != nil {
			messageSize = fmt.Sprintf("%d bytes", *evt.Message.GetVideoMessage().FileLength)
		} else if evt.Message.GetImageMessage() != nil && evt.Message.GetImageMessage().FileLength != nil {
			messageSize = fmt.Sprintf("%d bytes", *evt.Message.GetImageMessage().FileLength)
		} else if evt.Message.GetAudioMessage() != nil && evt.Message.GetAudioMessage().FileLength != nil {
			messageSize = fmt.Sprintf("%d bytes", *evt.Message.GetAudioMessage().FileLength)
		}

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] ===== MESSAGE RECEIVED ===== ID: %s, From: %s, Type: %s, Size: %s", mycli.userID, evt.Info.ID, evt.Info.Chat.String(), evt.Info.Type, messageSize)

		// se readMessages for true ele marca como lida
		if mycli.Instance.ReadMessages {
			messageIDs := []string{evt.Info.ID}
			err := mycli.WAClient.MarkRead(context.Background(), messageIDs, time.Now(), evt.Info.Sender, evt.Info.Sender)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to auto-mark message as read: %v", mycli.userID, err)
			} else {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Auto-marked message as read from %s", mycli.userID, evt.Info.Chat.String())
			}
		}

		// se ignoreStatus for true e o chat for broadcast ou o id for broadcast retorna
		if mycli.Instance.IgnoreStatus && (strings.Contains(evt.Info.Chat.String(), "@broadcast") || strings.Contains(evt.Info.ID, "@broadcast")) {
			return
		}

		// se ignoreGroup for true e o chat for grupo retorna
		if mycli.Instance.IgnoreGroups && strings.Contains(evt.Info.Chat.String(), "@g.us") {
			return
		}

		// Typebot. Runs after the broadcast/group checks so it respects the same
		// exclusions, and only handles text -- media does not open or advance a
		// conversation.
		if text := evt.Message.GetConversation(); text != "" {
			mycli.service.DispatchToTypebot(mycli.Instance, evt.Info.Chat.String(), evt.Info.PushName, text, evt.Info.IsFromMe)
		} else if extended := evt.Message.GetExtendedTextMessage().GetText(); extended != "" {
			mycli.service.DispatchToTypebot(mycli.Instance, evt.Info.Chat.String(), evt.Info.PushName, extended, evt.Info.IsFromMe)
		}

		// Verifica advanced settings para ignorar grupos
		if (mycli.config.EventIgnoreGroup || mycli.Instance.IgnoreGroups) && strings.Contains(evt.Info.Chat.String(), "@g.us") {
			return
		}

		// Verifica advanced settings para ignorar status/broadcast
		if (mycli.config.EventIgnoreStatus || mycli.Instance.IgnoreStatus) && (strings.Contains(evt.Info.Chat.String(), "@broadcast") || strings.Contains(evt.Info.ID, "@broadcast")) {
			return
		}

		// Poll vote decryption must run BEFORE the JID swap below mutates the
		// event. whatsmeow derives the vote's GCM additional data from
		// msg.Info.Sender and looks the message secret up by msg.Info.Chat;
		// inverting those JIDs first makes a vote from a LID-addressed contact
		// fail with "cipher: message authentication failed".
		if evt.Message.GetPollUpdateMessage() != nil {
			mycli.handlePollVote(evt)
		}

		// Trata o caso especial onde Sender é @lid e SenderAlt é @s.whatsapp.net
		// Neste caso, devemos inverter: Sender e Chat devem ser @s.whatsapp.net, SenderAlt deve ser @lid
		senderStr := evt.Info.Sender.String()
		senderAltStr := evt.Info.SenderAlt.String()
		chatStr := evt.Info.Chat.String()

		if strings.Contains(senderStr, "@lid") && strings.Contains(senderAltStr, "@s.whatsapp.net") {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Detected LID/WhatsApp JID swap case - Sender: %s, SenderAlt: %s", mycli.userID, senderStr, senderAltStr)

			// Limpa os IDs antes de fazer a troca
			cleanSenderAlt := cleanSenderID(senderAltStr)
			cleanSender := cleanSenderID(senderStr)

			// Inverte: Sender e Chat recebem o @s.whatsapp.net, SenderAlt recebe o @lid
			if cleanedWhatsAppJID, err := types.ParseJID(cleanSenderAlt); err == nil {
				evt.Info.Sender = cleanedWhatsAppJID
				// Se Chat também é @lid, atualiza para @s.whatsapp.net
				if strings.Contains(chatStr, "@lid") {
					evt.Info.Chat = cleanedWhatsAppJID
				}
			}

			if cleanedLID, err := types.ParseJID(cleanSender); err == nil {
				evt.Info.SenderAlt = cleanedLID
			}

			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] JID swap completed - New Sender: %s, New SenderAlt: %s, New Chat: %s",
				mycli.userID, evt.Info.Sender.String(), evt.Info.SenderAlt.String(), evt.Info.Chat.String())
		} else {
			// Comportamento normal: apenas limpa os IDs
			cleanSender := cleanSenderID(senderStr)
			if cleanedJID, err := types.ParseJID(cleanSender); err == nil {
				evt.Info.Sender = cleanedJID
			}

			cleanSenderAlt := cleanSenderID(senderAltStr)
			if cleanedLID, err := types.ParseJID(cleanSenderAlt); err == nil {
				evt.Info.SenderAlt = cleanedLID
			}
		}

		// Trata mensagens enviadas em multi-device (celular primário / WhatsApp Web)
		// O whatsmeow preenche evt.Info.Chat como o próprio número da empresa e armazena o lead de destino
		// em evt.Info.DeviceSentMeta.DestinationJID.
		var validDestJID *types.JID
		if evt.Info.IsFromMe && evt.Info.DeviceSentMeta != nil && evt.Info.DeviceSentMeta.DestinationJID != "" {
			if destJID, err := types.ParseJID(evt.Info.DeviceSentMeta.DestinationJID); err == nil && !destJID.IsEmpty() {
				validDestJID = &destJID
				if !evt.Info.IsGroup {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Outbound multi-device message detected - routing Chat from %s to DestinationJID %s",
						mycli.userID, evt.Info.Chat.String(), destJID.String())
					evt.Info.Chat = destJID
				}
			} else if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to parse DestinationJID '%s': %v",
					mycli.userID, evt.Info.DeviceSentMeta.DestinationJID, err)
			}
		}

		// Auto-marca mensagens como lidas se configurado
		if mycli.Instance.ReadMessages && !evt.Info.IsFromMe {
			go func() {
				time.Sleep(1 * time.Second) // Pequeno delay para parecer mais natural
				err := mycli.WAClient.MarkRead(context.Background(), []types.MessageID{evt.Info.ID}, evt.Info.Timestamp, evt.Info.Chat, evt.Info.Sender)
				if err != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to auto-mark message as read: %v", mycli.userID, err)
				} else {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Auto-marked message as read from %s", mycli.userID, evt.Info.Chat.String())
				}
			}()
		}

		// Edits arrive sealed in a secretEncryptedMessage envelope. Unwrap before typing the
		// message, so it is classified as "edit" and the webhook carries the new text.
		mycli.unwrapSecretEncryptedEdit(evt)

		parsedMessageType := utils.GetMessageType(evt.Message)
		if parsedMessageType == "ignore" || strings.HasPrefix(parsedMessageType, "unknown_protocol_") {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Message ignored because it's a unknown protocol message", mycli.userID)
			return
		}

		// Drop thumbnails before the map round trip: the payload discards them
		// later anyway, so serializing them first is wasted work.
		stripMessageThumbnails(evt.Message)

		if postMap["data"] != nil {
			jsonBytes, err := json.Marshal(postMap["data"])
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
				return
			}

			var dataMap map[string]interface{}
			err = json.Unmarshal(jsonBytes, &dataMap)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
				return
			}

			postMap["data"] = dataMap
		} else {
			postMap["data"] = make(map[string]interface{})
		}

		dataMap, ok := postMap["data"].(map[string]interface{})
		if !ok {
			dataMap = make(map[string]interface{})
		}

		if validDestJID != nil {
			destStr := validDestJID.String()
			dataMap["recipient"] = destStr
			dataMap["Recipient"] = destStr
			if !evt.Info.IsGroup {
				dataMap["chat"] = destStr
				dataMap["Chat"] = destStr
			}
		}

		// Explicit action flags for edit/revoke so consumers do not have to
		// decode protocolMessage.type (a numeric enum: 0 = REVOKE, 14 = MESSAGE_EDIT).
		// See issue #92.
		switch parsedMessageType {
		case "edit":
			dataMap["IsEdit"] = true
			dataMap["messageType"] = "edit"
		case "revoke":
			dataMap["IsRevoke"] = true
			dataMap["messageType"] = "revoke"
		}

		referral := extractReferralFromMessage(evt.Message)

		var quotedMessage *waE2E.Message
		var stanzaID string

		if evt.Message.GetExtendedTextMessage() != nil {
			quotedMessage = evt.Message.GetExtendedTextMessage().GetContextInfo().GetQuotedMessage()
			stanzaID = evt.Message.GetExtendedTextMessage().GetContextInfo().GetStanzaID()
		} else if evt.Message.GetImageMessage() != nil {
			quotedMessage = evt.Message.GetImageMessage().GetContextInfo().GetQuotedMessage()
			stanzaID = evt.Message.GetImageMessage().GetContextInfo().GetStanzaID()
		} else if evt.Message.GetAudioMessage() != nil {
			quotedMessage = evt.Message.GetAudioMessage().GetContextInfo().GetQuotedMessage()
			stanzaID = evt.Message.GetAudioMessage().GetContextInfo().GetStanzaID()
		} else if evt.Message.GetDocumentMessage() != nil {
			quotedMessage = evt.Message.GetDocumentMessage().GetContextInfo().GetQuotedMessage()
			stanzaID = evt.Message.GetDocumentMessage().GetContextInfo().GetStanzaID()
		} else if evt.Message.GetVideoMessage() != nil {
			quotedMessage = evt.Message.GetVideoMessage().GetContextInfo().GetQuotedMessage()
			stanzaID = evt.Message.GetVideoMessage().GetContextInfo().GetStanzaID()
		}

		if stanzaID != "" && quotedMessage != nil {
			quotedMap := make(map[string]interface{})

			quotedMap["stanzaID"] = stanzaID
			quotedMap["quotedMessage"] = quotedMessage

			dataMap["quoted"] = quotedMap
			dataMap["isQuoted"] = true
		}

		if len(referral) > 0 {
			dataMap["referral"] = referral
		}

		if mycli.config.WebhookFiles {
			isMedia := false

			img := evt.Message.GetImageMessage()
			audio := evt.Message.GetAudioMessage()
			document := evt.Message.GetDocumentMessage()
			video := evt.Message.GetVideoMessage()
			sticker := evt.Message.GetStickerMessage()

			// Check for associated child messages (like media in replies)
			var associatedImg *waE2E.ImageMessage
			var associatedAudio *waE2E.AudioMessage
			var associatedDocument *waE2E.DocumentMessage
			var associatedVideo *waE2E.VideoMessage
			var associatedSticker *waE2E.StickerMessage

			if evt.Message.GetAssociatedChildMessage() != nil {
				childMsg := evt.Message.GetAssociatedChildMessage().GetMessage()
				if childMsg != nil {
					associatedImg = childMsg.GetImageMessage()
					associatedAudio = childMsg.GetAudioMessage()
					associatedDocument = childMsg.GetDocumentMessage()
					associatedVideo = childMsg.GetVideoMessage()
					associatedSticker = childMsg.GetStickerMessage()
				}
			}

			if img != nil || audio != nil || document != nil || video != nil || sticker != nil ||
				associatedImg != nil || associatedAudio != nil || associatedDocument != nil ||
				associatedVideo != nil || associatedSticker != nil {
				isMedia = true
			}

			if isMedia {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Processing media message - ID: %s", mycli.userID, evt.Info.ID)

				var data []byte
				var err error
				var extension string
				var mimeType string
				var mediaSize int64

				// Create context with timeout for large files
				downloadCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()

				downloadStart := time.Now()

				// Handle regular media messages
				if img != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Downloading image - ID: %s", mycli.userID, evt.Info.ID)
					data, err = mycli.WAClient.Download(downloadCtx, img)
					extension = ".jpg"
					mimeType = "image/jpeg"
					if img.FileLength != nil {
						mediaSize = int64(*img.FileLength)
					}
				} else if audio != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Downloading audio - ID: %s", mycli.userID, evt.Info.ID)
					data, err = mycli.WAClient.Download(downloadCtx, audio)
					extension = ".ogg"
					mimeType = "audio/ogg"
					if audio.FileLength != nil {
						mediaSize = int64(*audio.FileLength)
					}
				} else if document != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Downloading document - ID: %s, FileName: %s, Size: %d bytes", mycli.userID, evt.Info.ID, document.GetFileName(), document.GetFileLength())
					data, err = mycli.WAClient.Download(downloadCtx, document)
					extension = getExtensionFromMimeType(document.GetMimetype())
					mimeType = document.GetMimetype()
					if document.FileLength != nil {
						mediaSize = int64(*document.FileLength)
					}
				} else if video != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Downloading video - ID: %s, Size: %d bytes", mycli.userID, evt.Info.ID, video.GetFileLength())
					data, err = mycli.WAClient.Download(downloadCtx, video)
					extension = ".mp4"
					mimeType = "video/mp4"
					if video.FileLength != nil {
						mediaSize = int64(*video.FileLength)
					}
				} else if sticker != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Downloading sticker - ID: %s", mycli.userID, evt.Info.ID)
					data, err = mycli.WAClient.Download(downloadCtx, sticker)
					extension = ".png"
					mimeType = "image/png"
					if sticker.FileLength != nil {
						mediaSize = int64(*sticker.FileLength)
					}

					if err == nil {
						webpReader := bytes.NewReader(data)
						img, decErr := webp.Decode(webpReader)
						if decErr != nil {
							mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to decode webp sticker, keeping raw webp: %v", mycli.userID, decErr)
							extension = ".webp"
							mimeType = "image/webp"
						} else {
							var pngBuffer bytes.Buffer
							if encErr := png.Encode(&pngBuffer, img); encErr != nil {
								mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to encode png from sticker, keeping raw webp: %v", mycli.userID, encErr)
								extension = ".webp"
								mimeType = "image/webp"
							} else {
								data = pngBuffer.Bytes()
							}
						}
					}
					// Handle associated child media messages
				} else if associatedImg != nil {
					data, err = mycli.WAClient.Download(context.Background(), associatedImg)
					extension = ".jpg"
					mimeType = "image/jpeg"
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Processing associated child image message", mycli.userID)
				} else if associatedAudio != nil {
					data, err = mycli.WAClient.Download(context.Background(), associatedAudio)
					extension = ".ogg"
					mimeType = "audio/ogg"
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Processing associated child audio message", mycli.userID)
				} else if associatedDocument != nil {
					data, err = mycli.WAClient.Download(context.Background(), associatedDocument)
					extension = getExtensionFromMimeType(associatedDocument.GetMimetype())
					mimeType = associatedDocument.GetMimetype()
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Processing associated child document message", mycli.userID)
				} else if associatedVideo != nil {
					data, err = mycli.WAClient.Download(context.Background(), associatedVideo)
					extension = ".mp4"
					mimeType = "video/mp4"
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Processing associated child video message", mycli.userID)
				} else if associatedSticker != nil {
					data, err = mycli.WAClient.Download(context.Background(), associatedSticker)
					extension = ".png"
					mimeType = "image/png"
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Processing associated child sticker message", mycli.userID)

					if err == nil {
						webpReader := bytes.NewReader(data)
						img, decErr := webp.Decode(webpReader)
						if decErr != nil {
							mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to decode webp sticker, keeping raw webp: %v", mycli.userID, decErr)
							extension = ".webp"
							mimeType = "image/webp"
						} else {
							var pngBuffer bytes.Buffer
							if encErr := png.Encode(&pngBuffer, img); encErr != nil {
								mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Failed to encode png from associated sticker, keeping raw webp: %v", mycli.userID, encErr)
								extension = ".webp"
								mimeType = "image/webp"
							} else {
								data = pngBuffer.Bytes()
							}
						}
					}
				}

				downloadDuration := time.Since(downloadStart)

				if err != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to download media - ID: %s, Size: %d bytes, Duration: %v, Error: %v", mycli.userID, evt.Info.ID, mediaSize, downloadDuration, err)

					// Check if it's a timeout error
					if downloadCtx.Err() == context.DeadlineExceeded {
						mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Download timeout exceeded (5 minutes) for large file - ID: %s, Size: %d bytes", mycli.userID, evt.Info.ID, mediaSize)
					}

					// Don't return here - continue processing the message without media
					mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Continuing message processing without media download - ID: %s", mycli.userID, evt.Info.ID)
				} else {
					actualSize := len(data)
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Media download successful - ID: %s, Expected: %d bytes, Actual: %d bytes, Duration: %v", mycli.userID, evt.Info.ID, mediaSize, actualSize, downloadDuration)

					// Check for size mismatch
					if mediaSize > 0 && int64(actualSize) != mediaSize {
						mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Size mismatch detected - ID: %s, Expected: %d, Got: %d", mycli.userID, evt.Info.ID, mediaSize, actualSize)
					}

					// Log large file processing
					if actualSize > 13*1024*1024 { // 13MB
						mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Processing large file (>13MB) - ID: %s, Size: %d bytes", mycli.userID, evt.Info.ID, actualSize)
					}
				}

				messageMap, ok := dataMap["Message"].(map[string]interface{})
				if !ok {
					messageMap = make(map[string]interface{})
				}

				// Only process storage if download was successful
				if err == nil && len(data) > 0 {
					if mycli.config.MinioEnabled {
						fileName := evt.Info.ID + extension
						storageStart := time.Now()

						mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Uploading to S3/Minio - ID: %s, FileName: %s, Size: %d bytes", mycli.userID, evt.Info.ID, fileName, len(data))

						mediaURL, err := mycli.mediaStorage.Store(context.Background(), data, fileName, mimeType)
						storageDuration := time.Since(storageStart)

						if err != nil {
							mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to store media in S3/Minio - ID: %s, Size: %d bytes, Duration: %v, Error: %v", mycli.userID, evt.Info.ID, len(data), storageDuration, err)

							// Continue processing without storage URL
							mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Continuing message processing without S3 URL - ID: %s", mycli.userID, evt.Info.ID)
						} else {
							mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] S3/Minio upload successful - ID: %s, Size: %d bytes, Duration: %v, URL: %s", mycli.userID, evt.Info.ID, len(data), storageDuration, mediaURL)
							messageMap["mediaUrl"] = mediaURL
							messageMap["mimetype"] = mimeType
						}
					} else {
						mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Encoding to base64 - ID: %s, Size: %d bytes", mycli.userID, evt.Info.ID, len(data))
						encodeStart := time.Now()

						encodeData := base64.StdEncoding.EncodeToString(data)
						encodeDuration := time.Since(encodeStart)

						mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Base64 encoding completed - ID: %s, Original: %d bytes, Encoded: %d chars, Duration: %v", mycli.userID, evt.Info.ID, len(data), len(encodeData), encodeDuration)
						messageMap["base64"] = encodeData
					}
				} else {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Skipping media storage due to download failure - ID: %s", mycli.userID, evt.Info.ID)
				}

				dataMap["Message"] = messageMap
			}
		}

		isGroup := strings.HasSuffix(evt.Info.Chat.String(), "@g.us")
		if isGroup {
			groupData, err := mycli.getGroupInfoCached(evt.Info.Chat)
			if err == nil {
				dataMap["groupData"] = groupData
			}
		}

		delete(dataMap, "RawMessage")

		if message, ok := dataMap["Message"].(map[string]interface{}); ok {
			if imageMessage, ok := message["imageMessage"].(map[string]interface{}); ok {
				delete(imageMessage, "JPEGThumbnail")
				message["imageMessage"] = imageMessage
				dataMap["Message"] = message
			}

			if videoMessage, ok := message["videoMessage"].(map[string]interface{}); ok {
				delete(videoMessage, "JPEGThumbnail")
				message["videoMessage"] = videoMessage
				dataMap["Message"] = message
			}

			if documentMessage, ok := message["documentMessage"].(map[string]interface{}); ok {
				delete(documentMessage, "JPEGThumbnail")
				message["documentMessage"] = documentMessage
				dataMap["Message"] = message
			}
		}

		postMap["data"] = dataMap

		if mycli.config.DatabaseSaveMessages {
			// Messages sent from another own device arrive here with IsFromMe;
			// only genuinely inbound ones are "Received".
			status := "Received"
			if evt.Info.IsFromMe {
				status = "Sent"
			}
			message := message_model.Message{
				MessageID:  evt.Info.ID,
				InstanceId: mycli.userID,
				Timestamp:  evt.Info.Timestamp.Format("2006-01-02 15:04:05"),
				Status:     status,
				Source:     evt.Info.Chat.ToNonAD().User,
				Referral:   referral,
			}

			mycli.persistMessageAsync(message)
		}

		// ===== BUTTON CLICK EVENT DETECTION =====
		// Detecta cliques em botões e emite evento separado "ButtonClick"
		// Suporta 3 formatos: ButtonsResponseMessage, InteractiveResponseMessage (NativeFlow), TemplateButtonReplyMessage
		var buttonClickData map[string]interface{}

		if resp := evt.Message.GetButtonsResponseMessage(); resp != nil {
			// Legacy buttons response
			buttonClickData = map[string]interface{}{
				"buttonId":   resp.GetSelectedButtonID(),
				"buttonText": resp.GetSelectedDisplayText(),
				"type":       "buttons_response",
			}
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Button click detected (legacy): buttonId=%s, buttonText=%s", mycli.userID, resp.GetSelectedButtonID(), resp.GetSelectedDisplayText())
		} else if resp := evt.Message.GetInteractiveResponseMessage(); resp != nil {
			// NativeFlow interactive response (quick_reply, cta_url, cta_call, cta_copy)
			if nf := resp.GetNativeFlowResponseMessage(); nf != nil {
				buttonId := ""
				buttonText := ""
				// Parse paramsJSON to extract id and display_text
				if nf.GetParamsJSON() != "" {
					var params map[string]interface{}
					if err := json.Unmarshal([]byte(nf.GetParamsJSON()), &params); err == nil {
						if id, ok := params["id"].(string); ok {
							buttonId = id
						}
						if dt, ok := params["display_text"].(string); ok {
							buttonText = dt
						}
					}
				}
				buttonClickData = map[string]interface{}{
					"buttonId":   buttonId,
					"buttonText": buttonText,
					"type":       "native_flow_response",
					"name":       nf.GetName(),
					"paramsJSON": nf.GetParamsJSON(),
				}
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Button click detected (native_flow): name=%s, buttonId=%s, buttonText=%s", mycli.userID, nf.GetName(), buttonId, buttonText)
			}
		} else if resp := evt.Message.GetTemplateButtonReplyMessage(); resp != nil {
			// Template button reply
			buttonClickData = map[string]interface{}{
				"buttonId":   resp.GetSelectedID(),
				"buttonText": resp.GetSelectedDisplayText(),
				"type":       "template_button_reply",
			}
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Button click detected (template): buttonId=%s, buttonText=%s", mycli.userID, resp.GetSelectedID(), resp.GetSelectedDisplayText())
		} else if resp := evt.Message.GetListResponseMessage(); resp != nil {
			// List response (single select)
			buttonClickData = map[string]interface{}{
				"buttonId":    resp.GetSingleSelectReply().GetSelectedRowID(),
				"buttonText":  resp.GetTitle(),
				"type":        "list_response",
				"description": resp.GetDescription(),
			}
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] List selection detected: rowId=%s, title=%s", mycli.userID, resp.GetSingleSelectReply().GetSelectedRowID(), resp.GetTitle())
		}

		// Se detectou clique em botão, emite evento separado "ButtonClick"
		if buttonClickData != nil {
			buttonClickMap := map[string]interface{}{
				"event": "ButtonClick",
				"data": map[string]interface{}{
					"buttonId":   buttonClickData["buttonId"],
					"buttonText": buttonClickData["buttonText"],
					"type":       buttonClickData["type"],
					"phone":      dataMap["Sender"],
					"jid":        dataMap["Sender"],
					"pushName":   dataMap["PushName"],
					"messageId":  dataMap["ID"],
					"chat":       dataMap["Chat"],
					"fromMe":     dataMap["FromMe"],
					"timestamp":  evt.Info.Timestamp.Unix(),
					"extraData":  buttonClickData,
				},
				"instanceToken": mycli.token,
				"instanceId":    mycli.userID,
				"instanceName":  mycli.Instance.Name,
			}

			buttonClickJSON, err := json.Marshal(buttonClickMap)
			if err == nil {
				buttonClickQueue := strings.ToLower(fmt.Sprintf("%s.buttonclick", userID))
				go mycli.service.CallWebhook(mycli.Instance, buttonClickQueue, buttonClickJSON)
				if mycli.config.AmqpGlobalEnabled || mycli.config.NatsGlobalEnabled {
					go mycli.service.SendToGlobalQueues("ButtonClick", buttonClickJSON, mycli.userID)
				}
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] ===== BUTTON CLICK EVENT DISPATCHED ===== Type: %s, ButtonId: %s", mycli.userID, buttonClickData["type"], buttonClickData["buttonId"])
			}
		}

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] ===== MESSAGE PROCESSING COMPLETED ===== ID: %s, From: %s, Type: %s, Webhook: %v", mycli.userID, evt.Info.ID, evt.Info.Chat.String(), evt.Info.Type, doWebhook)
	case *events.Receipt:
		doWebhook = true
		postMap["event"] = "Receipt"

		// se ignoreGroup for true e o chat for grupo retorna
		if mycli.Instance.IgnoreGroups && strings.Contains(evt.Chat.String(), "@g.us") {
			return
		}

		if mycli.config.EventIgnoreGroup && strings.Contains(evt.Chat.String(), "@g.us") {
			return
		}

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Receipt received with ID: %s from %s with type %s", mycli.userID, evt.MessageIDs[0], evt.SourceString(), evt.Type)

		if evt.Type == types.ReceiptTypeRead || evt.Type == types.ReceiptTypeReadSelf {

			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Message was read by %s", mycli.userID, evt.SourceString())
			if evt.Type == types.ReceiptTypeRead {
				postMap["state"] = "Read"
				for _, v := range evt.MessageIDs {
					messageKey := fmt.Sprintf("%s_%s_%s", mycli.userID, v, "Read")
					if _, found := mycli.processedMessages.Get(messageKey); found {
						mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Message duplicated ignored: %s", mycli.userID, v)
						continue
					}

					mycli.processedMessages.Set(messageKey, true, 30*time.Minute)

					var message message_model.Message

					message.MessageID = v
					message.InstanceId = mycli.userID
					message.Timestamp = evt.Timestamp.Format("2006-01-02 15:04:05")
					message.Status = "Read"
					message.Source = evt.Chat.ToNonAD().User

					if mycli.config.DatabaseSaveMessages {
						mycli.persistMessageAsync(message)
					}
				}
			} else {
				postMap["state"] = "ReadSelf"
			}
		} else if evt.Type == types.ReceiptTypeDelivered {
			postMap["state"] = "Delivered"

			var message message_model.Message

			message.MessageID = evt.MessageIDs[0]
			message.InstanceId = mycli.userID
			message.Timestamp = evt.Timestamp.Format("2006-01-02 15:04:05")
			message.Status = "Delivered"
			message.Source = evt.Chat.ToNonAD().User

			messageKey := fmt.Sprintf("%s_%s_%s", mycli.userID, evt.MessageIDs[0], "Delivered")
			if _, found := mycli.processedMessages.Get(messageKey); found {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Message duplicated ignored: %s", mycli.userID, evt.MessageIDs[0])
				return
			}

			mycli.processedMessages.Set(messageKey, true, 30*time.Minute)

			if mycli.config.DatabaseSaveMessages {
				mycli.persistMessageAsync(message)
			}

			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Message delivered to %s", mycli.userID, evt.SourceString())
		} else {
			return
		}
	case *events.Presence:
		doWebhook = true
		postMap["event"] = "Presence"
		// Explicit top-level fields so consumers don't depend on types.JID/time marshaling.
		postMap["from"] = evt.From.String()

		if evt.Unavailable {
			postMap["state"] = "offline"
			if evt.LastSeen.IsZero() {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] User is now offline", mycli.userID)
			} else {
				postMap["lastSeen"] = evt.LastSeen.Unix()
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] User is now offline since %s", mycli.userID, evt.LastSeen.Format("2006-01-02 15:04:05"))
			}
		} else {
			postMap["state"] = "online"
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] User is now online", mycli.userID)
		}
	case *events.Archive:
		doWebhook = true
		postMap["event"] = "Archive"

		// postMap["data"] still holds the raw event at this point, so the type
		// assertion below panicked on every Archive event. Same marshal/unmarshal
		// step every other case in this switch already does.
		if postMap["data"] != nil {
			jsonBytes, err := json.Marshal(postMap["data"])
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
				return
			}

			var parsed map[string]interface{}
			if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
				return
			}

			postMap["data"] = parsed
		} else {
			postMap["data"] = make(map[string]interface{})
		}

		dataMap := postMap["data"].(map[string]interface{})
		dataMap["JID"] = evt.JID
		dataMap["Timestamp"] = evt.Timestamp
		dataMap["Action"] = evt.Action
		dataMap["FromFullSync"] = evt.FromFullSync
		postMap["data"] = dataMap

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Chat archived", mycli.userID)
	case *events.HistorySync:
		doWebhook = true
		postMap["event"] = "HistorySync"

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] History sync event received %+v", mycli.userID, evt.Data.SyncType)
	case *events.AppState:
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] App state event received %+v", mycli.userID, evt)
	case *events.LoggedOut:
		doWebhook = true
		postMap["event"] = "LoggedOut"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Logged out for reason %s", mycli.userID, evt.Reason.String())

		// Limpar cache de userInfo para esta instância
		mycli.userInfoCache.Delete(mycli.Instance.Token)
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] UserInfo cache cleared for token: %s", mycli.userID, mycli.Instance.Token)

		mycli.Instance.DisconnectReason = evt.Reason.String()
		mycli.Instance.Connected = false
		err := mycli.instanceRepository.UpdateConnected(mycli.Instance.Id, mycli.Instance.Connected, mycli.Instance.DisconnectReason)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error updating instance: %s", mycli.Instance.Id, err)
		}

		if postMap["data"] != nil {
			jsonBytes, err := json.Marshal(postMap["data"])
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
				return
			}

			var dataMap map[string]interface{}
			err = json.Unmarshal(jsonBytes, &dataMap)
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
				return
			}

			postMap["data"] = dataMap
		} else {
			postMap["data"] = make(map[string]interface{})
		}

		dataMap := postMap["data"].(map[string]interface{})

		dataMap["reason"] = evt.Reason.String()

		// Enviar evento LoggedOut para webhook/RabbitMQ ANTES de matar o canal
		postMap["instanceToken"] = mycli.Instance.Token
		postMap["instanceId"] = mycli.userID
		postMap["instanceName"] = mycli.Instance.Name

		values, err := json.Marshal(postMap)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal JSON for LoggedOut event", mycli.userID)
		} else {
			var queueName string
			if _, ok := postMap["event"]; ok {
				queueName = strings.ToLower(fmt.Sprintf("%s.%s", mycli.userID, postMap["event"]))
			}

			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] ===== DISPATCHING LOGGEDOUT EVENT ===== Queue: %s", mycli.userID, queueName)

			// Enviar para webhook/RabbitMQ
			go mycli.service.CallWebhook(mycli.Instance, queueName, values)

			if mycli.config.AmqpGlobalEnabled || mycli.config.NatsGlobalEnabled {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Sending LoggedOut to global queues - AMQP: %v, NATS: %v", mycli.userID, mycli.config.AmqpGlobalEnabled, mycli.config.NatsGlobalEnabled)
				go mycli.service.SendToGlobalQueues(postMap["event"].(string), values, mycli.userID)
			}
		}

		// Agora mata o canal DEPOIS de enviar o evento
		mycli.killChannel.Get(mycli.userID) <- true
	case *events.ChatPresence:
		doWebhook = true
		postMap["event"] = "ChatPresence"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Chat presence received %+v", mycli.userID, evt)
	case *events.CallOffer:
		doWebhook = true
		postMap["event"] = "CallOffer"

		// Verifica se deve rejeitar chamadas automaticamente
		if mycli.Instance.RejectCall {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Auto-rejecting call from %s", mycli.userID, evt.CallCreator.String())

			// Rejeita a chamada
			mycli.WAClient.RejectCall(context.Background(), evt.CallCreator, evt.CallID)

			// Envia mensagem de rejeição se configurada
			if mycli.Instance.MsgRejectCall != "" {
				msg := &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: &mycli.Instance.MsgRejectCall,
					},
				}

				_, err := mycli.WAClient.SendMessage(context.Background(), evt.CallCreator, msg)
				if err != nil {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to send reject call message: %v", mycli.userID, err)
				} else {
					mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Sent reject call message to %s", mycli.userID, evt.CallCreator.String())
				}
			}
			return
		}

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Got call offer %+v", mycli.userID, evt)
	case *events.CallAccept:
		doWebhook = true
		postMap["event"] = "CallAccept"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Got call accept %+v", mycli.userID, evt)
	case *events.CallTerminate:
		doWebhook = true
		postMap["event"] = "CallTerminate"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Got call terminate %+v", mycli.userID, evt)
	case *events.CallOfferNotice:
		doWebhook = true
		postMap["event"] = "CallOfferNotice"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Got call offer notice %+v", mycli.userID, evt)
	case *events.CallRelayLatency:
		doWebhook = true
		postMap["event"] = "CallRelayLatency"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Got call relay latency %+v", mycli.userID, evt)
	case *events.OfflineSyncCompleted:
		doWebhook = true
		postMap["event"] = "OfflineSyncCompleted"
	case *events.NotifyAccountReachoutTimelock:
		// WhatsApp pushed the account's reach-out timelock state — the
		// account-level limit behind error 463. Forward it so operators can react
		// before cold sends start failing; GET /instance/limits exposes the same
		// data on demand.
		doWebhook = true
		postMap["event"] = "AccountReachoutTimelock"
		ends := int64(0)
		if !evt.TimeEnforcementEnds.IsZero() {
			ends = evt.TimeEnforcementEnds.Unix()
		}
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Account reach-out timelock notification: active=%v type=%s ends=%d", mycli.userID, evt.IsActive, evt.EnforcementType, ends)
		postMap["data"] = map[string]interface{}{
			"isActive":            evt.IsActive,
			"enforcementType":     evt.EnforcementType,
			"timeEnforcementEnds": ends,
		}
	case *events.PrivacySettings:
		doWebhook = true
		postMap["event"] = "PrivacySettings"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Privacy settings changed", mycli.userID)
		postMap["data"] = map[string]interface{}{
			"newSettings":         evt.NewSettings,
			"groupAddChanged":     evt.GroupAddChanged,
			"lastSeenChanged":     evt.LastSeenChanged,
			"statusChanged":       evt.StatusChanged,
			"profileChanged":      evt.ProfileChanged,
			"readReceiptsChanged": evt.ReadReceiptsChanged,
			"onlineChanged":       evt.OnlineChanged,
			"callAddChanged":      evt.CallAddChanged,
			"messagesChanged":     evt.MessagesChanged,
			"defenseChanged":      evt.DefenseChanged,
			"stickersChanged":     evt.StickersChanged,
		}
	case *events.Blocklist:
		doWebhook = true
		postMap["event"] = "Blocklist"
		changes := make([]map[string]string, 0, len(evt.Changes))
		for _, c := range evt.Changes {
			changes = append(changes, map[string]string{"jid": c.JID.String(), "action": string(c.Action)})
		}
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Blocklist changed (%s), %d change(s)", mycli.userID, evt.Action, len(changes))
		postMap["data"] = map[string]interface{}{
			"action":  string(evt.Action),
			"dhash":   evt.DHash,
			"changes": changes,
		}
	case *events.NewsletterLiveUpdate:
		doWebhook = true
		postMap["event"] = "NewsletterLiveUpdate"
		postMap["data"] = map[string]interface{}{
			"jid":      evt.JID.String(),
			"time":     evt.Time.Unix(),
			"messages": len(evt.Messages),
		}
	case *events.NewsletterMuteChange:
		doWebhook = true
		postMap["event"] = "NewsletterMuteChange"
		postMap["data"] = map[string]interface{}{
			"id":   evt.ID.String(),
			"mute": string(evt.Mute),
		}
	case *events.ConnectFailure:
		doWebhook = true
		postMap["event"] = "ConnectFailure"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Connection failed with reason %s", mycli.userID, evt.Reason.String())

		// Limpar cache de userInfo para esta instância
		mycli.userInfoCache.Delete(mycli.Instance.Token)
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] UserInfo cache cleared for token: %s", mycli.userID, mycli.Instance.Token)

		mycli.Instance.DisconnectReason = evt.Reason.String()
		mycli.Instance.Connected = false
		err := mycli.instanceRepository.UpdateConnected(mycli.Instance.Id, mycli.Instance.Connected, mycli.Instance.DisconnectReason)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error updating instance: %s", mycli.Instance.Id, err)
		}
	case *events.StreamError:
		// whatsmeow only emits this for stream errors it does not recognise
		// (known ones are handled internally). The socket is not usable, so heal
		// it instead of waiting for the TCP layer to notice.
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Stream error %s; reconnecting", mycli.userID, evt.Code)
		mycli.scheduleReconnect("StreamError")
	case *events.KeepAliveTimeout:
		// The keepalive ping timed out; the socket is likely dead even though the
		// TCP connection may not have noticed. whatsmeow does not act on this
		// itself. Wait for the second consecutive timeout so a single blip does
		// not drop a healthy socket.
		if evt.ErrorCount >= 2 {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Keepalive timed out %d times (last success %s); reconnecting", mycli.userID, evt.ErrorCount, evt.LastSuccess.Format(time.RFC3339))
			mycli.scheduleReconnect("KeepAliveTimeout")
		} else {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Keepalive timeout (attempt %d); waiting for the next one", mycli.userID, evt.ErrorCount)
		}
	case *events.Disconnected:
		doWebhook = true
		postMap["event"] = "Disconnected"

		// A dropped socket gets a fresh pairing context on reconnect, so an
		// in-flight passkey ceremony's challenge can never complete. Clear it
		// instead of leaving the extension polling a dead ceremony until the TTL
		// expires (issue #107).
		if mycli.passkeyCeremony.Clear(mycli.userID) {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Cleared in-flight passkey ceremony after socket disconnect (issue #107)", mycli.userID)
		}

		// Limpar cache de userInfo para esta instância (mas não para reconexão automática)
		mycli.userInfoCache.Delete(mycli.Instance.Token)
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] UserInfo cache cleared for token: %s", mycli.userID, mycli.Instance.Token)

		mycli.Instance.DisconnectReason = "Disconnected emitted because the websocket is closed by the server."
		mycli.Instance.Connected = false
		err := mycli.instanceRepository.UpdateConnected(mycli.Instance.Id, mycli.Instance.Connected, mycli.Instance.DisconnectReason)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error updating instance: %s", mycli.Instance.Id, err)
		}

		mycli.scheduleReconnect("Disconnected")
	case *events.LabelEdit:
		doWebhook = true
		postMap["event"] = "LabelEdit"
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Got label edit %+v", mycli.userID, evt.Action)

		label := label_model.Label{
			InstanceID:   mycli.userID,
			LabelID:      evt.LabelID,
			LabelName:    utils.GetStringValue(evt.Action.Name),
			LabelColor:   fmt.Sprintf("%d", evt.Action.Color),
			PredefinedId: fmt.Sprintf("%d", evt.Action.PredefinedID),
		}

		err := mycli.labelRepository.UpsertLabel(label)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to upsert label: %v", mycli.userID, err)
		}
	case *events.LabelAssociationChat:
		doWebhook = true
		postMap["event"] = "LabelAssociationChat"

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Label association chat received %+v", mycli.userID, evt)
	case *events.LabelAssociationMessage:
		doWebhook = true
		postMap["event"] = "LabelAssociationMessage"

		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Label association message received %+v", mycli.userID, evt)
	case *events.Contact:
		doWebhook = true
		postMap["event"] = "Contact"
	case *events.PushName:
		doWebhook = true
		postMap["event"] = "PushName"
	case *events.Picture:
		doWebhook = true
		postMap["event"] = "Picture"
	case *events.UserAbout:
		doWebhook = true
		postMap["event"] = "UserAbout"
	case *events.IdentityChange:
		doWebhook = false
	case *events.GroupInfo:
		doWebhook = true
		postMap["event"] = "GroupInfo"
	case *events.JoinedGroup:
		doWebhook = true
		postMap["event"] = "JoinedGroup"
	case *events.NewsletterJoin:
		doWebhook = true
		postMap["event"] = "NewsletterJoin"
	case *events.NewsletterLeave:
		doWebhook = true
		postMap["event"] = "NewsletterLeave"
	case *events.UndecryptableMessage:
		jsonEvt, err := json.Marshal(evt)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Undecryptable message received: %s", mycli.userID, evt.Info.ID)
		}
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Undecryptable message received all: %+v", mycli.userID, string(jsonEvt))

		if evt.UnavailableType == "view_once" {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Undecryptable message received view_once: %s", mycli.userID, evt.Info.ID)

			doWebhook = true
			postMap["event"] = "Message"

			postMap["data"] = evt
		} else if strings.HasPrefix(evt.Info.ID, "66") || strings.HasPrefix(evt.Info.ID, "67") {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] ID 66 or 67 found, reconnecting client", mycli.userID)
			mycli.WAClient.Disconnect()
			err := mycli.WAClient.Connect()
			if err != nil {
				mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Error reconnecting client: %s", mycli.userID, err)
			}
		} else {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] ID is not 66 or 67 or view_once, skipping", mycli.userID)
		}
	default:
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] Unhandled event %s: %+v", mycli.userID, fmt.Sprintf("%T", evt), evt)
		return
	}

	if doWebhook {
		postMap["instanceToken"] = mycli.token
		postMap["instanceId"] = mycli.userID
		postMap["instanceName"] = mycli.Instance.Name

		values, err := json.Marshal(postMap)
		if err != nil {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogError("[%s] Failed to marshal JSON for queue", mycli.userID)
			return
		}

		var queueName string
		if _, ok := postMap["event"]; ok {
			queueName = strings.ToLower(fmt.Sprintf("%s.%s", userID, postMap["event"]))
		}

		// Log webhook dispatch
		eventType := "unknown"
		if event, ok := postMap["event"].(string); ok {
			eventType = event
		}

		dataSize := len(values)
		mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] ===== DISPATCHING WEBHOOK ===== Event: %s, Queue: %s, DataSize: %d bytes", mycli.userID, eventType, queueName, dataSize)

		go mycli.service.CallWebhook(mycli.Instance, queueName, values)

		if mycli.config.AmqpGlobalEnabled || mycli.config.NatsGlobalEnabled {
			mycli.loggerWrapper.GetLogger(mycli.userID).LogInfo("[%s] Sending to global queues - Event: %s, AMQP: %v, NATS: %v", mycli.userID, eventType, mycli.config.AmqpGlobalEnabled, mycli.config.NatsGlobalEnabled)
			go mycli.service.SendToGlobalQueues(postMap["event"].(string), values, mycli.userID)
		}
	} else {
		mycli.loggerWrapper.GetLogger(mycli.userID).LogWarn("[%s] ===== WEBHOOK SKIPPED ===== doWebhook=false", mycli.userID)
	}
}

// webhookEventMeta is the minimal view of an event payload CallWebhook needs in
// order to route it. The payload is already-serialized JSON, and unmarshalling
// it into a generic map materializes the whole message tree just to read the
// event name and the chat JID. Every event payload's "data" is a JSON object, so
// this parses in one pass; a fallback below recovers the event name if that ever
// stops being true.
type webhookEventMeta struct {
	Event string `json:"event"`
	Data  struct {
		Info struct {
			Chat string `json:"Chat"`
		} `json:"Info"`
		Chat string `json:"Chat"`
	} `json:"data"`
}

// parseWebhookEvent extracts the event name and (best-effort) the chat JIDs used
// by the group/newsletter fallback routing. It returns ok=false only when the
// payload is not an object or carries no event name, matching the old behaviour.
func parseWebhookEvent(jsonData []byte) (eventType, dataChat, infoChat string, ok bool) {
	var meta webhookEventMeta
	if err := json.Unmarshal(jsonData, &meta); err == nil && meta.Event != "" {
		return meta.Event, meta.Data.Chat, meta.Data.Info.Chat, true
	}

	// The data shape was unexpected: still recover the event name so the event
	// is not silently dropped.
	var top struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(jsonData, &top); err == nil && top.Event != "" {
		return top.Event, "", "", true
	}
	return "", "", "", false
}

func (w *whatsmeowService) CallWebhook(instance *instance_model.Instance, queueName string, jsonData []byte) {
	eventType, dataChat, infoChat, ok := parseWebhookEvent(jsonData)
	if !ok {
		return
	}

	eventArray := strings.Split(instance.Events, ",")

	var subscriptions []string

	if len(eventArray) < 1 {
		subscriptions = append(subscriptions, event_types.MESSAGE)
		subscriptions = append(subscriptions, event_types.SEND_MESSAGE)
	} else {
		for _, arg := range eventArray {
			if !event_types.IsEventType(arg) {
				w.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Message type discarded: %s", instance.Id, arg)
				continue
			}
			if !utils.Find(subscriptions, arg) {
				subscriptions = append(subscriptions, arg)
			}

		}
	}

	if contains(subscriptions, "ALL") {
		w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
		w.sendToQueueOrWebhook(instance, queueName, jsonData)
		return
	}

	w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] subscriptions %s eventType %s", instance.Id, subscriptions, eventType)

	switch eventType {
	case "Message":
		if contains(subscriptions, "MESSAGE") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		} else {
			// Forward to GROUP/NEWSLETTER subscribers even without MESSAGE subscription
			if strings.HasSuffix(infoChat, "@g.us") && contains(subscriptions, "GROUP") {
				w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s (Group)", instance.Id, eventType)
				w.sendToQueueOrWebhook(instance, queueName, jsonData)
			} else if strings.HasSuffix(infoChat, "@newsletter") && contains(subscriptions, "NEWSLETTER") {
				w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s (Newsletter)", instance.Id, eventType)
				w.sendToQueueOrWebhook(instance, queueName, jsonData)
			}
		}
	case "SendMessage":
		if contains(subscriptions, "SEND_MESSAGE") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		} else {
			if strings.HasSuffix(infoChat, "@g.us") && contains(subscriptions, "GROUP") {
				w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s (Group)", instance.Id, eventType)
				w.sendToQueueOrWebhook(instance, queueName, jsonData)
			} else if strings.HasSuffix(infoChat, "@newsletter") && contains(subscriptions, "NEWSLETTER") {
				w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s (Newsletter)", instance.Id, eventType)
				w.sendToQueueOrWebhook(instance, queueName, jsonData)
			}
		}
	case "Receipt":
		if contains(subscriptions, "READ_RECEIPT") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		} else {
			if strings.HasSuffix(dataChat, "@g.us") && contains(subscriptions, "GROUP") {
				w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s (Group)", instance.Id, eventType)
				w.sendToQueueOrWebhook(instance, queueName, jsonData)
			} else if strings.HasSuffix(dataChat, "@newsletter") && contains(subscriptions, "NEWSLETTER") {
				w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s (Newsletter)", instance.Id, eventType)
				w.sendToQueueOrWebhook(instance, queueName, jsonData)
			}
		}
	case "Presence":
		if contains(subscriptions, "PRESENCE") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "HistorySync":
		if contains(subscriptions, "HISTORY_SYNC") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "ChatPresence", "Archive":
		if contains(subscriptions, "CHAT_PRESENCE") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "CallOffer", "CallAccept", "CallTerminate", "CallOfferNotice", "CallRelayLatency":
		if contains(subscriptions, "CALL") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "Connected", "PairSuccess", "TemporaryBan", "LoggedOut", "ConnectFailure", "Disconnected", "AccountReachoutTimelock", "PrivacySettings", "Blocklist":
		if contains(subscriptions, "CONNECTION") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "LabelEdit", "LabelAssociationChat", "LabelAssociationMessage":
		if contains(subscriptions, "LABEL") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "Contact", "PushName":
		if contains(subscriptions, "CONTACT") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "Picture":
		if contains(subscriptions, "PICTURE") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "UserAbout":
		if contains(subscriptions, "USER_ABOUT") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "GroupInfo", "JoinedGroup":
		if contains(subscriptions, "GROUP") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "NewsletterJoin", "NewsletterLeave", "NewsletterLiveUpdate", "NewsletterMuteChange":
		if contains(subscriptions, "NEWSLETTER") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "QRCode", "QRTimeout", "QRSuccess":
		if contains(subscriptions, "QRCODE") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "PasskeyRequest", "PasskeyConfirmation", "PasskeyError", "PairError":
		// Passkey (WebAuthn) events are part of the #wapk pairing flow, and
		// PairError is a pairing failure, so they are delivered to dedicated
		// PASSKEY subscribers and to QRCODE subscribers. Previously they hit
		// `default: return` and were silently dropped, even though myEventHandler
		// logged "DISPATCHING WEBHOOK" before this filter ran (issue #105).
		if shouldForwardPasskey(subscriptions) {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}
	case "ButtonClick":
		if contains(subscriptions, "BUTTON_CLICK") || contains(subscriptions, "MESSAGE") {
			w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Event received of type %s", instance.Id, eventType)
			w.sendToQueueOrWebhook(instance, queueName, jsonData)
		}

	default:
		return
	}
}

func contains(subscriptions []string, event string) bool {
	for _, sub := range subscriptions {
		if strings.EqualFold(sub, event) {
			return true
		}
	}
	return false
}

// shouldForwardPasskey reports whether a Passkey* event should be delivered for
// the given subscription list. Passkey (WebAuthn) events are part of the #wapk
// pairing flow, so they go to dedicated PASSKEY subscribers and to QRCODE
// subscribers (issue #105).
func shouldForwardPasskey(subscriptions []string) bool {
	return contains(subscriptions, event_types.PASSKEY) || contains(subscriptions, event_types.QRCODE)
}

// SetTypebotService wires the Typebot processor. Called once at boot, before any
// client starts, so no lock is needed: the write happens before any goroutine
// that reads the field exists.
func (w *whatsmeowService) SetTypebotService(processor TypebotProcessor) {
	w.typebotProcessor = processor
}

// DispatchToTypebot forwards a received message to the Typebot in its own
// goroutine. Without this, the HTTP call (up to 30s) would hold the whatsmeow
// event dispatch goroutine and delay everything else for that instance.
func (w whatsmeowService) DispatchToTypebot(instance *instance_model.Instance, remoteJid, pushName, content string, fromMe bool) {
	if w.typebotProcessor == nil || instance == nil {
		return
	}
	go w.typebotProcessor.ProcessMessage(instance, remoteJid, pushName, content, fromMe)
}

// SendOperationalEvent publishes an operational event (today only the Typebot
// auto-pause) bypassing CallWebhook's subscription filter, so the alert always
// reaches the configured queue/webhook.
func (w *whatsmeowService) SendOperationalEvent(instance *instance_model.Instance, event string, data map[string]any) {
	if instance == nil {
		return
	}

	payload := map[string]any{
		"event":         event,
		"instanceId":    instance.Id,
		"instanceName":  instance.Name,
		"instanceToken": instance.Token,
		"data":          data,
	}

	values, err := json.Marshal(payload)
	if err != nil {
		w.loggerWrapper.GetLogger(instance.Id).LogError("[%s] erro ao serializar evento operacional %s: %v", instance.Id, event, err)
		return
	}

	queueName := strings.ToLower(fmt.Sprintf("%s.%s", instance.Id, event))
	go w.sendToQueueOrWebhook(instance, queueName, values)
}

func (w *whatsmeowService) sendToQueueOrWebhook(instance *instance_model.Instance, queueName string, jsonData []byte) {
	if instance.RabbitmqEnable == "enabled" || instance.RabbitmqEnable == "true" {
		err := w.rabbitmqProducer.Produce(queueName, jsonData, instance.RabbitmqEnable, instance.Id)
		if err != nil {
			w.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to send message to rabbitmq: %s", instance.Id, err)
			return
		}
		w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Message sent to rabbitmq successfully", instance.Id)
	}

	if instance.NatsEnable == "enabled" || instance.NatsEnable == "true" {
		err := w.natsProducer.Produce(queueName, jsonData, instance.NatsEnable, instance.Id)
		if err != nil {
			w.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to send message to nats: %s", instance.Id, err)
			return
		}
		w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Message sent to nats successfully", instance.Id)
	}

	if instance.WebSocketEnable == "enabled" || instance.WebSocketEnable == "true" {
		err := w.websocketProducer.Produce(queueName, jsonData, instance.Id, instance.Token)
		if err != nil {
			w.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to send message to websocket: %s", instance.Id, err)
			return
		}
		w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Message sent to websocket successfully", instance.Id)
	}

	if instance.Webhook != "" && instance.Webhook != "disabled" {
		err := w.webhookProducer.Produce(queueName, jsonData, instance.Webhook, instance.Id)
		if err != nil {
			w.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to send message to webhook: %s", instance.Id, err)
			return
		}
		w.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Message sent to webhook successfully", instance.Id)
	}
}

func (w whatsmeowService) StartInstance(instanceId string) error {
	instance, err := w.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		return err
	}

	if instance.Proxy == "" && w.config.ProxyHost != "" && w.config.ProxyPort != "" && w.config.ProxyUsername != "" && w.config.ProxyPassword != "" {
		proxyConfig := ProxyConfig{
			Protocol: utils.NormalizeProxyProtocol(w.config.ProxyProtocol, w.config.ProxyPort),
			Host:     w.config.ProxyHost,
			Port:     w.config.ProxyPort,
			Username: w.config.ProxyUsername,
			Password: w.config.ProxyPassword,
		}

		proxyJSON, err := json.Marshal(proxyConfig)
		if err != nil {
			w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to marshal proxy config: %v", instanceId, err)
			return err
		}

		instance.Proxy = string(proxyJSON)

		err = w.instanceRepository.UpdateProxy(instance.Id, instance.Proxy)
		if err != nil {
			w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to update instance: %s", instanceId, err)
			return err
		}
	}

	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Starting client", instance.Id)

	v := Values{map[string]string{
		"Id":     instance.Id,
		"Jid":    instance.Jid,
		"Token":  instance.Token,
		"Events": instance.Events,
		"osName": instance.OsName,
		"Proxy":  instance.Proxy,
	}}

	w.userInfoCache.Set(instance.Token, v, cache.NoExpiration)

	eventArray := strings.Split(instance.Events, ",")

	var subscribedEvents []string

	if len(eventArray) < 1 {
		subscribedEvents = append(subscribedEvents, event_types.MESSAGE)
	} else {
		for _, arg := range eventArray {
			if !event_types.IsEventType(arg) {
				w.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Message type discarded: %s", instanceId, arg)
				continue
			}
			if !utils.Find(subscribedEvents, arg) {
				subscribedEvents = append(subscribedEvents, arg)
			}

		}
	}

	w.killChannel.Set(instance.Id, make(chan bool))

	clientData := &ClientData{
		Instance:      instance,
		Subscriptions: subscribedEvents,
		Phone:         "",
		IsProxy:       false,
	}

	if instance.Proxy != "" {
		var proxyConfig ProxyConfig
		err := json.Unmarshal([]byte(instance.Proxy), &proxyConfig)
		if err != nil {
			w.loggerWrapper.GetLogger(instanceId).LogError("[%s] error unmarshalling proxy config", instanceId)
			return err
		}

		if proxyConfig.Host != "" {
			clientData.IsProxy = true
		}
	}

	go w.StartClient(clientData)

	return nil
}

func (w whatsmeowService) ConnectOnStartup(clientName string) {
	w.loggerWrapper.GetLogger(clientName).LogInfo("Connecting all instances on startup")
	var instances []*instance_model.Instance
	var err error

	// Restore every PAIRED instance (jid set), not just those flagged Connected:
	// after any restart the live-socket column is false for all of them, so
	// "connected only" would leave paired sessions offline until a manual
	// reconnect. StartClient skips instances with no session in the auth store.
	if clientName != "" {
		instances, err = w.instanceRepository.GetAllPairedInstancesByClientName(clientName)
		if err != nil {
			w.loggerWrapper.GetLogger(clientName).LogError("[%s] Error getting all paired instances: %s", clientName, err)
			return
		}
	} else {
		instances, err = w.instanceRepository.GetAllPairedInstances()
		if err != nil {
			w.loggerWrapper.GetLogger(clientName).LogError("[%s] Error getting all paired instances: %s", clientName, err)
			return
		}
	}

	w.loggerWrapper.GetLogger(clientName).LogInfo("[%s] Found %d paired instances", clientName, len(instances))

	for _, instance := range instances {
		w.loggerWrapper.GetLogger(clientName).LogInfo("[%s] Starting client for user '%s'", clientName, instance.Id)

		err := w.StartInstance(instance.Id)
		if err != nil {
			w.loggerWrapper.GetLogger(clientName).LogError("[%s] Error starting client: %s", clientName, err)
		}
	}
}

func getExtensionFromMimeType(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "application/pdf":
		return ".pdf"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ".xlsx"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return ".pptx"
	default:
		// Se não encontrar um tipo conhecido, extrai a extensão do mimetype
		parts := strings.Split(mimeType, "/")
		if len(parts) > 1 {
			return "." + parts[1]
		}
		return ".bin"
	}
}

// globalEventTypeFor maps a whatsmeow event name to the coarse "global event"
// group used by AMQP_GLOBAL_EVENTS and NATS_GLOBAL_EVENTS. Keeping the mapping
// in one place fixes a divergence where the AMQP and NATS switches recognised
// different sets of events, so PICTURE / USER_ABOUT / BUTTON_CLICK configured
// for NATS were silently never published.
func globalEventTypeFor(eventType string) (string, bool) {
	switch eventType {
	case "Message":
		return "MESSAGE", true
	case "SendMessage":
		return "SEND_MESSAGE", true
	case "Receipt":
		return "READ_RECEIPT", true
	case "Presence":
		return "PRESENCE", true
	case "HistorySync":
		return "HISTORY_SYNC", true
	case "ChatPresence", "Archive":
		return "CHAT_PRESENCE", true
	case "CallOffer", "CallAccept", "CallTerminate", "CallOfferNotice", "CallRelayLatency":
		return "CALL", true
	case "Connected", "PairSuccess", "TemporaryBan", "LoggedOut", "ConnectFailure", "Disconnected":
		return "CONNECTION", true
	case "LabelEdit", "LabelAssociationChat", "LabelAssociationMessage":
		return "LABEL", true
	case "Contact", "PushName":
		return "CONTACT", true
	case "Picture":
		return "PICTURE", true
	case "UserAbout":
		return "USER_ABOUT", true
	case "ButtonClick":
		return "BUTTON_CLICK", true
	case "GroupInfo", "JoinedGroup":
		return "GROUP", true
	case "NewsletterJoin", "NewsletterLeave":
		return "NEWSLETTER", true
	case "QRCode", "QRTimeout", "QRSuccess":
		return "QRCODE", true
	default:
		return "", false
	}
}

func (w *whatsmeowService) SendToGlobalQueues(eventType string, payload []byte, userId string) {
	w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Starting sendToGlobalQueues for event: %s", userId, eventType)

	globalEventType, mapped := globalEventTypeFor(eventType)

	// AMQP: AMQP_SPECIFIC_EVENTS tem prioridade sobre AMQP_GLOBAL_EVENTS
	if w.config.AmqpGlobalEnabled {
		shouldSendToAmqp := false
		amqpQueueName := strings.ToLower(eventType)

		if len(w.config.AmqpSpecificEvents) > 0 {
			// A lista específica casa com o nome do evento cru.
			if utils.Find(w.config.AmqpSpecificEvents, eventType) {
				shouldSendToAmqp = true
			}
		} else if mapped && utils.Find(w.config.AmqpGlobalEvents, globalEventType) {
			shouldSendToAmqp = true
		}

		if shouldSendToAmqp {
			w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Sending to AMQP queue: %s", userId, amqpQueueName)
			if err := w.rabbitmqProducer.Produce(amqpQueueName, payload, "global", userId); err != nil {
				w.loggerWrapper.GetLogger(userId).LogError("[%s] Failed to send message to RabbitMQ global queue %s: %v", userId, amqpQueueName, err)
			} else {
				w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Successfully sent message to RabbitMQ global queue %s", userId, amqpQueueName)
			}
		} else {
			w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Event %s not configured for AMQP", userId, eventType)
		}
	}

	// NATS (NATS_GLOBAL_EVENTS)
	if w.config.NatsGlobalEnabled {
		if mapped && utils.Find(w.config.NatsGlobalEvents, globalEventType) {
			queueName := strings.ToLower(eventType)
			w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Sending to NATS subject: %s", userId, queueName)
			if err := w.natsProducer.Produce(queueName, payload, "global", userId); err != nil {
				w.loggerWrapper.GetLogger(userId).LogError("[%s] Failed to send message to NATS global subject %s: %v", userId, queueName, err)
			} else {
				w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Successfully sent message to NATS global subject %s", userId, queueName)
			}
		} else {
			w.loggerWrapper.GetLogger(userId).LogInfo("[%s] Event %s (group %q) not configured for NATS", userId, eventType, globalEventType)
		}
	}
}

var (
	cachedWebVersion   *clientVersion
	cachedWebVersionAt time.Time
	cachedWebVersionMu sync.Mutex

	// webVersionFetchMu serialises the actual fetch. It is distinct from the
	// cache lock so the HTTP request (and its 10s timeout) is never performed
	// while holding cachedWebVersionMu, which would block every reader.
	webVersionFetchMu  sync.Mutex
	webVersionCacheTTL = 1 * time.Hour
)

// whatsAppWebVersionClient bounds the WhatsApp Web version lookup.
var whatsAppWebVersionClient = &http.Client{Timeout: 10 * time.Second}

// whatsAppWebVersionURL is a var so tests can point the lookup at a local server.
var whatsAppWebVersionURL = "https://web.whatsapp.com/sw.js"

// webVersionRevisionPatterns are compiled once; they used to be recompiled on
// every fetch (and inside a loop).
var webVersionRevisionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`"client_revision":\s*(\d+)`),              // Formato direto
	regexp.MustCompile(`\\"client_revision\\":\s*(\d+)`),          // Formato escaped
	regexp.MustCompile(`client_revision\\?\\"?:[\s]*(\d+)`),       // Formato mais flexível
	regexp.MustCompile(`["']client_revision["'][\s]*:[\s]*(\d+)`), // Com aspas variadas
}

// getCachedWebVersion returns the cached version when it is still fresh.
func getCachedWebVersion() *clientVersion {
	cachedWebVersionMu.Lock()
	defer cachedWebVersionMu.Unlock()
	if cachedWebVersion != nil && time.Since(cachedWebVersionAt) < webVersionCacheTTL {
		return cachedWebVersion
	}
	return nil
}

func fetchWhatsAppWebVersion() (*clientVersion, error) {
	if v := getCachedWebVersion(); v != nil {
		return v, nil
	}

	// One goroutine performs the request while the others wait; the cache lock
	// is only taken to read/write the value, never across the HTTP call.
	webVersionFetchMu.Lock()
	defer webVersionFetchMu.Unlock()

	// Re-check: another goroutine may have populated the cache while we waited.
	if v := getCachedWebVersion(); v != nil {
		return v, nil
	}

	version, err := fetchWhatsAppWebVersionUncached()
	if err != nil {
		return nil, err
	}

	cachedWebVersionMu.Lock()
	cachedWebVersion = version
	cachedWebVersionAt = time.Now()
	cachedWebVersionMu.Unlock()

	return version, nil
}

// fetchWhatsAppWebVersionUncached performs the actual HTTP request and parse. It
// must be called with webVersionFetchMu held.
func fetchWhatsAppWebVersionUncached() (*clientVersion, error) {
	// Bounded client: this runs inside StartClient (startup and every reconnect),
	// so a hung http.Get would block bringing instances online.
	resp, err := whatsAppWebVersionClient.Get(whatsAppWebVersionURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch WhatsApp Web version: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	content := string(body)

	for _, re := range webVersionRevisionPatterns {
		matches := re.FindStringSubmatch(content)
		if len(matches) < 2 {
			continue
		}

		clientRevision, err := strconv.Atoi(matches[1])
		if err != nil || clientRevision <= 0 {
			continue
		}

		return &clientVersion{
			Major: 2,
			Minor: 3000,
			Patch: clientRevision,
		}, nil
	}

	// Se chegou aqui, nenhum padrão funcionou - log do conteúdo para debug
	// Mostra apenas uma parte para não logar muito
	previewLength := 500
	if len(content) > previewLength {
		content = content[:previewLength] + "..."
	}

	return nil, fmt.Errorf("could not find client revision in the fetched content. Content preview: %s", content)
}

func (w whatsmeowService) UpdateInstanceSettings(instanceId string) error {
	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Updating instance settings in runtime", instanceId)

	// Busca a instância atualizada do banco
	instance, err := w.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error getting instance from DB: %v", instanceId, err)
		return err
	}

	// Verifica se o MyClient existe
	myClient, exists := w.myClientPointer.Lookup(instanceId)
	if !exists {
		w.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] MyClient not found in runtime, instance may not be connected", instanceId)
		return fmt.Errorf("instance %s not found in runtime", instanceId)
	}

	// Atualiza as configurações no MyClient em execução
	myClient.Instance = instance
	myClient.webhookUrl = instance.Webhook
	myClient.rabbitmqEnable = instance.RabbitmqEnable
	myClient.natsEnable = instance.NatsEnable
	myClient.websocketEnable = instance.WebSocketEnable

	// Atualiza as subscriptions se os eventos mudaram
	eventArray := strings.Split(instance.Events, ",")
	var subscribedEvents []string

	if len(eventArray) < 1 {
		subscribedEvents = append(subscribedEvents, event_types.MESSAGE)
	} else {
		for _, arg := range eventArray {
			if !event_types.IsEventType(arg) {
				w.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Message type discarded: %s", instanceId, arg)
				continue
			}
			if !utils.Find(subscribedEvents, arg) {
				subscribedEvents = append(subscribedEvents, arg)
			}
		}
	}

	myClient.subscriptions = subscribedEvents

	// Atualiza o cache do userInfo com as novas configurações
	v := Values{map[string]string{
		"Id":     instance.Id,
		"Jid":    instance.Jid,
		"Token":  instance.Token,
		"Events": instance.Events,
		"osName": instance.OsName,
		"Proxy":  instance.Proxy,
	}}
	w.userInfoCache.Set(instance.Token, v, cache.NoExpiration)

	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Instance settings and cache updated in runtime successfully", instanceId)
	return nil
}

func (w whatsmeowService) UpdateInstanceAdvancedSettings(instanceId string) error {
	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Updating advanced settings in runtime", instanceId)

	// Busca a instância atualizada do banco
	instance, err := w.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		w.loggerWrapper.GetLogger(instanceId).LogError("[%s] Error getting instance from DB: %v", instanceId, err)
		return err
	}

	// Verifica se o MyClient existe
	myClient, exists := w.myClientPointer.Lookup(instanceId)
	if !exists {
		w.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] MyClient not found in runtime, instance may not be connected", instanceId)
		return fmt.Errorf("instance %s not found in runtime", instanceId)
	}

	// Atualiza a instância no MyClient com as advanced settings atualizadas
	previousAlwaysOnline := myClient.Instance != nil && myClient.Instance.AlwaysOnline
	myClient.Instance = instance

	// Turning alwaysOnline off must take effect immediately: otherwise the device
	// stays "available" until the periodic presence loop next ticks (up to hours
	// away) and the operator's phone keeps missing notifications. Turning it on
	// is picked up on the next connect (the presence loop is started there).
	// Issues #70/#54/#55.
	if previousAlwaysOnline && !instance.AlwaysOnline {
		if client := w.clientPointer.Get(instanceId); client != nil && client.IsConnected() {
			if perr := client.SendPresence(context.Background(), types.PresenceUnavailable); perr != nil {
				w.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Failed to mark self unavailable after alwaysOnline was turned off (non-fatal): %v", instanceId, perr)
			} else {
				w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Marked self as unavailable (alwaysOnline turned off)", instanceId)
			}
		}
	}

	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Advanced settings updated in runtime successfully", instanceId)
	return nil
}

func (w whatsmeowService) ClearInstanceCache(instanceId string, token string) error {
	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Clearing instance cache - Token: %s", instanceId, token)

	// Limpar userInfoCache
	w.userInfoCache.Delete(token)

	// Limpar myClientPointer se existir
	if _, exists := w.myClientPointer.Lookup(instanceId); exists {
		w.myClientPointer.Delete(instanceId)
		w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] MyClient pointer cleared", instanceId)
	}

	// Limpar clientPointer se existir
	if _, exists := w.clientPointer.Lookup(instanceId); exists {
		w.clientPointer.Delete(instanceId)
		w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Client pointer cleared", instanceId)
	}

	// Limpar killChannel se existir
	if killChan, exists := w.killChannel.Lookup(instanceId); exists {
		select {
		case killChan <- true:
			// Canal recebeu o sinal
		default:
			// Canal pode estar bloqueado, apenas fecha
		}
		close(killChan)
		w.killChannel.Delete(instanceId)
		w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Kill channel cleared", instanceId)
	}

	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Instance cache completely cleared", instanceId)
	return nil
}

func NewWhatsmeowService(
	instanceRepository instance_repository.InstanceRepository,
	authDB *sql.DB,
	messageRepository message_repository.MessageRepository,
	labelRepository label_repository.LabelRepository,
	config *config.Config,
	killChannel *safemap.Map[chan bool],
	clientPointer *safemap.Map[*whatsmeow.Client],
	rabbitmqProducer producer_interfaces.Producer,
	webhookProducer producer_interfaces.Producer,
	websocketProducer producer_interfaces.Producer,
	sqliteDB *sql.DB,
	exPath string,
	mediaStorage storage_interfaces.MediaStorage,
	natsProducer producer_interfaces.Producer,
	loggerWrapper *logger_wrapper.LoggerManager,
) WhatsmeowService {
	// Inicializar PollService de forma segura
	pollSvc := poll_service.NewPollService(authDB, loggerWrapper)

	return &whatsmeowService{
		instanceRepository: instanceRepository,
		authDB:             authDB,
		messageRepository:  messageRepository,
		labelRepository:    labelRepository,
		pollService:        pollSvc, // NOVO: Serviço de enquetes
		config:             config,
		killChannel:        killChannel,
		userInfoCache:      cache.New(5*time.Minute, 10*time.Minute),
		chatNameCache:      cache.New(10*time.Minute, 15*time.Minute),
		groupInfoCache:     cache.New(5*time.Minute, 10*time.Minute),
		clientPointer:      clientPointer,
		myClientPointer:    safemap.New[*MyClient](),
		rabbitmqProducer:   rabbitmqProducer,
		webhookProducer:    webhookProducer,
		websocketProducer:  websocketProducer,
		sqliteDB:           sqliteDB,
		exPath:             exPath,
		mediaStorage:       mediaStorage,
		processedMessages:  cache.New(30*time.Minute, 1*time.Hour),
		natsProducer:       natsProducer,
		loggerWrapper:      loggerWrapper,
		passkeyCeremony:    ceremony.NewStore(),
		mediaRetryPending:  cache.New(10*time.Minute, 15*time.Minute),
		mediaRetryBytes:    cache.New(30*time.Minute, time.Hour),
		authStore:          &sharedSQLStore{},
		persistPool:        newPersistPool(persistWorkers, persistQueueSize),
	}
}

// GetPollService retorna o serviço de polls (evita dupla inicialização)
func (w *whatsmeowService) GetPollService() poll_service.PollService {
	return w.pollService
}

// mediaRetryEntry is what we keep so the (asynchronous) media-retry response can
// be decrypted and re-downloaded.
type mediaRetryEntry struct {
	mediaKey []byte
	media    whatsmeow.DownloadableMessage
}

func mediaRetryKey(instanceId, messageID string) string {
	return instanceId + "|" + messageID
}

// RequestMediaRetry asks the sender's phone to re-upload media whose download
// failed (whatsmeow returns ErrMediaDownloadFailedWith403/404/410 for expired
// direct paths). The response arrives later as *events.MediaRetry; the refreshed
// bytes are then served by GetRetriedMedia. See whatsmeow's mediaretry.go.
func (w *whatsmeowService) RequestMediaRetry(instanceId string, info *types.MessageInfo, mediaKey []byte, media whatsmeow.DownloadableMessage) error {
	if info == nil || info.ID == "" || len(mediaKey) == 0 || media == nil {
		return fmt.Errorf("media retry requires the message info, media key and media")
	}
	client, ok := w.clientPointer.Lookup(instanceId)
	if !ok || client == nil {
		return fmt.Errorf("no active client for instance %s", instanceId)
	}
	if err := client.SendMediaRetryReceipt(context.Background(), info, mediaKey); err != nil {
		return err
	}
	w.mediaRetryPending.Set(mediaRetryKey(instanceId, info.ID), mediaRetryEntry{mediaKey: mediaKey, media: media}, cache.DefaultExpiration)
	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Requested media retry for message %s", instanceId, info.ID)
	return nil
}

// GetRetriedMedia returns the bytes re-downloaded after a successful media retry.
func (w *whatsmeowService) GetRetriedMedia(instanceId, messageID string) ([]byte, bool) {
	if v, ok := w.mediaRetryBytes.Get(mediaRetryKey(instanceId, messageID)); ok {
		if b, ok := v.([]byte); ok {
			return b, true
		}
	}
	return nil, false
}

// HandleMediaRetry decrypts a media-retry response, re-downloads the media from
// the refreshed direct path and caches the bytes.
func (w *whatsmeowService) HandleMediaRetry(instanceId string, evt *events.MediaRetry) {
	if evt == nil {
		return
	}
	key := mediaRetryKey(instanceId, string(evt.MessageID))
	v, ok := w.mediaRetryPending.Get(key)
	if !ok {
		return
	}
	entry := v.(mediaRetryEntry)
	logger := w.loggerWrapper.GetLogger(instanceId)

	notif, err := whatsmeow.DecryptMediaRetryNotification(evt, entry.mediaKey)
	if err != nil {
		logger.LogWarn("[%s] media retry response for %s could not be decrypted: %v", instanceId, evt.MessageID, err)
		return
	}
	if notif.GetResult() != waMmsRetry.MediaRetryNotification_SUCCESS {
		logger.LogWarn("[%s] media retry for %s was refused (result %v)", instanceId, evt.MessageID, notif.GetResult())
		return
	}

	setMediaDirectPath(entry.media, notif.GetDirectPath())

	client, ok := w.clientPointer.Lookup(instanceId)
	if !ok || client == nil {
		return
	}
	data, err := client.Download(context.Background(), entry.media)
	if err != nil {
		logger.LogWarn("[%s] media re-download after retry failed for %s: %v", instanceId, evt.MessageID, err)
		return
	}
	w.mediaRetryBytes.Set(key, data, cache.DefaultExpiration)
	w.mediaRetryPending.Delete(key)
	logger.LogInfo("[%s] media retry succeeded for %s (%d bytes)", instanceId, evt.MessageID, len(data))
}

// setMediaDirectPath replaces the (expired) direct path on the cached media
// with the refreshed one from the retry response.
func setMediaDirectPath(media whatsmeow.DownloadableMessage, path string) {
	if path == "" {
		return
	}
	switch m := media.(type) {
	case *waE2E.ImageMessage:
		m.DirectPath = proto.String(path)
	case *waE2E.VideoMessage:
		m.DirectPath = proto.String(path)
	case *waE2E.AudioMessage:
		m.DirectPath = proto.String(path)
	case *waE2E.DocumentMessage:
		m.DirectPath = proto.String(path)
	case *waE2E.StickerMessage:
		m.DirectPath = proto.String(path)
	}
}

// DeleteInstanceDevice removes the instance's device from the whatsmeow store.
// Deleting an instance used to leave its whatsmeow_device row (and, through it,
// the sessions, identity keys, pre-keys and contacts that cascade from it)
// orphaned in the auth database forever. A never-paired instance has no JID, so
// this is a no-op for it.
func (w *whatsmeowService) DeleteInstanceDevice(instanceId, jid string) error {
	if jid == "" {
		return nil
	}
	parsed, ok := utils.ParseJID(jid)
	if !ok || parsed.IsEmpty() {
		return nil
	}

	container, err := w.getSharedSQLStoreContainer()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	device, err := container.GetDevice(ctx, parsed)
	if err != nil {
		return err
	}
	if device == nil {
		return nil
	}

	if err := device.Delete(ctx); err != nil {
		return err
	}
	w.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Deleted whatsmeow device store for %s", instanceId, parsed.String())
	return nil
}

// GetInstanceOverview returns the instance's own profile picture, push name and
// local contact count for the dashboard. Everything is best-effort: a missing
// client or a failed picture lookup just leaves the corresponding field empty,
// and the profile-picture IQ is time-bounded so the endpoint can never hang.
func (w *whatsmeowService) GetInstanceOverview(instanceId string) (*InstanceOverview, error) {
	client, ok := w.clientPointer.Lookup(instanceId)
	if !ok || client == nil {
		return &InstanceOverview{}, nil
	}

	overview := &InstanceOverview{Connected: client.IsConnected()}
	if client.Store != nil {
		// Platform (android/ios/...) and business name come from the persisted
		// device row, so they are available even while disconnected. This is the
		// phone that scanned the QR — whatsmeow has no model string.
		overview.Platform = client.Store.Platform
		overview.BusinessName = client.Store.BusinessName
	}
	if !overview.Connected || client.Store == nil {
		return overview, nil
	}

	overview.ProfileName = client.Store.PushName
	if client.Store.ID != nil {
		jid := client.Store.ID.ToNonAD()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if pic, err := client.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{Preview: true}); err == nil && pic != nil {
			overview.ProfilePicURL = pic.URL
		}
		cancel()
	}
	if contacts, err := client.Store.Contacts.GetAllContacts(context.Background()); err == nil {
		overview.ContactsCount = len(contacts)
	}
	return overview, nil
}

// ChatIdentity is the resolved identity of a bare message source: a display name
// and, when the source is (or maps back to) a phone number, that number. The
// phone lets the caller merge a conversation that was persisted once under a LID
// and once under the phone number into a single row.
type ChatIdentity struct {
	Name  string `json:"name,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// ResolveChats maps the bare user strings persisted in messages.source to a
// display name and, where known, the phone number behind them. It resolves, in
// order: the special status/broadcast sources, a saved contact, a LID mapped
// back to its phone number, and a group subject.
//
// /server/stats is global and the stored source carries no instance id, so each
// source is tried against every live client until one resolves it. Results are
// cached (see chatNameCache) because the dashboard polls /server/stats every ~15s
// and group lookups are network round-trips.
func (w *whatsmeowService) ResolveChats(users []string) map[string]ChatIdentity {
	out := make(map[string]ChatIdentity, len(users))
	if len(users) == 0 {
		return out
	}

	clients := make([]*whatsmeow.Client, 0, w.clientPointer.Len())
	for _, c := range w.clientPointer.Snapshot() {
		if c != nil && c.Store != nil && c.IsConnected() {
			clients = append(clients, c)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	for _, user := range users {
		if w.chatNameCache != nil {
			if id, ok := w.chatNameCache.Get(user); ok {
				out[user] = id.(ChatIdentity)
				continue
			}
		}
		id := resolveChatIdentity(ctx, clients, user)
		if w.chatNameCache != nil {
			w.chatNameCache.Set(user, id, cache.DefaultExpiration)
		}
		out[user] = id
	}
	return out
}

// resolveChatIdentity resolves a single bare source string. Order matters: a
// phone number that is a saved contact wins, then a LID mapped back to its phone
// number (contact or just the number), then a group subject. An unresolved source
// yields an empty identity and the caller falls back to the raw key.
func resolveChatIdentity(ctx context.Context, clients []*whatsmeow.Client, user string) ChatIdentity {
	if user == "" {
		return ChatIdentity{}
	}
	switch user {
	case "status", "0":
		return ChatIdentity{Name: "Status"}
	}
	if strings.Contains(user, "broadcast") {
		return ChatIdentity{Name: "Transmissão"}
	}

	var lidPhone string
	for _, cli := range clients {
		// 1) The source is a phone number.
		if name := contactDisplayName(ctx, cli, types.NewJID(user, types.DefaultUserServer)); name != "" {
			return ChatIdentity{Name: name, Phone: user}
		}
		// 2) The source is a LID: map it back to the phone number.
		if pn, err := cli.Store.LIDs.GetPNForLID(ctx, types.NewJID(user, types.HiddenUserServer)); err == nil && !pn.IsEmpty() {
			lidPhone = pn.User
			if name := contactDisplayName(ctx, cli, pn.ToNonAD()); name != "" {
				return ChatIdentity{Name: name, Phone: pn.User}
			}
		}
	}
	// A LID that mapped to a phone but has no saved contact: the phone alone is
	// enough to both label and merge it.
	if lidPhone != "" {
		return ChatIdentity{Phone: lidPhone}
	}

	// 3) The source is a group id (groups store the group's user part).
	for _, cli := range clients {
		gctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		info, err := cli.GetGroupInfo(gctx, types.NewJID(user, types.GroupServer))
		cancel()
		if err == nil && info != nil && info.Name != "" {
			return ChatIdentity{Name: info.Name}
		}
	}

	return ChatIdentity{}
}

// contactDisplayName returns the best available name for a contact, or "".
func contactDisplayName(ctx context.Context, cli *whatsmeow.Client, jid types.JID) string {
	c, err := cli.Store.Contacts.GetContact(ctx, jid)
	if err != nil {
		return ""
	}
	switch {
	case c.FullName != "":
		return c.FullName
	case c.BusinessName != "":
		return c.BusinessName
	case c.PushName != "":
		return c.PushName
	case c.FirstName != "":
		return c.FirstName
	}
	return ""
}

// PasskeyCeremonyStore exposes the shared ceremony store so the public HTTP
// polling endpoint can read the current stage for a given ceremony token.
func (w *whatsmeowService) PasskeyCeremonyStore() *ceremony.Store {
	return w.passkeyCeremony
}

// SubmitPasskeyResponse forwards the browser's WebAuthn assertion to WhatsApp
// for the given instance. Called by POST /passkey-ceremony/{token}/response.
func (w *whatsmeowService) SubmitPasskeyResponse(instanceId string, resp *types.WebAuthnResponse) error {
	client, ok := w.clientPointer.Lookup(instanceId)
	if !ok || client == nil {
		return fmt.Errorf("no active client for instance %s", instanceId)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := client.SendPasskeyResponse(ctx, resp); err != nil {
		w.passkeyCeremony.SetError(instanceId, err.Error())
		return err
	}
	// Server will asynchronously emit PairPasskeyConfirmation (or Error) into
	// the event handler; move to the waiting stage in the meantime.
	w.passkeyCeremony.SetAwaitingConfirmation(instanceId)
	return nil
}

// ConfirmPasskey finishes the pairing after the user verified the code.
// Called by POST /passkey-ceremony/{token}/confirm.
func (w *whatsmeowService) ConfirmPasskey(instanceId string) error {
	client, ok := w.clientPointer.Lookup(instanceId)
	if !ok || client == nil {
		return fmt.Errorf("no active client for instance %s", instanceId)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := client.SendPasskeyConfirmation(ctx); err != nil {
		w.passkeyCeremony.SetError(instanceId, err.Error())
		return err
	}
	w.passkeyCeremony.SetConfirmed(instanceId)
	return nil
}

// cleanSenderID remove a parte ":numero" do sender ID para exibir apenas o remoteJid correto
// Exemplo: "557499879409:3@s.whatsapp.net" -> "557499879409@s.whatsapp.net"
func cleanSenderID(senderID string) string {
	// Procura pelo padrão ":numero" antes do @
	if colonIndex := strings.Index(senderID, ":"); colonIndex != -1 {
		if atIndex := strings.Index(senderID, "@"); atIndex != -1 && colonIndex < atIndex {
			// Remove a parte ":numero" mantendo apenas o número principal e o domínio
			return senderID[:colonIndex] + senderID[atIndex:]
		}
	}
	return senderID
}
