package instance_service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/EvolutionAPI/evolution-go/pkg/config"
	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	instance_repository "github.com/EvolutionAPI/evolution-go/pkg/instance/repository"
	event_types "github.com/EvolutionAPI/evolution-go/pkg/internal/event_types"
	"github.com/EvolutionAPI/evolution-go/pkg/utils"
	whatsmeow_service "github.com/EvolutionAPI/evolution-go/pkg/whatsmeow/service"
	"github.com/gomessguii/logger"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type InstanceService interface {
	Create(data *CreateStruct) (*instance_model.Instance, error)
	Connect(data *ConnectStruct, instance *instance_model.Instance) (*instance_model.Instance, string, string, error)
	Reconnect(instance *instance_model.Instance) error
	Disconnect(instance *instance_model.Instance) (*instance_model.Instance, error)
	Logout(instance *instance_model.Instance) (*instance_model.Instance, error)
	Status(instance *instance_model.Instance) (*StatusStruct, error)
	GetQr(instance *instance_model.Instance) (*QrcodeStruct, error)
	Pair(data *PairStruct, instance *instance_model.Instance) (*PairReturnStruct, error)
	GetAll() ([]*instance_model.Instance, error)
	Info(instanceId string) (*instance_model.Instance, error)
	Delete(id string) error
	RemoveProxy(id string) error
	GetInstanceByToken(token string) (*instance_model.Instance, error)
}

type instances struct {
	instanceRepository      instance_repository.InstanceRepository
	config                  *config.Config
	killChannel             map[string](chan bool)
	clientPointer           map[string]*whatsmeow.Client
	linkingCodeEventChannel chan whatsmeow_service.LinkingCodeEvent
	whatsmeowService        whatsmeow_service.WhatsmeowService
}

type ProxyConfig struct {
	Port     string `json:"port"`
	Password string `json:"password"`
	Username string `json:"username"`
	Host     string `json:"host"`
}

type CreateStruct struct {
	InstanceId string       `json:"instanceId"`
	Name       string       `json:"name"`
	Token      string       `json:"token"`
	Proxy      *ProxyConfig `json:"proxy"`
}

type ConnectStruct struct {
	WebhookUrl      string   `json:"webhookUrl"`
	Subscribe       []string `json:"subscribe"`
	Immediate       bool     `json:"immediate"`
	Phone           string   `json:"phone"`
	RabbitmqEnable  string   `json:"rabbitmqEnable"`
	WebSocketEnable string   `json:"websocketEnable"`
}

type StatusStruct struct {
	Connected bool
	LoggedIn  bool
	myJid     *types.JID
	Name      string
}

type QrcodeStruct struct {
	Qrcode string
	Code   string
}

type PairStruct struct {
	Subscribe []string `json:"subscribe"`
	Phone     string   `json:"phone"`
}

type PairReturnStruct struct {
	PairingCode string
}

func (i *instances) ensureClientConnected(instanceId string) (*whatsmeow.Client, error) {
	client := i.clientPointer[instanceId]
	logger.LogInfo("[%s] Checking client connection status - Client exists: %v", instanceId, client != nil)

	if client == nil {
		logger.LogInfo("[%s] No client found, attempting to start new instance", instanceId)
		err := i.whatsmeowService.StartInstance(instanceId)
		if err != nil {
			logger.LogError("[%s] Failed to start instance: %v", instanceId, err)
			return nil, errors.New("no active session found")
		}

		logger.LogInfo("[%s] Instance started, waiting 2 seconds...", instanceId)
		time.Sleep(2 * time.Second)

		client = i.clientPointer[instanceId]
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
	proxyJson, err := json.Marshal(data.Proxy)
	if err != nil {
		return nil, err
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

	createdInstance, err := i.instanceRepository.Create(instance)
	if err != nil {
		return nil, err
	}

	return createdInstance, nil
}

func (i instances) Connect(data *ConnectStruct, instance *instance_model.Instance) (*instance_model.Instance, string, string, error) {

	var subscribedEvents []string

	if len(data.Subscribe) < 1 {
		subscribedEvents = append(subscribedEvents, event_types.MESSAGE)
	} else {
		for _, arg := range data.Subscribe {
			if !event_types.IsEventType(arg) {
				logger.LogWarn("[%s] Message type discarded '%s'", instance.Id, arg)
				continue
			}
			if !utils.Find(subscribedEvents, arg) {
				subscribedEvents = append(subscribedEvents, arg)
			}

		}
	}
	eventString := strings.Join(subscribedEvents, ",")

	instance.Events = eventString
	instance.Webhook = data.WebhookUrl
	instance.RabbitmqEnable = data.RabbitmqEnable
	instance.WebSocketEnable = data.WebSocketEnable

	err := i.instanceRepository.Update(instance)
	if err != nil {
		logger.LogError("[%s] Error updating instance: %s", instance.Id, err)
		return nil, "", "", err
	}

	i.killChannel[instance.Id] = make(chan bool)

	clientData := &whatsmeow_service.ClientData{
		Instance:      instance,
		Subscriptions: subscribedEvents,
		Phone:         data.Phone,
		IsProxy:       false,
	}

	if instance.Proxy != "" {
		var proxyConfig ProxyConfig
		err := json.Unmarshal([]byte(instance.Proxy), &proxyConfig)
		if err != nil {
			logger.LogError("[%s] error unmarshalling proxy config", instance.Id, err)
			return nil, "", "", err
		}

		if proxyConfig.Host != "" {
			clientData.IsProxy = true
		}
	}

	go i.whatsmeowService.StartClient(clientData, false)

	// logger.LogInfo("Waiting 1 seconds")
	// time.Sleep(1000 * time.Millisecond)

	// if i.clientPointer[instance.Id] != nil {
	// 	if !i.clientPointer[instance.Id].IsConnected() {
	// 		return instance, "", "", fmt.Errorf("failed to connect")
	// 	}
	// } else {
	// 	return instance, "", "", fmt.Errorf("failed to connect")
	// }

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
			logger.LogInfo("[%s] Disconnection successful", instance.Id)
			i.killChannel[instance.Id] <- true

			instance.Events = ""

			err := i.instanceRepository.Update(instance)
			if err != nil {
				return instance, err
			}

			return instance, nil
		}
	}

	logger.LogWarn("[%s] Ignoring disconnect as it was not connected", instance.Id)
	return instance, nil
}

func (i instances) Logout(instance *instance_model.Instance) (*instance_model.Instance, error) {
	client, err := i.ensureClientConnected(instance.Id)
	if err != nil {
		return instance, err
	}

	if client.IsLoggedIn() && client.IsConnected() {
		err := client.Logout()
		if err != nil {
			return instance, err
		}

		instance.Jid = ""
		instance.Connected = false
		err = i.instanceRepository.Update(instance)
		if err != nil {
			return instance, err
		}

		select {
		case i.killChannel[instance.Id] <- true:
		case <-time.After(5 * time.Second):
		}

		delete(i.clientPointer, instance.Id)
		delete(i.killChannel, instance.Id)

		logger.LogInfo("[%s] Logout successful", instance.Id)
		return instance, nil
	}

	if client.IsConnected() {
		client.Disconnect()

		select {
		case i.killChannel[instance.Id] <- true:
		case <-time.After(5 * time.Second):
		}

		delete(i.clientPointer, instance.Id)
		delete(i.killChannel, instance.Id)

		logger.LogInfo("[%s] Disconnection successful", instance.Id)
		return instance, nil
	}

	logger.LogWarn("[%s] Ignoring logout as it was not connected", instance.Id)
	return instance, fmt.Errorf("ignoring logout as it was not connected")
}

func (i instances) Status(instance *instance_model.Instance) (*StatusStruct, error) {
	client, err := i.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	isConnected := client.IsConnected()
	isLoggedIn := client.IsLoggedIn()

	var myJid *types.JID
	var name string
	if isLoggedIn {
		myJid = client.Store.ID
		name = client.Store.PushName
	}

	status := &StatusStruct{
		Connected: isConnected,
		LoggedIn:  isLoggedIn,
		myJid:     myJid,
		Name:      name,
	}

	return status, nil
}

func (i instances) GetQr(instance *instance_model.Instance) (*QrcodeStruct, error) {
	client, err := i.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	if client.IsLoggedIn() {
		return nil, fmt.Errorf("session already logged in")
	}

	instance, err = i.instanceRepository.GetInstanceByID(instance.Id)
	if err != nil {
		return nil, err
	}

	code := instance.Qrcode
	if code == "" {
		return nil, fmt.Errorf("no QR code available")
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

func (i instances) Pair(data *PairStruct, instance *instance_model.Instance) (*PairReturnStruct, error) {
	if i.clientPointer[instance.Id] != nil {
		i.clientPointer[instance.Id].Disconnect()
		delete(i.clientPointer, instance.Id)
		// return nil, fmt.Errorf("client set to nil")
	}

	var eventArray []string
	var subscribedEvents []string

	if len(data.Subscribe) > 0 {
		eventArray = data.Subscribe
	} else {
		eventArray = strings.Split(instance.Events, ",")
	}

	if len(eventArray) < 1 {
		subscribedEvents = append(subscribedEvents, "MESSAGE")
	} else {
		for _, arg := range eventArray {
			if !event_types.IsEventType(arg) {
				logger.LogWarn("[%s] Message type discarded '%s'", instance.Id, arg)
				continue
			}
			if !utils.Find(subscribedEvents, arg) {
				subscribedEvents = append(subscribedEvents, arg)
			}

		}
	}

	instance.Events = strings.Join(subscribedEvents, ",")

	err := i.instanceRepository.Update(instance)
	if err != nil {
		logger.LogError("[%s] Error updating instance: %s", instance.Id, err)
		return nil, err
	}

	i.killChannel[instance.Id] = make(chan bool)

	clientData := &whatsmeow_service.ClientData{
		Instance:      instance,
		Subscriptions: subscribedEvents,
		Phone:         data.Phone,
		IsProxy:       false,
	}

	if instance.Proxy != "" {
		var proxyConfig ProxyConfig
		err := json.Unmarshal([]byte(instance.Proxy), &proxyConfig)
		if err != nil {
			logger.LogError("[%s] error unmarshalling proxy config", instance.Id, err)
			return nil, err
		}

		if proxyConfig.Host != "" {
			clientData.IsProxy = true
		}
	}

	go i.whatsmeowService.StartClient(clientData, false)

	logger.LogInfo("[%s] Waiting 1 seconds", instance.Id)
	time.Sleep(1000 * time.Millisecond)

	if i.clientPointer[instance.Id] != nil {
		if !i.clientPointer[instance.Id].IsConnected() {
			return nil, fmt.Errorf("failed to connect")
		}
	} else {
		return nil, fmt.Errorf("failed to connect")
	}

	select {
	case evt := <-i.linkingCodeEventChannel:
		code := evt.LinkingCode
		return &PairReturnStruct{PairingCode: code}, nil

	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("timeout waiting for linking code event")
	}
}

func (i instances) GetAll() ([]*instance_model.Instance, error) {
	instances, err := i.instanceRepository.GetAll(i.config.ClientName)
	if err != nil {
		return nil, err
	}

	return instances, nil
}

func (i instances) Info(instanceId string) (*instance_model.Instance, error) {
	instance, err := i.instanceRepository.GetInstanceByID(instanceId)
	if err != nil {
		return nil, err
	}

	return instance, nil
}

func (i instances) Delete(id string) error {
	instance, err := i.instanceRepository.GetInstanceByID(id)
	if err != nil {
		return err
	}

	if i.clientPointer[instance.Id] != nil && i.clientPointer[instance.Id].IsConnected() {
		if i.clientPointer[instance.Id].IsLoggedIn() {
			i.clientPointer[instance.Id].Logout()
		}
		i.clientPointer[instance.Id].Disconnect()
	}

	err = i.instanceRepository.Delete(id)
	if err != nil {
		return err
	}

	return nil
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

	return nil
}

func (i instances) GetInstanceByToken(token string) (*instance_model.Instance, error) {
	return i.instanceRepository.GetInstanceByToken(token)
}

func NewInstanceService(
	instanceRepository instance_repository.InstanceRepository,
	killChannel map[string](chan bool),
	clientPointer map[string]*whatsmeow.Client,
	linkingCodeEventChannel chan whatsmeow_service.LinkingCodeEvent,
	whatsmeowService whatsmeow_service.WhatsmeowService,
	config *config.Config,
) InstanceService {
	return &instances{
		instanceRepository:      instanceRepository,
		killChannel:             killChannel,
		clientPointer:           clientPointer,
		linkingCodeEventChannel: linkingCodeEventChannel,
		whatsmeowService:        whatsmeowService,
		config:                  config,
	}
}
