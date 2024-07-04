package session_service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	instance_model "github.com/Zapbox-API/evolution-go/pkg/instances/model"
	instance_repository "github.com/Zapbox-API/evolution-go/pkg/instances/repository"
	"github.com/Zapbox-API/evolution-go/pkg/utils"
	whatsmeow_service "github.com/Zapbox-API/evolution-go/pkg/whatsmeow/service"
	"github.com/gomessguii/logger"
)

type SessionService interface {
	Init(data *InitStruct) error
	Connect(data *ConnectStruct, instance *instance_model.Instance) (*instance_model.Instance, error)
}

type sessions struct {
	instanceRepository instance_repository.InstanceRepository
	killChannel        map[int](chan bool)
	clientPointer      map[int]whatsmeow_service.ClientInfo
	whatsmeowService   whatsmeow_service.WhatsmeowService
}

type ProxyConfig struct {
	Port     string `json:"port"`
	Password string `json:"password"`
	Username string `json:"username"`
	Address  string `json:"address"`
}

type InitStruct struct {
	Name  string       `json:"name"`
	Token string       `json:"token"`
	Os    string       `json:"os"`
	Proxy *ProxyConfig `json:"proxy"`
}

type ConnectStruct struct {
	Subscribe []string
	Immediate bool
	Phone     string
}

func (s sessions) Init(data *InitStruct) error {
	proxyJson, err := json.Marshal(data.Proxy)
	if err != nil {
		return err
	}

	instance := instance_model.Instance{
		Name:      data.Name,
		Token:     data.Token,
		OsName:    data.Os,
		Proxy:     string(proxyJson),
		Connected: false,
	}

	err = s.instanceRepository.Create(instance)
	if err != nil {
		return err
	}

	return nil
}

func (s sessions) Connect(data *ConnectStruct, instance *instance_model.Instance) (*instance_model.Instance, error) {

	var subscribedEvents []string

	if len(data.Subscribe) < 1 {
		subscribedEvents = append(subscribedEvents, "MESSAGE")
	} else {
		for _, arg := range data.Subscribe {
			if !utils.ValidateEvent(arg) {
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

	err := s.instanceRepository.Update(instance)
	if err != nil {
		logger.LogError("Error updating instance: %s", err)
		return nil, err
	}

	s.killChannel[instance.Id] = make(chan bool)

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

		if proxyConfig.Address != "" {
			clientData.IsProxy = true
		}
	}

	go s.whatsmeowService.StartClient(clientData)

	if !data.Immediate {
		logger.LogInfo("Waiting 10 seconds")
		time.Sleep(10000 * time.Millisecond)

		if s.clientPointer[instance.Id].WAClient != nil {
			if !s.clientPointer[instance.Id].WAClient.IsConnected() {
				return instance, fmt.Errorf("failed to connect")
			}
		} else {
			return instance, fmt.Errorf("failed to connect")
		}
	}

	return instance, nil
}

func NewSessionService(
	instanceRepository instance_repository.InstanceRepository,
	killChannel map[int](chan bool),
	clientPointer map[int]whatsmeow_service.ClientInfo,
	whatsmeowService whatsmeow_service.WhatsmeowService,
) SessionService {
	return &sessions{
		instanceRepository: instanceRepository,
		killChannel:        killChannel,
		clientPointer:      clientPointer,
		whatsmeowService:   whatsmeowService,
	}
}
