package whatsmeow_service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"math/rand"
	"strings"
	"time"

	"golang.org/x/image/webp"

	"github.com/gomessguii/logger"
	_ "github.com/lib/pq"
	"github.com/patrickmn/go-cache"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/EvolutionAPI/evolution-go/pkg/config"
	producer_interfaces "github.com/EvolutionAPI/evolution-go/pkg/events/interfaces"
	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	instance_repository "github.com/EvolutionAPI/evolution-go/pkg/instance/repository"
	"github.com/EvolutionAPI/evolution-go/pkg/internal/event_types"
	message_model "github.com/EvolutionAPI/evolution-go/pkg/message/model"
	message_repository "github.com/EvolutionAPI/evolution-go/pkg/message/repository"
	storage_interfaces "github.com/EvolutionAPI/evolution-go/pkg/storage/interfaces"
	"github.com/EvolutionAPI/evolution-go/pkg/utils"
)

type WhatsmeowService interface {
	StartClient(clientData *ClientData)
	ConnectOnStartup(clientName string)
}

type whatsmeowService struct {
	instanceRepository      instance_repository.InstanceRepository
	messageRepository       message_repository.MessageRepository
	config                  *config.Config
	killChannel             map[string](chan bool)
	userInfoCache           *cache.Cache
	clientPointer           map[string]*whatsmeow.Client
	linkingCodeEventChannel chan LinkingCodeEvent
	rabbitmqProducer        producer_interfaces.Producer
	webhookProducer         producer_interfaces.Producer
	websocketProducer       producer_interfaces.Producer
	sqliteDB                *sql.DB
	exPath                  string
	mediaStorage            storage_interfaces.MediaStorage
}

type MyClient struct {
	WAClient           *whatsmeow.Client
	eventHandlerID     uint32
	userID             string
	Instance           *instance_model.Instance
	token              string
	subscriptions      []string
	webhookUrl         string
	rabbitmqEnable     string
	websocketEnable    string
	instanceRepository instance_repository.InstanceRepository
	messageRepository  message_repository.MessageRepository
	clientPointer      map[string]*whatsmeow.Client
	killChannel        map[string](chan bool)
	userInfoCache      *cache.Cache
	config             *config.Config
	historySyncID      int32
	rabbitmqProducer   producer_interfaces.Producer
	webhookProducer    producer_interfaces.Producer
	websocketProducer  producer_interfaces.Producer
	mediaStorage       storage_interfaces.MediaStorage
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
	Host     string `json:"host"`
	Password string `json:"password"`
	Port     string `json:"port"`
	Username string `json:"username"`
}

type LinkingCodeEvent struct {
	LinkingCode string
	Token       string
}

func (w whatsmeowService) StartClient(cd *ClientData) {

	logger.LogInfo("Starting websocket connection to Whatsapp for user '%s'", cd.Instance.Id)

	var deviceStore *store.Device
	var err error

	if w.clientPointer[cd.Instance.Id] != nil {
		if w.clientPointer[cd.Instance.Id].IsConnected() {
			return
		}
	}

	var container *sqlstore.Container

	if w.config.WaDebug != "" {
		dbLog := waLog.Stdout("Database", w.config.WaDebug, true)
		if w.config.PostgresAuthDB != "" {
			container, err = sqlstore.New("postgres", w.config.PostgresAuthDB, dbLog)
		} else {
			dsn := fmt.Sprintf("file:%s/dbdata/main.db?_pragma=foreign_keys(1)&_busy_timeout=5000&cache=shared&mode=rwc&_journal_mode=WAL", w.exPath)
			container, err = sqlstore.New("sqlite", dsn, dbLog)
		}
	} else {
		if w.config.PostgresAuthDB != "" {
			container, err = sqlstore.New("postgres", w.config.PostgresAuthDB, nil)
		} else {
			dsn := fmt.Sprintf("file:%s/dbdata/main.db?_pragma=foreign_keys(1)&_busy_timeout=5000&cache=shared&mode=rwc&_journal_mode=WAL", w.exPath)
			container, err = sqlstore.New("sqlite", dsn, nil)
		}
	}

	if err != nil {
		logger.LogError("[%s] Failed to create container: %v", cd.Instance.Id, err)
		return
	}

	if cd.Instance.Jid != "" {
		jid, _ := utils.ParseJID(cd.Instance.Jid)
		logger.LogInfo("[%s] Jid found. Getting device store for jid: %s", cd.Instance.Id, jid)
		deviceStore, err = container.GetDevice(jid)
		if err != nil {
			panic(err)
		}
	} else {
		logger.LogWarn("[%s] No jid found. Creating new device", cd.Instance.Id)
		deviceStore = container.NewDevice()
	}

	if deviceStore == nil {
		logger.LogWarn("[%s] No store found. Creating new one", cd.Instance.Id)
		deviceStore = container.NewDevice()

		cd.Instance.Connected = false
		err := w.instanceRepository.Update(cd.Instance)
		if err != nil {
			logger.LogError("[%s] Error updating instance: %s", cd.Instance.Id, err)
		}
	}

	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
	store.DeviceProps.Os = &cd.Instance.OsName

	clientLog := waLog.Stdout("Client", w.config.WaDebug, true)
	var client *whatsmeow.Client
	if w.config.WaDebug != "" {
		client = whatsmeow.NewClient(deviceStore, clientLog)
	} else {
		client = whatsmeow.NewClient(deviceStore, nil)
	}

	w.clientPointer[cd.Instance.Id] = client

	if cd.IsProxy {
		var proxyConfig ProxyConfig
		err := json.Unmarshal([]byte(cd.Instance.Proxy), &proxyConfig)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		proxy, err := utils.CreateSocks5Proxy(proxyConfig.Host, proxyConfig.Port, proxyConfig.Username, proxyConfig.Password)
		if err != nil {
			logger.LogError("[%s] Proxy error, disabling proxy", cd.Instance.Id)
		} else {
			client.SetProxy(proxy)
			logger.LogInfo("[%s] Proxy enabled", cd.Instance.Id)
		}
	}

	mycli := MyClient{
		Instance:           cd.Instance,
		WAClient:           client,
		eventHandlerID:     1,
		userID:             cd.Instance.Id,
		token:              cd.Instance.Token,
		subscriptions:      cd.Subscriptions,
		webhookUrl:         cd.Instance.Webhook,
		rabbitmqEnable:     cd.Instance.RabbitmqEnable,
		websocketEnable:    cd.Instance.WebSocketEnable,
		instanceRepository: w.instanceRepository,
		messageRepository:  w.messageRepository,
		userInfoCache:      w.userInfoCache,
		clientPointer:      w.clientPointer,
		killChannel:        w.killChannel,
		config:             w.config,
		historySyncID:      0,
		rabbitmqProducer:   w.rabbitmqProducer,
		webhookProducer:    w.webhookProducer,
		websocketProducer:  w.websocketProducer,
		mediaStorage:       w.mediaStorage,
	}

	mycli.eventHandlerID = mycli.WAClient.AddEventHandler(mycli.myEventHandler)

	if client.Store.ID != nil {
		logger.LogInfo("[%s] Already logged in with JID: %s", cd.Instance.Id, client.Store.ID.String())
		err = client.Connect()
		if err != nil {
			logger.LogError("[%s] Failed to connect: %s", cd.Instance.Id, err)
			return
		}
	} else {
		qrChan, err := client.GetQRChannel(context.Background())
		if err != nil {
			// This error means that we're already logged in, so ignore it.
			if !errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
				logger.LogError("[%s] Failed to get QR channel", cd.Instance.Id)
			}
		} else {
			if cd.Phone != "" {
				logger.LogInfo("[%s] Requesting pairing code", cd.Instance.Id)
				client.Connect()
				linkingCode, err := client.PairPhone(cd.Phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
				if err != nil {
					logger.LogError("[%s] something went wrong calling pair phone", cd.Instance.Id)
				}

				logger.LogInfo("[%s] Pairing code: %s", cd.Instance.Id, linkingCode)

				linkingCodeEvent := LinkingCodeEvent{
					LinkingCode: linkingCode,
					Token:       cd.Instance.Token,
				}

				w.linkingCodeEventChannel <- linkingCodeEvent
			} else {
				err = client.Connect()
				if err != nil {
					panic(err)
				}
			}
			for evt := range qrChan {
				logger.LogInfo("[%s] Received QR code event %s", cd.Instance.Id, evt.Event)
				if evt.Event == "code" {
					if w.config.LogType != "json" {
						fmt.Println("QR code:\n", evt.Code)
					}

					image, _ := qrcode.Encode(evt.Code, qrcode.Medium, 256)
					base64qrcode := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)

					base64WithCode := base64qrcode + "|" + evt.Code

					cd.Instance.Qrcode = base64WithCode

					err := w.instanceRepository.Update(cd.Instance)
					if err != nil {
						logger.LogError("[%s] Error updating instance: %s", cd.Instance.Id, err)
					}

					postMap := make(map[string]interface{})

					postMap["event"] = "QRCode"

					dataMap := make(map[string]interface{})

					dataMap["qrcode"] = base64qrcode
					dataMap["code"] = evt.Code

					postMap["data"] = dataMap

					postMap["instanceToken"] = mycli.token
					postMap["instanceId"] = mycli.userID

					var queueName string

					if _, ok := postMap["event"]; ok {
						queueName = strings.ToLower(fmt.Sprintf("%s.%s", cd.Instance.Id, postMap["event"]))
					}

					values, err := json.Marshal(postMap)
					if err != nil {
						logger.LogError("[%s] Failed to marshal JSON for queue", cd.Instance.Id)
						return
					}

					go mycli.callWebhook(queueName, values)
				} else if evt.Event == "timeout" {
					cd.Instance.Qrcode = ""

					err := w.instanceRepository.Update(cd.Instance)
					if err != nil {
						logger.LogError("[%s] Error updating instance: %s", cd.Instance.Id, err)
					}

					logger.LogWarn("[%s] QR timeout killing channel", cd.Instance.Id)
					delete(w.clientPointer, cd.Instance.Id)
					w.killChannel[cd.Instance.Id] <- true

					postMap := make(map[string]interface{})

					postMap["event"] = "QRTimeout"

					dataMap := make(map[string]interface{})

					postMap["data"] = dataMap

					postMap["instanceToken"] = mycli.token
					postMap["instanceId"] = mycli.userID

					var queueName string

					if _, ok := postMap["event"]; ok {
						queueName = strings.ToLower(fmt.Sprintf("%s.%s", cd.Instance.Id, postMap["event"]))
					}

					values, err := json.Marshal(postMap)
					if err != nil {
						logger.LogError("[%s] Failed to marshal JSON for queue", cd.Instance.Id)
						return
					}

					go mycli.callWebhook(queueName, values)
				} else if evt.Event == "success" {
					logger.LogInfo("[%s] QR pairing ok!", cd.Instance.Id)
				} else {
					logger.LogInfo("[%s] Login event: %s", cd.Instance.Id, evt.Event)
				}
			}
		}
	}

	for {
		select {
		case <-w.killChannel[cd.Instance.Id]:
			logger.LogInfo("Received kill signal for user '%s'", cd.Instance.Id)
			client.Disconnect()

			delete(w.clientPointer, cd.Instance.Id)

			cd.Instance.Connected = false

			err := w.instanceRepository.Update(cd.Instance)
			if err != nil {
				logger.LogError("[%s] Error updating instance: %s", cd.Instance.Id, err)
			}

			postMap := make(map[string]interface{})

			postMap["event"] = "LoggedOut"

			dataMap := make(map[string]interface{})

			dataMap["reason"] = "Logged out"

			postMap["data"] = dataMap

			postMap["instanceToken"] = mycli.token
			postMap["instanceId"] = mycli.userID

			var queueName string

			if _, ok := postMap["event"]; ok {
				queueName = strings.ToLower(fmt.Sprintf("%s.%s", cd.Instance.Id, postMap["event"]))
			}

			values, err := json.Marshal(postMap)
			if err != nil {
				logger.LogError("[%s] Failed to marshal JSON for queue", cd.Instance.Id)
				return
			}

			go mycli.callWebhook(queueName, values)

			// restart client
			logger.LogInfo("[%s] Restarting client", cd.Instance.Id)
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
			processPresenceUpdates(mycli)

			ticker.Stop()
			randomInterval := time.Duration(1+rand.Intn(3)) * time.Hour
			ticker = time.NewTicker(randomInterval)
		}
	}
}

func processPresenceUpdates(mycli *MyClient) {
	now := time.Now()
	location, _ := time.LoadLocation("America/Sao_Paulo")
	nowSp := now.In(location)

	if nowSp.Hour() >= 1 && nowSp.Hour() < 24 {
		err := mycli.WAClient.SendPresence(types.PresenceAvailable)
		if err != nil {
			logger.LogError("[%s] Failed to set presence as available %v", mycli.userID, err)
		} else {
			logger.LogInfo("[%s] Marked self as available", mycli.userID)
		}

		time.Sleep(time.Duration(7+rand.Intn(29)) * time.Second)

		err = mycli.WAClient.SendPresence(types.PresenceUnavailable)
		if err != nil {
			logger.LogError("[%s] Failed to set presence as unavailable %v", mycli.userID, err)
		} else {
			logger.LogInfo("[%s] Marked self as unavailable", mycli.userID)
		}
	}
}

func (mycli *MyClient) myEventHandler(rawEvt interface{}) {
	userID := mycli.userID
	postMap := make(map[string]interface{})
	postMap["data"] = rawEvt
	doWebhook := false

	switch evt := rawEvt.(type) {
	case *events.AppStateSyncComplete:
		if len(mycli.WAClient.Store.PushName) > 0 && evt.Name == appstate.WAPatchCriticalBlock {
			err := mycli.WAClient.SendPresence(types.PresenceUnavailable)
			if err != nil {
				logger.LogWarn("[%s] Failed to send unavailable presence %v", mycli.userID, err)
			} else {
				logger.LogWarn("[%s] Marked self as unavailable", mycli.userID)
			}
		}
	case *events.Connected, *events.PushNameSetting:
		logger.LogInfo("[%s] events.Connected to Whatsapp for user '%s'", mycli.userID, mycli.WAClient.Store.PushName)
		if len(mycli.WAClient.Store.PushName) > 0 {
			doWebhook = true
			postMap["event"] = "Connected"

			if postMap["data"] != nil {
				jsonBytes, err := json.Marshal(postMap["data"])
				if err != nil {
					logger.LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
					return
				}

				var dataMap map[string]interface{}
				err = json.Unmarshal(jsonBytes, &dataMap)
				if err != nil {
					logger.LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
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
			// 	profilePicUrl, err := mycli.clientPointer[mycli.userID].GetProfilePictureInfo(jid, &whatsmeow.GetProfilePictureParams{
			// 		Preview: false,
			// 	})
			// 	if err != nil {
			// 		logger.LogError("[%s] Failed to get profile picture info: %v", mycli.userID, err)
			// 	} else {
			// 		dataMap["profilePicUrl"] = profilePicUrl.URL
			// 	}
			// }

			postMap["data"] = dataMap

			go schedulePresenceUpdates(mycli)

			err := mycli.WAClient.SendPresence(types.PresenceUnavailable)
			if err != nil {
				logger.LogWarn("[%s] Failed to send unavailable presence %v", mycli.userID, err)
			} else {
				logger.LogWarn("[%s] Marked self as unavailable", mycli.userID)
			}

			mycli.Instance.Connected = true
			err = mycli.instanceRepository.Update(mycli.Instance)
			if err != nil {
				logger.LogError("[%s] Error updating instance: %s", mycli.Instance.Id, err)
			}
		}
	case *events.PairSuccess:
		doWebhook = true
		postMap["event"] = "PairSuccess"
		logger.LogInfo("QR Pair Success for user '%s' with JID '%s'", mycli.userID, evt.ID.String())

		instance, err := mycli.instanceRepository.GetInstanceByID(mycli.userID)
		if err != nil {
			logger.LogError("[%s] Error getting instance: %s", mycli.userID, err)
		}

		instance.Qrcode = ""
		instance.Connected = true
		instance.Jid = evt.ID.String()

		logger.LogInfo("[%s] Updating JID: %s in Instance: %s", mycli.userID, evt.ID.String(), instance.Jid)

		logger.LogInfo("[%s] Attempting to update instance in DB: %+v", mycli.userID, instance)
		err = mycli.instanceRepository.Update(instance)
		if err != nil {
			logger.LogError("[%s] Error updating instance: %s", mycli.userID, err)
		} else {
			logger.LogInfo("[%s] Instance successfully updated", mycli.userID)
		}

		myUserInfo, found := mycli.userInfoCache.Get(mycli.token)

		if !found {
			logger.LogWarn("[%s] No user info cached on pairing?", mycli.userID)
		} else {
			txtid := myUserInfo.(Values).Get("Id")
			token := myUserInfo.(Values).Get("Token")

			updatedUserInfo := utils.UpdateUserInfo(myUserInfo, "Jid", evt.ID.String())

			mycli.userInfoCache.Set(token, updatedUserInfo, cache.NoExpiration)
			logger.LogInfo("[%s] User information set for user '%s'", mycli.userID, txtid)
		}

		if postMap["data"] != nil {
			jsonBytes, err := json.Marshal(postMap["data"])
			if err != nil {
				logger.LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
				return
			}

			var dataMap map[string]interface{}
			err = json.Unmarshal(jsonBytes, &dataMap)
			if err != nil {
				logger.LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
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

		// jid, ok := utils.ParseJID(mycli.WAClient.Store.ID.ToNonAD().User)
		// if ok {
		// 	profilePicUrl, err := mycli.clientPointer[mycli.userID].GetProfilePictureInfo(jid, &whatsmeow.GetProfilePictureParams{
		// 		Preview: false,
		// 	})
		// 	if err != nil {
		// 		logger.LogError("[%s] Failed to get profile picture info: %v", mycli.userID, err)
		// 	} else {
		// 		dataMap["profilePicUrl"] = profilePicUrl.URL
		// 	}
		// }

		postMap["data"] = dataMap
	case *events.StreamReplaced:
		logger.LogInfo("[%s] Received StreamReplaced event", mycli.userID)
		return
	case *events.TemporaryBan:
		logger.LogInfo("[%s] User received temporary ban for %s", mycli.userID, evt.Code.String())
		doWebhook = true
		postMap["event"] = "TemporaryBan"

		if postMap["data"] != nil {
			jsonBytes, err := json.Marshal(postMap["data"])
			if err != nil {
				logger.LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
				return
			}

			var dataMap map[string]interface{}
			err = json.Unmarshal(jsonBytes, &dataMap)
			if err != nil {
				logger.LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
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

		if postMap["data"] != nil {
			jsonBytes, err := json.Marshal(postMap["data"])
			if err != nil {
				logger.LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
				return
			}

			var dataMap map[string]interface{}
			err = json.Unmarshal(jsonBytes, &dataMap)
			if err != nil {
				logger.LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
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

		if evt.Message.GetPollUpdateMessage() != nil {
			decrypted, err := mycli.clientPointer[mycli.userID].DecryptPollVote(evt)
			if err != nil {
				logger.LogError("[%s] Failed to decrypt vote: %v", mycli.userID, err)
			} else {
				logger.LogInfo("[%s] Selected options in decrypted vote:", mycli.userID)
				for _, option := range decrypted.SelectedOptions {
					logger.LogInfo("- %X", option)

				}
			}

			dataMap["isPoll"] = true
			dataMap["pollVotes"] = decrypted
		}

		if protocolMessage := evt.Message.ProtocolMessage; protocolMessage != nil {
			if protocolMessage.GetType() == waE2E.ProtocolMessage_REVOKE {
				logger.LogInfo("[%s] Message revoked", mycli.userID)

				dataMap["revoked"] = true
			} else if protocolMessage.GetType() == waE2E.ProtocolMessage_MESSAGE_EDIT {
				logger.LogInfo("[%s] Message edited", mycli.userID)
				dataMap["edited"] = true
			} else {
				return
			}
		}

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

		if mycli.config.WebhookFiles {
			isMedia := false

			img := evt.Message.GetImageMessage()
			audio := evt.Message.GetAudioMessage()
			document := evt.Message.GetDocumentMessage()
			video := evt.Message.GetVideoMessage()
			sticker := evt.Message.GetStickerMessage()

			if img != nil || audio != nil || document != nil || video != nil || sticker != nil {
				isMedia = true
			}

			if isMedia {
				var data []byte
				var err error
				var extension string
				var mimeType string

				if img != nil {
					data, err = mycli.WAClient.Download(img)
					extension = ".jpg"
					mimeType = "image/jpeg"
				} else if audio != nil {
					data, err = mycli.WAClient.Download(audio)
					extension = ".ogg"
					mimeType = "audio/ogg"
				} else if document != nil {
					data, err = mycli.WAClient.Download(document)
					extension = getExtensionFromMimeType(document.GetMimetype())
					mimeType = document.GetMimetype()
				} else if video != nil {
					data, err = mycli.WAClient.Download(video)
					extension = ".mp4"
					mimeType = "video/mp4"
				} else if sticker != nil {
					data, err = mycli.WAClient.Download(sticker)
					extension = ".png"
					mimeType = "image/png"

					webpReader := bytes.NewReader(data)
					img, err := webp.Decode(webpReader)
					if err != nil {
						logger.LogError("[%s] Failed to decode webp image: %v", mycli.userID, err)
						return
					}

					var pngBuffer bytes.Buffer
					err = png.Encode(&pngBuffer, img)
					if err != nil {
						logger.LogError("[%s] Failed to encode png image: %v", mycli.userID, err)
						return
					}

					data = pngBuffer.Bytes()
				}

				if err != nil {
					logger.LogError("[%s] Failed to download media %v", mycli.userID, err)
					return
				}

				messageMap, ok := dataMap["Message"].(map[string]interface{})
				if !ok {
					messageMap = make(map[string]interface{})
				}

				if mycli.config.MinioEnabled {
					fileName := evt.Info.ID + extension

					mediaURL, err := mycli.mediaStorage.Store(context.Background(), data, fileName, mimeType)
					if err != nil {
						logger.LogError("[%s] Failed to store media: %v", mycli.userID, err)
						return
					}
					messageMap["mediaUrl"] = mediaURL
					messageMap["mimetype"] = mimeType
				} else {
					encodeData := base64.StdEncoding.EncodeToString(data)
					messageMap["base64"] = encodeData
				}

				dataMap["Message"] = messageMap
			}
		}

		isGroup := strings.HasSuffix(evt.Info.Chat.String(), "@g.us")
		if isGroup {
			groupData, err := mycli.WAClient.GetGroupInfo(evt.Info.Chat)
			if err == nil {
				dataMap["groupData"] = groupData
			}
		}

		// profilePicUrl, err := mycli.clientPointer[mycli.userID].GetProfilePictureInfo(evt.Info.Chat, &whatsmeow.GetProfilePictureParams{
		// 	Preview: false,
		// })
		// if err != nil {
		// 	logger.LogError("[%s] Failed to get profile picture info: %v", mycli.userID, err)
		// } else {
		// 	dataMap["profilePicUrl"] = profilePicUrl.URL

		// }

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

		logger.LogInfo("[%s] Message received with ID: %s from %s with type %s", mycli.userID, evt.Info.ID, evt.Info.Chat, evt.Info.Type)
	case *events.Receipt:
		doWebhook = true
		postMap["event"] = "Receipt"
		if evt.Type == types.ReceiptTypeRead || evt.Type == types.ReceiptTypeReadSelf {

			logger.LogInfo("[%s] Message was read by %s", mycli.userID, evt.SourceString())
			if evt.Type == types.ReceiptTypeRead {
				postMap["state"] = "Read"
				for _, v := range evt.MessageIDs {
					var message message_model.Message

					message.MessageID = v
					message.Timestamp = evt.Timestamp.Format("2006-01-02 15:04:05")
					message.Status = "Read"
					message.Source = evt.Chat.ToNonAD().User

					if mycli.config.DatabaseSaveMessages {
						go mycli.messageRepository.InsertMessage(message)
					}
				}
			} else {
				postMap["state"] = "ReadSelf"
			}
		} else if evt.Type == types.ReceiptTypeReadSelf {
			postMap["state"] = "Delivered"

			var message message_model.Message

			message.MessageID = evt.MessageIDs[0]
			message.Timestamp = evt.Timestamp.Format("2006-01-02 15:04:05")
			message.Status = "Delivered"
			message.Source = evt.Chat.ToNonAD().User

			if mycli.config.DatabaseSaveMessages {
				go mycli.messageRepository.InsertMessage(message)
			}

			logger.LogInfo("[%s] Message delivered to %s", mycli.userID, evt.SourceString())
		} else {
			return
		}
	case *events.Presence:
		doWebhook = true
		postMap["event"] = "Presence"
		if evt.Unavailable {
			postMap["state"] = "offline"
			if evt.LastSeen.IsZero() {
				logger.LogInfo("[%s] User is now offline", mycli.userID)
			} else {
				logger.LogInfo("[%s] User is now offline since %s", mycli.userID, evt.LastSeen.Format("2006-01-02 15:04:05"))
			}
		} else {
			postMap["state"] = "online"
			logger.LogInfo("[%s] User is now online", mycli.userID)
		}
	case *events.HistorySync:
		doWebhook = true
		postMap["event"] = "HistorySync"
	case *events.AppState:
		logger.LogInfo("[%s] App state event received %+v", mycli.userID, evt)
	case *events.LoggedOut:
		doWebhook = true
		postMap["event"] = "LoggedOut"
		logger.LogInfo("[%s] Logged out for reason %s", mycli.userID, evt.Reason.String())
		mycli.killChannel[mycli.userID] <- true

		mycli.Instance.Connected = false
		err := mycli.instanceRepository.Update(mycli.Instance)
		if err != nil {
			logger.LogError("[%s] Error updating instance: %s", mycli.Instance.Id, err)
		}

		if postMap["data"] != nil {
			jsonBytes, err := json.Marshal(postMap["data"])
			if err != nil {
				logger.LogError("[%s] Failed to marshal postMap['data']: %v", mycli.userID, err)
				return
			}

			var dataMap map[string]interface{}
			err = json.Unmarshal(jsonBytes, &dataMap)
			if err != nil {
				logger.LogError("[%s] Failed to unmarshal postMap['data'] to map[string]interface{}: %v", mycli.userID, err)
				return
			}

			postMap["data"] = dataMap
		} else {
			postMap["data"] = make(map[string]interface{})
		}

		dataMap := postMap["data"].(map[string]interface{})

		dataMap["reason"] = evt.Reason.String()

		postMap["data"] = dataMap
	case *events.ChatPresence:
		doWebhook = true
		postMap["event"] = "ChatPresence"
		logger.LogInfo("[%s] Chat presence received %+v", mycli.userID, evt)
	case *events.CallOffer:
		doWebhook = true
		postMap["event"] = "CallOffer"
		logger.LogInfo("[%s] Got call offer %+v", mycli.userID, evt)
	case *events.CallAccept:
		doWebhook = true
		postMap["event"] = "CallAccept"
		logger.LogInfo("[%s] Got call accept %+v", mycli.userID, evt)
	case *events.CallTerminate:
		doWebhook = true
		postMap["event"] = "CallTerminate"
		logger.LogInfo("[%s] Got call terminate %+v", mycli.userID, evt)
	case *events.CallOfferNotice:
		doWebhook = true
		postMap["event"] = "CallOfferNotice"
		logger.LogInfo("[%s] Got call offer notice %+v", mycli.userID, evt)
	case *events.CallRelayLatency:
		doWebhook = true
		postMap["event"] = "CallRelayLatency"
		logger.LogInfo("[%s] Got call relay latency %+v", mycli.userID, evt)
	case *events.OfflineSyncCompleted:
		doWebhook = true
		postMap["event"] = "OfflineSyncCompleted"
	case *events.ConnectFailure:
		doWebhook = true
		postMap["event"] = "ConnectFailure"
		logger.LogInfo("[%s] Connection failed with reason %s", mycli.userID, evt.Reason.String())
	case *events.Disconnected:
		doWebhook = true
		postMap["event"] = "Disconnected"
	case *events.LabelEdit:
		doWebhook = true
		postMap["event"] = "LabelEdit"
		// store label for later use
		// action := evt.Action
		// labelID := evt.LabelID
		// actionColor := evt.Action.Color
		// actionName := evt.Action.Name
		// actionDeleted := evt.Action.Deleted
		logger.LogInfo("[%s] Got label edit %+v", mycli.userID, evt.Action)
	case *events.LabelAssociationChat:
		doWebhook = true
		postMap["event"] = "LabelAssociationChat"
	case *events.LabelAssociationMessage:
		doWebhook = true
		postMap["event"] = "LabelAssociationMessage"
	case *events.Contact:
		doWebhook = true
		postMap["event"] = "Contact"
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
	default:
		logger.LogWarn("[%s] Unhandled event %s: %+v", mycli.userID, fmt.Sprintf("%T", evt), evt)
		return
	}

	if doWebhook {

		_, found := mycli.userInfoCache.Get(mycli.token)
		if !found {
			logger.LogWarn("[%s] Could not call queue for event %s as there is no user for this token with token %s", mycli.userID, postMap["event"], mycli.token)
		}

		postMap["instanceToken"] = mycli.token
		postMap["instanceId"] = mycli.userID

		values, err := json.Marshal(postMap)
		if err != nil {
			logger.LogError("[%s] Failed to marshal JSON for queue", mycli.userID)
			return
		}

		var queueName string

		if _, ok := postMap["event"]; ok {
			queueName = strings.ToLower(fmt.Sprintf("%s.%s", userID, postMap["event"]))
		}

		go mycli.callWebhook(queueName, values)
	}
}

func (mycli *MyClient) callWebhook(queueName string, jsonData []byte) {
	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return
	}

	eventType, ok := data["event"].(string)
	if !ok {
		return
	}

	if contains(mycli.subscriptions, "ALL") {
		logger.LogInfo("[%s] Event received of type %s", mycli.userID, eventType)
		mycli.sendToQueueOrWebhook(queueName, jsonData)
		return
	}

	switch eventType {
	case "Message":
		if contains(mycli.subscriptions, "MESSAGE") {
			logger.LogInfo("[%s] Event received of type %s", mycli.userID, eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "Receipt":
		if contains(mycli.subscriptions, "READ_RECEIPT") {
			logger.LogInfo("[%s] Event received of type %s", mycli.userID, eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "Presence":
		if contains(mycli.subscriptions, "PRESENCE") {
			logger.LogInfo("[%s] Event received of type %s", mycli.userID, eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "HistorySync":
		if contains(mycli.subscriptions, "HISTORY_SYNC") {
			logger.LogInfo("[%s] Event received of type %s", mycli.userID, eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "ChatPresence":
		if contains(mycli.subscriptions, "CHAT_PRESENCE") {
			logger.LogInfo("[%s] Event received of type %s", mycli.userID, eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "CallOffer", "CallAccept", "CallTerminate", "CallOfferNotice", "CallRelayLatency":
		if contains(mycli.subscriptions, "CALL") {
			logger.LogInfo("[%s] Event received of type %s", mycli.userID, eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "Connected", "PairSuccess", "TemporaryBan", "LoggedOut", "ConnectFailure", "Disconnected":
		if contains(mycli.subscriptions, "CONNECTION") {
			logger.LogInfo("[%s] Event received of type %s", mycli.userID, eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "LabelEdit", "LabelAssociationChat", "LabelAssociationMessage":
		if contains(mycli.subscriptions, "LABEL") {
			logger.LogInfo("[%s] Event received of type %s", mycli.userID, eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "Contact":
		if contains(mycli.subscriptions, "CONTACT") {
			logger.LogInfo("[%s] Event received of type %s", mycli.userID, eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "GroupInfo", "JoinedGroup":
		if contains(mycli.subscriptions, "GROUP") {
			logger.LogInfo("[%s] Event received of type %s", mycli.userID, eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "NewsletterJoin", "NewsletterLeave":
		if contains(mycli.subscriptions, "NEWSLETTER") {
			logger.LogInfo("[%s] Event received of type %s", mycli.userID, eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "QRCode", "QRTimeout", "QRSuccess":
		if contains(mycli.subscriptions, "QRCODE") {
			logger.LogInfo("[%s] Event received of type %s", mycli.userID, eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
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

func (mycli *MyClient) sendToQueueOrWebhook(queueName string, jsonData []byte) {
	if mycli.rabbitmqEnable == "true" {
		err := mycli.rabbitmqProducer.Produce(queueName, jsonData, mycli.rabbitmqEnable, mycli.userID)
		if err != nil {
			logger.LogError("[%s] Failed to send message to rabbitmq: %s", mycli.userID, err)
			return
		}
		logger.LogInfo("[%s] Message sent to rabbitmq successfully", mycli.userID)
	}

	if mycli.websocketEnable == "enabled" || mycli.websocketEnable == "true" {
		err := mycli.websocketProducer.Produce(queueName, jsonData, mycli.userID, mycli.token)
		if err != nil {
			logger.LogError("[%s] Failed to send message to websocket: %s", mycli.userID, err)
			return
		}
		logger.LogInfo("[%s] Message sent to websocket successfully", mycli.userID)
	}

	if mycli.webhookUrl != "" && mycli.webhookUrl != "disabled" {
		err := mycli.webhookProducer.Produce(queueName, jsonData, mycli.webhookUrl, mycli.userID)
		if err != nil {
			logger.LogError("[%s] Failed to send message to webhook: %s", mycli.userID, err)
			return
		}
		logger.LogInfo("[%s] Message sent to webhook successfully", mycli.userID)
	}
}

func (w whatsmeowService) ConnectOnStartup(clientName string) {
	logger.LogInfo("Connecting all instances on startup")
	var instances []*instance_model.Instance
	var err error

	if clientName != "" {
		instances, err = w.instanceRepository.GetAllConnectedInstancesByClientName(clientName)
		if err != nil {
			logger.LogError("[%s] Error getting all connected instances: %s", clientName, err)
			return
		}
	} else {
		instances, err = w.instanceRepository.GetAllConnectedInstances()
		if err != nil {
			logger.LogError("[%s] Error getting all connected instances: %s", clientName, err)
			return
		}
	}

	logger.LogInfo("[%s] Found %d connected instances", clientName, len(instances))

	for _, instance := range instances {
		logger.LogInfo("[%s] Starting client for user '%s'", clientName, instance.Id)

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
					logger.LogWarn("[%s] Message type discarded '%s'", clientName, arg)
					continue
				}
				if !utils.Find(subscribedEvents, arg) {
					subscribedEvents = append(subscribedEvents, arg)
				}

			}
		}

		w.killChannel[instance.Id] = make(chan bool)

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
				logger.LogError("[%s] error unmarshalling proxy config", clientName)
				return
			}

			if proxyConfig.Host != "" {
				clientData.IsProxy = true
			}
		}

		go w.StartClient(clientData)
	}
}

func NewWhatsmeowService(
	instanceRepository instance_repository.InstanceRepository,
	messageRepository message_repository.MessageRepository,
	config *config.Config,
	killChannel map[string](chan bool),
	clientPointer map[string]*whatsmeow.Client,
	linkingCodeEventChannel chan LinkingCodeEvent,
	rabbitmqProducer producer_interfaces.Producer,
	webhookProducer producer_interfaces.Producer,
	websocketProducer producer_interfaces.Producer,
	sqliteDB *sql.DB,
	exPath string,
	mediaStorage storage_interfaces.MediaStorage,
) WhatsmeowService {
	return &whatsmeowService{
		instanceRepository:      instanceRepository,
		messageRepository:       messageRepository,
		config:                  config,
		killChannel:             killChannel,
		userInfoCache:           cache.New(5*time.Minute, 10*time.Minute),
		clientPointer:           clientPointer,
		linkingCodeEventChannel: linkingCodeEventChannel,
		rabbitmqProducer:        rabbitmqProducer,
		webhookProducer:         webhookProducer,
		websocketProducer:       websocketProducer,
		sqliteDB:                sqliteDB,
		exPath:                  exPath,
		mediaStorage:            mediaStorage,
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
