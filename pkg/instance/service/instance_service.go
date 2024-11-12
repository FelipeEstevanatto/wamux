package instance_service

import (
	"encoding/json"
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
	WebhookUrl string   `json:"webhookUrl"`
	Subscribe  []string `json:"subscribe"`
	Immediate  bool     `json:"immediate"`
	Phone      string   `json:"phone"`
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
				logger.LogWarn("Message type discarded '%s'", arg)
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

	err := i.instanceRepository.Update(instance)
	if err != nil {
		logger.LogError("Error updating instance: %s", err)
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
			logger.LogError("error unmarshalling proxy config")
			return nil, "", "", err
		}

		if proxyConfig.Host != "" {
			clientData.IsProxy = true
		}
	}

	go i.whatsmeowService.StartClient(clientData)

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

func (i instances) Disconnect(instance *instance_model.Instance) (*instance_model.Instance, error) {
	if i.clientPointer[instance.Id] == nil {
		return instance, fmt.Errorf("no session found")
	}

	if i.clientPointer[instance.Id].IsConnected() {
		if i.clientPointer[instance.Id].IsLoggedIn() {
			logger.LogInfo("Disconnection successful")
			i.killChannel[instance.Id] <- true

			instance.Events = ""

			err := i.instanceRepository.Update(instance)
			if err != nil {
				return instance, err
			}

			return instance, nil
		}
	}

	logger.LogWarn("Ignoring disconnect as it was not connected")
	return instance, nil
}

func (i instances) Logout(instance *instance_model.Instance) (*instance_model.Instance, error) {
	if i.clientPointer[instance.Id] == nil {
		return instance, fmt.Errorf("no session found")
	}

	if i.clientPointer[instance.Id].IsLoggedIn() && i.clientPointer[instance.Id].IsConnected() {
		err := i.clientPointer[instance.Id].Logout()
		if err != nil {
			return instance, err
		}

		instance.Jid = ""

		err = i.instanceRepository.Update(instance)
		if err != nil {
			return instance, err
		}

		logger.LogInfo("Logout successful")
		i.killChannel[instance.Id] <- true
	} else {
		if i.clientPointer[instance.Id].IsConnected() {
			// chama o disconnect
			logger.LogInfo("Logout successful")
			i.killChannel[instance.Id] <- true
		} else {
			logger.LogWarn("Ignoring logout as it was not connected")
			return instance, fmt.Errorf("ignoring logout as it was not connected")

		}
	}

	return instance, nil
}

func (i instances) Status(instance *instance_model.Instance) (*StatusStruct, error) {
	if i.clientPointer[instance.Id] == nil {
		return nil, fmt.Errorf("no session found")
	}

	isConnected := i.clientPointer[instance.Id].IsConnected()
	isLoggedIn := i.clientPointer[instance.Id].IsLoggedIn()

	var myJid *types.JID
	var name string
	if isLoggedIn {
		myJid = i.clientPointer[instance.Id].Store.ID
		name = i.clientPointer[instance.Id].Store.PushName
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
	if i.clientPointer[instance.Id].IsLoggedIn() {
		return nil, fmt.Errorf("session already logged in")
	}

	instance, err := i.instanceRepository.GetInstanceByID(instance.Id)
	if err != nil {
		return nil, err
	}

	code := instance.Qrcode

	base64 := strings.Split(code, "|")[0]
	code = strings.Split(code, "|")[1]

	qr := &QrcodeStruct{
		Qrcode: base64,
		Code:   code,
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
				logger.LogWarn("Message type discarded '%s'", arg)
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
		logger.LogError("Error updating instance: %s", err)
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
			logger.LogError("error unmarshalling proxy config")
			return nil, err
		}

		if proxyConfig.Host != "" {
			clientData.IsProxy = true
		}
	}

	go i.whatsmeowService.StartClient(clientData)

	logger.LogInfo("Waiting 1 seconds")
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
