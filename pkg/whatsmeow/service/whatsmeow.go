package whatsmeow_service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/gomessguii/logger"
	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	"github.com/patrickmn/go-cache"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waProto "go.mau.fi/whatsmeow/binary/proto"
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
	"github.com/EvolutionAPI/evolution-go/pkg/utils"
)

type WhatsmeowService interface {
	StartClient(clientData *ClientData)
	ConnectOnStartup()
}

type whatsmeowService struct {
	instanceRepository      instance_repository.InstanceRepository
	messageRepository       message_repository.MessageRepository
	config                  *config.Config
	killChannel             map[string](chan bool)
	userInfoCache           *cache.Cache
	clientPointer           map[string]ClientInfo
	linkingCodeEventChannel chan LinkingCodeEvent
	rabbitmqProducer        producer_interfaces.Producer
	webhookProducer         producer_interfaces.Producer
	// s3Client                *S3Client
}

type MyClient struct {
	WAClient           *whatsmeow.Client
	eventHandlerID     uint32
	userID             string
	token              string
	subscriptions      []string
	webhookUrl         string
	instanceRepository instance_repository.InstanceRepository
	messageRepository  message_repository.MessageRepository
	clientPointer      map[string]ClientInfo
	killChannel        map[string](chan bool)
	userInfoCache      *cache.Cache
	config             *config.Config
	historySyncID      int32
	rabbitmqProducer   producer_interfaces.Producer
	webhookProducer    producer_interfaces.Producer
}

type ClientInfo struct {
	WAClient *whatsmeow.Client
	WSConn   *websocket.Conn
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

	if w.clientPointer[cd.Instance.Id].WAClient != nil {
		if w.clientPointer[cd.Instance.Id].WAClient.IsConnected() {
			return
		}
	}

	var container *sqlstore.Container

	if w.config.WaDebug != "" {
		dbLog := waLog.Stdout("Database", w.config.WaDebug, true)
		container, err = sqlstore.New("postgres", w.config.PostgresAuthDB, dbLog)
	} else {
		container, err = sqlstore.New("postgres", w.config.PostgresAuthDB, nil)
	}

	if err != nil {
		panic(err)
	}

	if cd.Instance.Jid != "" {
		jid, _ := utils.ParseJID(cd.Instance.Jid)
		deviceStore, err = container.GetDevice(jid)
		if err != nil {
			panic(err)
		}
	} else {
		logger.LogWarn("No jid found. Creating new device")
		deviceStore = container.NewDevice()
	}

	if deviceStore == nil {
		logger.LogWarn("No store found. Creating new one")
		deviceStore = container.NewDevice()
	}

	store.DeviceProps.PlatformType = waProto.DeviceProps_CHROME.Enum()
	store.DeviceProps.Os = &cd.Instance.OsName

	client, ok := w.clientPointer[cd.Instance.Id]
	if !ok {
		logger.LogError("Client not found")
	}

	clientLog := waLog.Stdout("Client", w.config.WaDebug, true)
	if w.config.WaDebug != "" {
		client.WAClient = whatsmeow.NewClient(deviceStore, clientLog)
	} else {
		client.WAClient = whatsmeow.NewClient(deviceStore, nil)
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
			logger.LogError("Proxy error, disabling proxy")
		} else {
			client.WAClient.SetProxy(proxy)
			logger.LogInfo("Proxy enabled")
		}
	}

	mycli := MyClient{
		WAClient:           client.WAClient,
		eventHandlerID:     1,
		userID:             cd.Instance.Id,
		token:              cd.Instance.Token,
		subscriptions:      cd.Subscriptions,
		webhookUrl:         cd.Instance.Webhook,
		instanceRepository: w.instanceRepository,
		messageRepository:  w.messageRepository,
		userInfoCache:      w.userInfoCache,
		clientPointer:      w.clientPointer,
		killChannel:        w.killChannel,
		config:             w.config,
		historySyncID:      0,
		rabbitmqProducer:   w.rabbitmqProducer,
		webhookProducer:    w.webhookProducer,
	}

	var clientHttp = make(map[string]*resty.Client)

	mycli.eventHandlerID = mycli.WAClient.AddEventHandler(mycli.myEventHandler)
	clientHttp[cd.Instance.Id] = resty.New()
	clientHttp[cd.Instance.Id].SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))
	if w.config.WaDebug == "DEBUG" {
		clientHttp[cd.Instance.Id].SetDebug(true)
	}

	clientHttp[cd.Instance.Id].SetTimeout(time.Duration(10) * time.Second)

	if client.WAClient.Store.ID == nil {
		qrChan, err := client.WAClient.GetQRChannel(context.Background())
		if err != nil {
			// This error means that we're already logged in, so ignore it.
			if !errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
				logger.LogError("Failed to get QR channel")
			}
		} else {
			if cd.Phone != "" {
				logger.LogInfo("Requesting pairing code")
				client.WAClient.Connect()
				linkingCode, err := client.WAClient.PairPhone(cd.Phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
				if err != nil {
					logger.LogError("something went wrong calling pair phone")
				}

				logger.LogInfo("Pairing code: %s", linkingCode)

				linkingCodeEvent := LinkingCodeEvent{
					LinkingCode: linkingCode,
					Token:       cd.Instance.Token,
				}

				w.linkingCodeEventChannel <- linkingCodeEvent

			} else {
				err = client.WAClient.Connect()
				if err != nil {
					panic(err)
				}
			}
			for evt := range qrChan {
				switch evt.Event {
				case "code":
					if w.config.LogType != "json" {
						fmt.Println("QR code:\n", evt.Code)
					}

					image, _ := qrcode.Encode(evt.Code, qrcode.Medium, 256)
					base64qrcode := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)

					base64WithCode := base64qrcode + "|" + evt.Code

					cd.Instance.Qrcode = base64WithCode

					err := w.instanceRepository.Update(cd.Instance)
					if err != nil {
						logger.LogError("Error updating instance: %s", err)
					}

				case "timeout":
					cd.Instance.Qrcode = ""

					err := w.instanceRepository.Update(cd.Instance)
					if err != nil {
						logger.LogError("Error updating instance: %s", err)
					}

					logger.LogWarn("QR timeout killing channel")
					delete(w.clientPointer, cd.Instance.Id)
					w.killChannel[cd.Instance.Id] <- true

				case "success":
					logger.LogInfo("QR pairing ok!")

					cd.Instance.Qrcode = ""
					cd.Instance.Connected = true

					err := w.instanceRepository.Update(cd.Instance)
					if err != nil {
						logger.LogError("Error updating instance: %s", err)
					}

				default:
					logger.LogInfo("Login event: %s", evt.Event)
				}
			}

		}
	} else {
		logger.LogInfo("Already logged in, just connect")
		err = client.WAClient.Connect()
		if err != nil {
			panic(err)
		}
	}

	for {
		select {
		case <-w.killChannel[cd.Instance.Id]:
			logger.LogInfo("Received kill signal for user '%s'", cd.Instance.Id)
			client.WAClient.Disconnect()

			delete(w.clientPointer, cd.Instance.Id)

			cd.Instance.Connected = false

			err := w.instanceRepository.Update(cd.Instance)
			if err != nil {
				logger.LogError("Error updating instance: %s", err)
			}
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
			logger.LogError("Failed to set presence as available")
		} else {
			logger.LogInfo("Marked self as available")
		}

		time.Sleep(time.Duration(7+rand.Intn(29)) * time.Second)

		err = mycli.WAClient.SendPresence(types.PresenceUnavailable)
		if err != nil {
			logger.LogError("Failed to set presence as unavailable")
		} else {
			logger.LogInfo("Marked self as unavailable")
		}
	}
}

func (mycli *MyClient) myEventHandler(rawEvt interface{}) {
	userID := mycli.userID
	postMap := make(map[string]interface{})
	postMap["data"] = rawEvt

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)

	switch evt := rawEvt.(type) {
	case *events.AppStateSyncComplete:
		// don't send webhook for this event
		if len(mycli.WAClient.Store.PushName) > 0 && evt.Name == appstate.WAPatchCriticalBlock {
			err := mycli.WAClient.SendPresence(types.PresenceUnavailable)
			if err != nil {
				logger.LogWarn("Failed to send unavailable presence")
			} else {
				logger.LogWarn("Marked self as unavailable")
			}
		}
	case *events.Connected, *events.PushNameSetting:
		postMap["event"] = "Connected" // CONNECTION
		// if len(mycli.WAClient.Store.PushName) == 0 {
		// 	return
		// }

		dataMap := postMap["data"].(map[string]interface{})

		dataMap["status"] = "open"
		dataMap["jid"] = mycli.WAClient.Store.ID.String()
		dataMap["pushName"] = mycli.WAClient.Store.PushName

		postMap["data"] = dataMap

		go schedulePresenceUpdates(mycli)

		err := mycli.WAClient.SendPresence(types.PresenceUnavailable)
		if err != nil {
			logger.LogWarn("Failed to send unavailable presence")
		} else {
			logger.LogWarn("Marked self as unavailable")
		}

		err = mycli.instanceRepository.UpdateConnected(userID, true)
		if err != nil {
			logger.LogError("Error updating instance: %s", err)
		}
	case *events.PairSuccess:
		postMap["PairSuccess"] = "PairSuccess" // CONNECTION
		logger.LogInfo("QR Pair Success for user '%s'", mycli.userID)
		jid := evt.ID

		err := mycli.instanceRepository.UpdateJid(userID, jid.String())
		if err != nil {
			logger.LogError("Error updating instance: %s", err)
		}

		myUserInfo, found := mycli.userInfoCache.Get(mycli.token)

		if !found {
			logger.LogWarn("No user info cached on pairing?")
		} else {
			txtid := myUserInfo.(Values).Get("Id")
			token := myUserInfo.(Values).Get("Token")

			v := utils.UpdateUserInfo(myUserInfo, "Jid", jid.String())

			mycli.userInfoCache.Set(token, v, cache.NoExpiration)
			logger.LogInfo("User information set for user '%s'", txtid)
		}

		dataMap := postMap["data"].(map[string]interface{})

		dataMap["status"] = "open"
		dataMap["jid"] = mycli.WAClient.Store.ID.String()
		dataMap["pushName"] = mycli.WAClient.Store.PushName

		postMap["data"] = dataMap
	case *events.StreamReplaced:
		logger.LogInfo("Received StreamReplaced event")
		return
	case *events.TemporaryBan:
		logger.LogInfo("User received temporary ban for %s", evt.Code.String())
		postMap["event"] = "TemporaryBan" // CONNECTION

		post := make(map[string]interface{})
		post["reason"] = evt.Code.String()
		post["expire"] = evt.Expire

		postMap["data"] = post
	case *events.Message:
		postMap["event"] = "Message" // MESSAGE

		// metaParts := []string{fmt.Sprintf("pushname: %s", evt.Info.PushName), fmt.Sprintf("timestamp: %s", evt.Info.Timestamp)}
		// if evt.Info.Type != "" {
		// 	metaParts = append(metaParts, fmt.Sprintf("type: %s", evt.Info.Type))
		// }
		// if evt.Info.Category != "" {
		// 	metaParts = append(metaParts, fmt.Sprintf("category: %s", evt.Info.Category))
		// }
		// if evt.IsViewOnce {
		// 	metaParts = append(metaParts, "view once")
		// }
		// if evt.IsViewOnce {
		// 	metaParts = append(metaParts, "ephemeral")
		// }

		if protocolMessage := evt.Message.ProtocolMessage; protocolMessage != nil {
			if protocolMessage.GetType() == waE2E.ProtocolMessage_REVOKE {
				logger.LogInfo("Message revoked")
				postMap["revoked"] = true
			}
		}

		if mycli.clientPointer[userID].WSConn != nil {
			logger.LogInfo("Sending message to ws")
			// convert evt to json
			jsonBytes, err := json.Marshal(evt)
			if err != nil {
				logger.LogError("Error marshalling message to json")
			}
			// convert json to string
			jsonString := string(jsonBytes)

			err = mycli.clientPointer[userID].WSConn.WriteJSON(jsonString)
			if err != nil {
				logger.LogError("Error sending message to ws")
			}
		}

		// if mycli.config.WebhookFiles {
		// 	img := evt.Message.GetImageMessage()
		// 	if img != nil {
		// 		data, err := mycli.WAClient.Download(img)
		// 		if err != nil {
		// 			logger.LogError("Failed to download image")
		// 			return
		// 		}

		// 		extension := ""
		// 		exts, err := mime.ExtensionsByType(img.GetMimetype())
		// 		if err == nil && len(exts) > 0 {
		// 			extension = exts[0]
		// 		}

		// 		// Preparar chave para o S3
		// 		key := fmt.Sprintf("%s/%s%s", bucketFolder, evt.Info.ID, extension)
		// 		url, err := uploadToS3(s3Client.GetClient(), bucketName, key, data)

		// 		if err != nil {
		// 			log.Error().Err(err).Msg("Failed to upload image to S3")
		// 			return
		// 		}
		// 		log.Info().Str("url", url).Msg("Image uploaded to S3")

		// 		// Adicione a URL ao payload do webhook
		// 		postmap["mediaUrl"] = url
		// 	}

		// 	// try to get Audio if any
		// 	audio := evt.Message.GetAudioMessage()
		// 	if audio != nil {
		// 		data, err := mycli.WAClient.Download(audio)
		// 		if err != nil {
		// 			log.Error().Err(err).Msg("Failed to download audio")
		// 			return
		// 		}

		// 		// Determinar a extensão do arquivo
		// 		extension := ""
		// 		exts, err := mime.ExtensionsByType(audio.GetMimetype())
		// 		if err == nil && len(exts) > 0 {
		// 			extension = exts[0]
		// 		}

		// 		// Preparar chave para o S3
		// 		key := fmt.Sprintf("%s/%s%s", bucketFolder, evt.Info.ID, extension)
		// 		url, err := uploadToS3(s3Client.GetClient(), bucketName, key, data)

		// 		if err != nil {
		// 			log.Error().Err(err).Msg("Failed to upload audio to S3")
		// 			return
		// 		}
		// 		log.Info().Str("url", url).Msg("Audio uploaded to S3")

		// 		// Adicione a URL ao payload do webhook
		// 		postmap["mediaUrl"] = url
		// 	}

		// 	// try to get Document if any
		// 	document := evt.Message.GetDocumentMessage()
		// 	if document != nil {
		// 		data, err := mycli.WAClient.Download(document)
		// 		if err != nil {
		// 			log.Error().Err(err).Msg("Failed to download document")
		// 			return
		// 		}

		// 		// Determinar a extensão do arquivo
		// 		extension := ""
		// 		exts, err := mime.ExtensionsByType(document.GetMimetype())
		// 		if err == nil && len(exts) > 0 {
		// 			extension = exts[0]
		// 		} else if document.FileName != nil {
		// 			extension = filepath.Ext(*document.FileName)
		// 		}

		// 		// Preparar chave para o S3
		// 		key := fmt.Sprintf("%s/%s%s", bucketFolder, evt.Info.ID, extension)
		// 		url, err := uploadToS3(s3Client.GetClient(), bucketName, key, data)

		// 		if err != nil {
		// 			log.Error().Err(err).Msg("Failed to upload document to S3")
		// 			return
		// 		}
		// 		log.Info().Str("url", url).Msg("Document uploaded to S3")

		// 		// Adicione a URL ao payload do webhook
		// 		postmap["mediaUrl"] = url
		// 	}

		// 	video := evt.Message.GetVideoMessage()
		// 	if video != nil {
		// 		data, err := mycli.WAClient.Download(video)
		// 		if err != nil {
		// 			log.Error().Err(err).Msg("Failed to download video")
		// 			return
		// 		}

		// 		// Determinar a extensão do arquivo
		// 		extension := ""
		// 		exts, err := mime.ExtensionsByType(video.GetMimetype())
		// 		if err == nil && len(exts) > 0 {
		// 			extension = exts[0]
		// 		}

		// 		// Preparar chave para o S3
		// 		key := fmt.Sprintf("%s/%s%s", bucketFolder, evt.Info.ID, extension)
		// 		url, err := uploadToS3(s3Client.GetClient(), bucketName, key, data)

		// 		if err != nil {
		// 			log.Error().Err(err).Msg("Failed to upload video to S3")
		// 			return
		// 		}
		// 		log.Info().Str("url", url).Msg("Video uploaded to S3")

		// 		// Adicione a URL ao payload do webhook
		// 		postmap["mediaUrl"] = url
		// 	}
		// }

		logger.LogInfo("Message received with ID: %s from %s", evt.Info.ID, evt.Info.Chat)
	case *events.Receipt:
		postMap["event"] = "Receipt" // READ_RECEIPT
		if evt.Type == types.ReceiptTypeRead || evt.Type == types.ReceiptTypeReadSelf {

			logger.LogInfo("Message was read by %s", evt.SourceString())
			if evt.Type == types.ReceiptTypeRead {
				postMap["state"] = "Read"
				for _, v := range evt.MessageIDs {
					var message message_model.Message

					message.MessageID = v
					message.Timestamp = evt.Timestamp.Format("2006-01-02 15:04:05")
					message.Status = "Read"
					message.Source = evt.Chat.ToNonAD().User

					go mycli.messageRepository.InsertMessage(message)
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

			go mycli.messageRepository.InsertMessage(message)

			logger.LogInfo("Message delivered to %s", evt.SourceString())
		} else {
			return
		}
	case *events.Presence:
		postMap["event"] = "Presence" // PRESENCE
		if evt.Unavailable {
			postMap["state"] = "offline"
			if evt.LastSeen.IsZero() {
				logger.LogInfo("User is now offline")
			} else {
				logger.LogInfo("User is now offline since %s", evt.LastSeen.Format("2006-01-02 15:04:05"))
			}
		} else {
			postMap["state"] = "online"
			logger.LogInfo("User is now online")
		}
	case *events.HistorySync:
		postMap["event"] = "HistorySync" // HISTORY_SYNC

		userDirectory := fmt.Sprintf("%s/files/user_%s", exPath, userID)
		_, err := os.Stat(userDirectory)
		if os.IsNotExist(err) {
			errDir := os.MkdirAll(userDirectory, 0751)
			if errDir != nil {
				logger.LogError("Could not create user directory")
				return
			}
		}

		id := atomic.AddInt32(&mycli.historySyncID, 1)
		fileName := fmt.Sprintf("%s/history-%d.json", userDirectory, id)
		file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			logger.LogError("Failed to open file to write history sync")
			return
		}
		enc := json.NewEncoder(file)
		enc.SetIndent("", "  ")
		err = enc.Encode(evt.Data)
		if err != nil {
			logger.LogError("Failed to write history sync")
			return
		}
		logger.LogInfo("Wrote history sync to %s", fileName)
		_ = file.Close()
	case *events.AppState:
		logger.LogInfo("App state event received %+v", evt)
	case *events.LoggedOut:
		postMap["event"] = "LoggedOut" // CONNECTION
		logger.LogInfo("Logged out for reason %s", evt.Reason.String())
		mycli.killChannel[mycli.userID] <- true

		err := mycli.instanceRepository.UpdateConnected(mycli.userID, false)
		if err != nil {
			logger.LogError("Error updating instance: %s", err)
		}

		post := make(map[string]interface{})
		post["reason"] = evt.Reason.String()
		postMap["data"] = post
	case *events.ChatPresence:
		postMap["event"] = "ChatPresence" // CHAT_PRESENCE
		logger.LogInfo("Chat presence received %+v", evt)
	case *events.CallOffer:
		postMap["event"] = "CallOffer" // CALL
		logger.LogInfo("Got call offer %+v", evt)
	case *events.CallAccept:
		postMap["event"] = "CallAccept" // CALL
		logger.LogInfo("Got call accept %+v", evt)
	case *events.CallTerminate:
		postMap["event"] = "CallTerminate" // CALL
		logger.LogInfo("Got call terminate %+v", evt)
	case *events.CallOfferNotice:
		postMap["event"] = "CallOfferNotice" // CALL
		logger.LogInfo("Got call offer notice %+v", evt)
	case *events.CallRelayLatency:
		postMap["event"] = "CallRelayLatency" // CALL
		logger.LogInfo("Got call relay latency %+v", evt)
	case *events.OfflineSyncCompleted:
		postMap["event"] = "OfflineSyncCompleted" // CALL
	case *events.ConnectFailure:
		postMap["event"] = "ConnectFailure" // CONNECTION
		logger.LogInfo("Connection failed with reason %s", evt.Reason.String())
	case *events.Disconnected:
		postMap["event"] = "Disconnected" // CONNECTION
	case *events.LabelEdit:
		postMap["event"] = "LabelEdit" // LABEL
		// store label for later use
		// action := evt.Action
		// labelID := evt.LabelID
		// actionColor := evt.Action.Color
		// actionName := evt.Action.Name
		// actionDeleted := evt.Action.Deleted
		logger.LogInfo("Got label edit %+v", evt.Action)
	case *events.LabelAssociationChat:
		postMap["event"] = "LabelAssociationChat" // LABEL
	case *events.LabelAssociationMessage:
		postMap["event"] = "LabelAssociationMessage" // LABEL
	case *events.Contact:
		postMap["event"] = "Contact" // CONTACT
	case *events.GroupInfo:
		postMap["event"] = "GroupInfo" // GROUP
	case *events.JoinedGroup:
		postMap["event"] = "JoinedGroup" // GROUP
	case *events.NewsletterJoin:
		postMap["event"] = "NewsletterJoin" // NEWSLETTER
	case *events.NewsletterLeave:
		postMap["event"] = "NewsletterLeave" // NEWSLETTER
	default:
		logger.LogWarn("Unhandled event %+v", evt)
		return
	}

	_, found := mycli.userInfoCache.Get(mycli.token)
	if !found {
		logger.LogWarn("Could not call queue as there is no user for this token with token %s", mycli.token)
	}

	postMap["instanceToken"] = mycli.token
	postMap["instanceId"] = mycli.userID

	values, err := json.Marshal(postMap)
	if err != nil {
		logger.LogError("Failed to marshal JSON for queue")
		return
	}

	var queueName string

	if _, ok := postMap["event"]; ok {
		queueName = fmt.Sprintf("%s.%s", userID, postMap["event"])
	}

	go mycli.callWebhook(queueName, values)
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
		logger.LogInfo("Event received of type %s", eventType)
		mycli.sendToQueueOrWebhook(queueName, jsonData)
		return
	}

	switch eventType {
	case "Message":
		if contains(mycli.subscriptions, "MESSAGE") {
			logger.LogInfo("Event received of type %s", eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "Receipt":
		if contains(mycli.subscriptions, "READ_RECEIPT") {
			logger.LogInfo("Event received of type %s", eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "Presence":
		if contains(mycli.subscriptions, "PRESENCE") {
			logger.LogInfo("Event received of type %s", eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "HistorySync":
		if contains(mycli.subscriptions, "HISTORY_SYNC") {
			logger.LogInfo("Event received of type %s", eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "ChatPresence":
		if contains(mycli.subscriptions, "CHAT_PRESENCE") {
			logger.LogInfo("Event received of type %s", eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "CallOffer", "CallAccept", "CallTerminate", "CallOfferNotice", "CallRelayLatency":
		if contains(mycli.subscriptions, "CALL") {
			logger.LogInfo("Event received of type %s", eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "Connected", "PairSuccess", "TemporaryBan", "LoggedOut", "ConnectFailure", "Disconnected":
		if contains(mycli.subscriptions, "CONNECTION") {
			logger.LogInfo("Event received of type %s", eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "LabelEdit", "LabelAssociationChat", "LabelAssociationMessage":
		if contains(mycli.subscriptions, "LABEL") {
			logger.LogInfo("Event received of type %s", eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "Contact":
		if contains(mycli.subscriptions, "CONTACT") {
			logger.LogInfo("Event received of type %s", eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "GroupInfo", "JoinedGroup":
		if contains(mycli.subscriptions, "GROUP") {
			logger.LogInfo("Event received of type %s", eventType)
			mycli.sendToQueueOrWebhook(queueName, jsonData)
		}
	case "NewsletterJoin", "NewsletterLeave":
		if contains(mycli.subscriptions, "NEWSLETTER") {
			logger.LogInfo("Event received of type %s", eventType)
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
	if mycli.config.AmqpUrl != "" {
		err := mycli.rabbitmqProducer.Produce(queueName, jsonData, "")
		if err != nil {
			logger.LogError("Failed to send message to rabbitmq: %s", err)
			return
		}
		logger.LogInfo("Message enqueued successfully")
	}

	err := mycli.webhookProducer.Produce(queueName, jsonData, mycli.webhookUrl)
	if err != nil {
		logger.LogError("Failed to send message to webhook: %s", err)
		return
	}
	logger.LogInfo("Message sent to webhook successfully")
}

func (w whatsmeowService) ConnectOnStartup() {
	logger.LogInfo("Connecting all instances on startup")
	instances, err := w.instanceRepository.GetAllConnectedInstances()
	if err != nil {
		logger.LogError("Error getting all connected instances: %s", err)
		return
	}

	for _, instance := range instances {
		logger.LogInfo("Starting client for user '%s'", instance.Id)

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
					logger.LogWarn("Message type discarded '%s'", arg)
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
				logger.LogError("error unmarshalling proxy config")
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
	clientPointer map[string]ClientInfo,
	linkingCodeEventChannel chan LinkingCodeEvent,
	rabbitmqProducer producer_interfaces.Producer,
	webhookProducer producer_interfaces.Producer,
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
	}
}
