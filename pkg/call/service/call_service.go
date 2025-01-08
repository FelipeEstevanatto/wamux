package call_service

import (
	"errors"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	"github.com/gomessguii/logger"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type CallService interface {
	RejectCall(data *RejectCallStruct, instance *instance_model.Instance) error
}

type callService struct {
	clientPointer map[string]*whatsmeow.Client
}

type RejectCallStruct struct {
	CallCreator types.JID `json:"callCreator"`
	CallID      string    `json:"callId"`
}

func (c *callService) RejectCall(data *RejectCallStruct, instance *instance_model.Instance) error {
	if c.clientPointer[instance.Id] == nil {
		return errors.New("no session found")
	}

	err := c.clientPointer[instance.Id].RejectCall(data.CallCreator, data.CallID)
	if err != nil {
		logger.LogError("[%s] error reject call: %v", instance.Id, err)
		return err
	}

	return nil
}

func NewCallService(
	clientPointer map[string]*whatsmeow.Client,
) CallService {
	return &callService{
		clientPointer: clientPointer,
	}
}
