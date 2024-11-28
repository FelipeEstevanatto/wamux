package user_service

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	"github.com/EvolutionAPI/evolution-go/pkg/utils"
	whatsmeow_service "github.com/EvolutionAPI/evolution-go/pkg/whatsmeow/service"
	"github.com/gomessguii/logger"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type UserService interface {
	GetUser(data *CheckUserStruct, instance *instance_model.Instance) (*UserCollection, error)
	CheckUser(data *CheckUserStruct, instance *instance_model.Instance) (*CheckUserCollection, error)
	GetAvatar(data *GetAvatarStruct, instance *instance_model.Instance) (*types.ProfilePictureInfo, error)
	GetContacts(instance *instance_model.Instance) ([]ContactInfo, error)
	GetPrivacy(instance *instance_model.Instance) (types.PrivacySettings, error)
	SetPrivacy(data *PrivacyStruct, instance *instance_model.Instance) (*types.PrivacySettings, error)
	BlockContact(data *BlockStruct, instance *instance_model.Instance) (*types.Blocklist, error)
	UnlockContact(data *BlockStruct, instance *instance_model.Instance) (*types.Blocklist, error)
	GetBlockList(instance *instance_model.Instance) (*types.Blocklist, error)
	SetProfilePicture(data *SetProfilePictureStruct, instance *instance_model.Instance) (bool, error)
	SetProfileName(data *SetProfileNameStruct, instance *instance_model.Instance) (bool, error)
	SetProfileStatus(data *SetProfileStatusStruct, instance *instance_model.Instance) (bool, error)
}

type userService struct {
	clientPointer    map[string]*whatsmeow.Client
	whatsmeowService whatsmeow_service.WhatsmeowService
}

type ContactInfo struct {
	Jid          string `json:"Jid"`
	Found        bool   `json:"Found"`
	FirstName    string `json:"FirstName"`
	FullName     string `json:"FullName"`
	PushName     string `json:"PushName"`
	BusinessName string `json:"BusinessName"`
}

type UserCollection struct {
	Users map[types.JID]types.UserInfo
}

type User struct {
	Query        string
	IsInWhatsapp bool
	JID          string
	VerifiedName string
}

type CheckUserCollection struct {
	Users []User
}

type CheckUserStruct struct {
	Number []string `json:"number"`
}

type GetAvatarStruct struct {
	Number  string `json:"number"`
	Preview bool   `json:"preview"`
}

type BlockStruct struct {
	Number string `json:"number"`
}

type SetProfilePictureStruct struct {
	Image string `json:"image"`
}

type SetProfileNameStruct struct {
	Name string `json:"name"`
}

type SetProfileStatusStruct struct {
	Status string `json:"status"`
}

type PrivacyStruct struct {
	GroupAdd     types.PrivacySetting `json:"groupAdd"`
	LastSeen     types.PrivacySetting `json:"lastSeen"`
	Status       types.PrivacySetting `json:"status"`
	Profile      types.PrivacySetting `json:"profile"`
	ReadReceipts types.PrivacySetting `json:"readReceipts"`
	CallAdd      types.PrivacySetting `json:"callAdd"`
	Online       types.PrivacySetting `json:"online"`
}

func (u *userService) GetUser(data *CheckUserStruct, instance *instance_model.Instance) (*UserCollection, error) {
	if u.clientPointer[instance.Id] == nil {
		return nil, errors.New("no session found")
	}

	var jids []types.JID
	for _, arg := range data.Number {
		jid, ok := utils.ParseJID(arg)
		if !ok {
			return nil, errors.New("invalid phone number")
		}
		jids = append(jids, jid)
	}
	resp, err := u.clientPointer[instance.Id].GetUserInfo(jids)
	if err != nil {
		return nil, err
	}

	uc := new(UserCollection)
	uc.Users = make(map[types.JID]types.UserInfo)

	for jid, info := range resp {
		uc.Users[jid] = info
	}

	return uc, nil
}

func (u *userService) CheckUser(data *CheckUserStruct, instance *instance_model.Instance) (*CheckUserCollection, error) {
	if u.clientPointer[instance.Id] == nil {
		return nil, errors.New("no session found")
	}

	resp, err := u.clientPointer[instance.Id].IsOnWhatsApp(data.Number)
	if err != nil {
		return nil, err
	}

	uc := new(CheckUserCollection)
	for _, item := range resp {
		if item.VerifiedName != nil {
			var msg = User{Query: item.Query, IsInWhatsapp: item.IsIn, JID: fmt.Sprintf("%v", item.JID), VerifiedName: item.VerifiedName.Details.GetVerifiedName()}
			uc.Users = append(uc.Users, msg)
		} else {
			var msg = User{Query: item.Query, IsInWhatsapp: item.IsIn, JID: fmt.Sprintf("%v", item.JID), VerifiedName: ""}
			uc.Users = append(uc.Users, msg)
		}
	}

	return uc, nil
}

func (u *userService) GetAvatar(data *GetAvatarStruct, instance *instance_model.Instance) (*types.ProfilePictureInfo, error) {
	if u.clientPointer[instance.Id] == nil {
		return nil, errors.New("no session found")
	}

	jid, ok := utils.ParseJID(data.Number)
	if !ok {
		return nil, errors.New("invalid phone number")
	}

	var pic *types.ProfilePictureInfo

	pic, err := u.clientPointer[instance.Id].GetProfilePictureInfo(jid, &whatsmeow.GetProfilePictureParams{
		Preview: data.Preview,
	})
	if err != nil {
		return nil, err
	}

	if pic == nil {
		return nil, errors.New("no profile picture found")
	}

	logger.LogInfo("[%s] Got avatar %s", instance.Id, pic.URL)

	return pic, nil
}

func (u *userService) GetContacts(instance *instance_model.Instance) ([]ContactInfo, error) {
	if u.clientPointer[instance.Id] == nil {
		return nil, errors.New("no session found")
	}

	contacts, err := u.clientPointer[instance.Id].Store.Contacts.GetAllContacts()
	if err != nil {
		return nil, err
	}

	var contactsArray []ContactInfo

	for jid, contact := range contacts {
		contactsArray = append(contactsArray, ContactInfo{
			Jid:          jid.String(),
			Found:        contact.Found,
			FirstName:    contact.FirstName,
			FullName:     contact.FullName,
			PushName:     contact.PushName,
			BusinessName: contact.BusinessName,
		})
	}

	return contactsArray, nil

}

func (u *userService) GetPrivacy(instance *instance_model.Instance) (types.PrivacySettings, error) {
	if u.clientPointer[instance.Id] == nil {
		return types.PrivacySettings{}, errors.New("no session found")
	}

	privacy := u.clientPointer[instance.Id].GetPrivacySettings()

	return privacy, nil
}

func (u *userService) SetPrivacy(data *PrivacyStruct, instance *instance_model.Instance) (*types.PrivacySettings, error) {
	if u.clientPointer[instance.Id] == nil {
		return nil, errors.New("no session found")
	}

	privacySettings := []struct {
		name  types.PrivacySettingType
		value types.PrivacySetting
	}{
		{types.PrivacySettingTypeGroupAdd, data.GroupAdd},
		{types.PrivacySettingTypeLastSeen, data.LastSeen},
		{types.PrivacySettingTypeStatus, data.Status},
		{types.PrivacySettingTypeProfile, data.Profile},
		{types.PrivacySettingTypeReadReceipts, data.ReadReceipts},
		{types.PrivacySettingTypeCallAdd, data.CallAdd},
		{types.PrivacySettingTypeOnline, data.Online},
	}

	for _, setting := range privacySettings {
		_, err := u.clientPointer[instance.Id].SetPrivacySetting(setting.name, setting.value)
		if err != nil {
			return nil, err
		}
	}

	privacy := u.clientPointer[instance.Id].GetPrivacySettings()

	return &privacy, nil
}

func (u *userService) BlockContact(data *BlockStruct, instance *instance_model.Instance) (*types.Blocklist, error) {
	if u.clientPointer[instance.Id] == nil {
		return nil, errors.New("no session found")
	}

	jid, ok := utils.ParseJID(data.Number)
	if !ok {
		return nil, errors.New("invalid phone number")
	}

	resp, err := u.clientPointer[instance.Id].UpdateBlocklist(jid, events.BlocklistChangeActionBlock)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (u *userService) UnlockContact(data *BlockStruct, instance *instance_model.Instance) (*types.Blocklist, error) {
	if u.clientPointer[instance.Id] == nil {
		return nil, errors.New("no session found")
	}

	jid, ok := utils.ParseJID(data.Number)
	if !ok {
		return nil, errors.New("invalid phone number")
	}

	resp, err := u.clientPointer[instance.Id].UpdateBlocklist(jid, events.BlocklistChangeActionUnblock)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (u *userService) GetBlockList(instance *instance_model.Instance) (*types.Blocklist, error) {
	if u.clientPointer[instance.Id] == nil {
		return nil, errors.New("no session found")
	}

	resp, err := u.clientPointer[instance.Id].GetBlocklist()
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (u *userService) SetProfilePicture(data *SetProfilePictureStruct, instance *instance_model.Instance) (bool, error) {
	if u.clientPointer[instance.Id] == nil {
		return false, errors.New("no session found")
	}

	var filedata []byte

	resp, err := http.Get(data.Image)
	if err != nil {
		return false, fmt.Errorf("failed to fetch image from URL: %v", err)
	}
	defer resp.Body.Close()

	filedata, err = io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read image data: %v", err)
	}

	_, err = u.clientPointer[instance.Id].SetGroupPhoto(types.EmptyJID, filedata)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (u *userService) SetProfileName(data *SetProfileNameStruct, instance *instance_model.Instance) (bool, error) {
	if u.clientPointer[instance.Id] == nil {
		return false, errors.New("no session found")
	}

	err := u.clientPointer[instance.Id].SetGroupName(types.EmptyJID, data.Name)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (u *userService) SetProfileStatus(data *SetProfileStatusStruct, instance *instance_model.Instance) (bool, error) {
	if u.clientPointer[instance.Id] == nil {
		return false, errors.New("no session found")
	}

	err := u.clientPointer[instance.Id].SetStatusMessage(data.Status)
	if err != nil {
		return false, err
	}

	return true, nil
}

func NewUserService(
	clientPointer map[string]*whatsmeow.Client,
	whatsmeowService whatsmeow_service.WhatsmeowService,
) UserService {
	return &userService{
		clientPointer:    clientPointer,
		whatsmeowService: whatsmeowService,
	}
}
