package newsletter_service

import (
	"context"
	"errors"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	whatsmeow_service "github.com/EvolutionAPI/evolution-go/pkg/whatsmeow/service"
	"github.com/gomessguii/logger"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type NewsletterService interface {
	CreateNewsletter(data *CreateNewsletterStruct, instance *instance_model.Instance) (*types.NewsletterMetadata, error)
	ListNewsletter(instance *instance_model.Instance) ([]*types.NewsletterMetadata, error)
	GetNewsletter(data *GetNewsletterStruct, instance *instance_model.Instance) (*types.NewsletterMetadata, error)
	GetNewsletterInvite(data *GetNewsletterInviteStruct, instance *instance_model.Instance) (*types.NewsletterMetadata, error)
	SubscribeNewsletter(data *GetNewsletterStruct, instance *instance_model.Instance) error
	GetNewsletterMessages(data *GetNewsletterMessagesStruct, instance *instance_model.Instance) ([]*types.NewsletterMessage, error)
}

type newsletterService struct {
	clientPointer map[string]whatsmeow_service.ClientInfo
}

type CreateNewsletterStruct struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type GetNewsletterStruct struct {
	JID types.JID `json:"jid"`
}

type GetNewsletterInviteStruct struct {
	Key string `json:"key"`
}

type GetNewsletterMessagesStruct struct {
	JID      types.JID `json:"jid"`
	Count    int       `json:"count"`
	BeforeID int       `json:"before_id"`
}

func (n *newsletterService) CreateNewsletter(data *CreateNewsletterStruct, instance *instance_model.Instance) (*types.NewsletterMetadata, error) {
	if n.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	newsletter, err := n.clientPointer[instance.Id].WAClient.CreateNewsletter(whatsmeow.CreateNewsletterParams{
		Name:        data.Name,
		Description: data.Description,
	})
	if err != nil {
		logger.LogError("error create newsletter: %v", err)
		return nil, err
	}

	return newsletter, nil
}

func (n *newsletterService) ListNewsletter(instance *instance_model.Instance) ([]*types.NewsletterMetadata, error) {
	if n.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	newsletters, err := n.clientPointer[instance.Id].WAClient.GetSubscribedNewsletters()
	if err != nil {
		logger.LogError("error list newsletters: %v", err)
		return nil, err
	}

	return newsletters, nil
}

func (n *newsletterService) GetNewsletter(data *GetNewsletterStruct, instance *instance_model.Instance) (*types.NewsletterMetadata, error) {
	if n.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	newsletter, err := n.clientPointer[instance.Id].WAClient.GetNewsletterInfo(data.JID)
	if err != nil {
		logger.LogError("error list newsletter: %v", err)
		return nil, err
	}

	return newsletter, nil
}

func (n *newsletterService) GetNewsletterInvite(data *GetNewsletterInviteStruct, instance *instance_model.Instance) (*types.NewsletterMetadata, error) {
	if n.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	newsletter, err := n.clientPointer[instance.Id].WAClient.GetNewsletterInfoWithInvite(data.Key)
	if err != nil {
		logger.LogError("error list newsletter: %v", err)
		return nil, err
	}

	return newsletter, nil
}

func (n *newsletterService) SubscribeNewsletter(data *GetNewsletterStruct, instance *instance_model.Instance) error {
	if n.clientPointer[instance.Id].WAClient == nil {
		return errors.New("no session found")
	}

	_, err := n.clientPointer[instance.Id].WAClient.NewsletterSubscribeLiveUpdates(context.TODO(), data.JID)
	if err != nil {
		logger.LogError("error list newsletter: %v", err)
		return err
	}

	return nil
}

func (n *newsletterService) GetNewsletterMessages(data *GetNewsletterMessagesStruct, instance *instance_model.Instance) ([]*types.NewsletterMessage, error) {
	if n.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	messages, err := n.clientPointer[instance.Id].WAClient.GetNewsletterMessages(data.JID,
		&whatsmeow.GetNewsletterMessagesParams{
			Count: data.Count, Before: data.BeforeID,
		})
	if err != nil {
		logger.LogError("error list newsletter: %v", err)
		return nil, err
	}

	return messages, nil
}

func NewNewsletterService(
	clientPointer map[string]whatsmeow_service.ClientInfo,
) NewsletterService {
	return &newsletterService{
		clientPointer: clientPointer,
	}
}
