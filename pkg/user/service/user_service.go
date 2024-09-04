package user_service

import (
	"errors"
	"fmt"

	instance_model "github.com/Zapbox-API/evolution-go/pkg/instance/model"
	"github.com/Zapbox-API/evolution-go/pkg/utils"
	whatsmeow_service "github.com/Zapbox-API/evolution-go/pkg/whatsmeow/service"
	"github.com/gomessguii/logger"
	"github.com/vincent-petithory/dataurl"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type UserService interface {
	GetUser(data *CheckUserStruct, instance *instance_model.Instance) (*UserCollection, error)
	CheckUser(data *CheckUserStruct, instance *instance_model.Instance) (*CheckUserCollection, error)
	GetAvatar(data *GetAvatarStruct, instance *instance_model.Instance) (*types.ProfilePictureInfo, error)
	GetContacts(instance *instance_model.Instance) (map[types.JID]types.ContactInfo, error)
	GetPrivacy(instance *instance_model.Instance) (map[string]interface{}, error)
	BlockContact(data *BlockStruct, instance *instance_model.Instance) (*types.Blocklist, error)
	UnlockContact(data *BlockStruct, instance *instance_model.Instance) (*types.Blocklist, error)
	GetBlockList(instance *instance_model.Instance) (*types.Blocklist, error)
	SetProfilePicture(data *SetProfilePictureStruct, instance *instance_model.Instance) (bool, error)
}

type userService struct {
	clientPointer    map[string]whatsmeow_service.ClientInfo
	whatsmeowService whatsmeow_service.WhatsmeowService
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
	Phone []string `json:"phone"`
}

type GetAvatarStruct struct {
	Phone   string `json:"phone"`
	Preview bool   `json:"preview"`
}

type BlockStruct struct {
	Phone string `json:"phone"`
}

type SetProfilePictureStruct struct {
	Image string `json:"image"`
}

func (u *userService) GetUser(data *CheckUserStruct, instance *instance_model.Instance) (*UserCollection, error) {
	if u.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	var jids []types.JID
	for _, arg := range data.Phone {
		jid, ok := utils.ParseJID(arg)
		if !ok {
			return nil, errors.New("invalid phone number")
		}
		jids = append(jids, jid)
	}
	resp, err := u.clientPointer[instance.Id].WAClient.GetUserInfo(jids)
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
	if u.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	resp, err := u.clientPointer[instance.Id].WAClient.IsOnWhatsApp(data.Phone)
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
	if u.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	jid, ok := utils.ParseJID(data.Phone)
	if !ok {
		return nil, errors.New("invalid phone number")
	}

	var pic *types.ProfilePictureInfo

	pic, err := u.clientPointer[instance.Id].WAClient.GetProfilePictureInfo(jid, &whatsmeow.GetProfilePictureParams{
		Preview: data.Preview,
	})
	if err != nil {
		return nil, err
	}

	if pic == nil {
		return nil, errors.New("no profile picture found")
	}

	logger.LogInfo("Got avatar %s", pic.URL)

	return pic, nil
}

func (u *userService) GetContacts(instance *instance_model.Instance) (map[types.JID]types.ContactInfo, error) {
	if u.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	result := map[types.JID]types.ContactInfo{}
	result, err := u.clientPointer[instance.Id].WAClient.Store.Contacts.GetAllContacts()
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (u *userService) GetPrivacy(instance *instance_model.Instance) (map[string]interface{}, error) {
	if u.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	privacy := u.clientPointer[instance.Id].WAClient.GetPrivacySettings()
	response := map[string]interface{}{"Data": privacy}

	return response, nil
}

func (u *userService) BlockContact(data *BlockStruct, instance *instance_model.Instance) (*types.Blocklist, error) {
	if u.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	jid, ok := utils.ParseJID(data.Phone)
	if !ok {
		return nil, errors.New("invalid phone number")
	}

	resp, err := u.clientPointer[instance.Id].WAClient.UpdateBlocklist(jid, events.BlocklistChangeActionBlock)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (u *userService) UnlockContact(data *BlockStruct, instance *instance_model.Instance) (*types.Blocklist, error) {
	if u.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	jid, ok := utils.ParseJID(data.Phone)
	if !ok {
		return nil, errors.New("invalid phone number")
	}

	resp, err := u.clientPointer[instance.Id].WAClient.UpdateBlocklist(jid, events.BlocklistChangeActionUnblock)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (u *userService) GetBlockList(instance *instance_model.Instance) (*types.Blocklist, error) {
	if u.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	resp, err := u.clientPointer[instance.Id].WAClient.GetBlocklist()
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (u *userService) SetProfilePicture(data *SetProfilePictureStruct, instance *instance_model.Instance) (bool, error) {
	if u.clientPointer[instance.Id].WAClient == nil {
		return false, errors.New("no session found")
	}

	var filedata []byte

	if data.Image[0:10] == "data:image" {
		dataURL, err := dataurl.DecodeString(data.Image)
		if err != nil {
			return false, err
		} else {
			filedata = dataURL.Data
		}
	} else {
		return false, errors.New("image data should start with \"data:image/png;base64,\"")
	}

	_, err := u.clientPointer[instance.Id].WAClient.SetGroupPhoto(types.EmptyJID, filedata)
	if err != nil {
		return false, err
	}

	return true, nil
}

func NewUserService(
	clientPointer map[string]whatsmeow_service.ClientInfo,
	whatsmeowService whatsmeow_service.WhatsmeowService,
) UserService {
	return &userService{
		clientPointer:    clientPointer,
		whatsmeowService: whatsmeowService,
	}
}
