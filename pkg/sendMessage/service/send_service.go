package send_service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"time"

	instance_model "github.com/Zapbox-API/evolution-go/pkg/instance/model"
	"github.com/Zapbox-API/evolution-go/pkg/utils"
	whatsmeow_service "github.com/Zapbox-API/evolution-go/pkg/whatsmeow/service"
	"github.com/gabriel-vasile/mimetype"
	"github.com/gomessguii/logger"
	"github.com/vincent-petithory/dataurl"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type SendService interface {
	SendText(data *TextStruct, instance *instance_model.Instance) (string, string, error)
	SendLink(data *LinkStruct, instance *instance_model.Instance) (string, string, error)
	SendMediaUrl(data *MediaStruct, instance *instance_model.Instance) (string, string, error)
	SendPoll(data *PollStruct, instance *instance_model.Instance) (string, string, error)
	SendSticker(data *StickerStruct, instance *instance_model.Instance) (string, string, error)
	SendLocation(data *LocationStruct, instance *instance_model.Instance) (string, string, error)
	SendContact(data *ContactStruct, instance *instance_model.Instance) (string, string, error)
	SendList(data *ListStruct, instance *instance_model.Instance) (string, string, error)
}

type sendService struct {
	clientPointer    map[int]whatsmeow_service.ClientInfo
	whatsmeowService whatsmeow_service.WhatsmeowService
}

type TextStruct struct {
	Phone       string            `json:"phone"`
	Text        string            `json:"text"`
	Id          string            `json:"id"`
	ContextInfo waE2E.ContextInfo `json:"contextInfo"`
}

type LinkStruct struct {
	Phone       string            `json:"phone"`
	Text        string            `json:"text"`
	Title       string            `json:"title"`
	Url         string            `json:"url"`
	Description string            `json:"description"`
	ImgUrl      string            `json:"imgUrl"`
	Id          string            `json:"id"`
	ContextInfo waE2E.ContextInfo `json:"contextInfo"`
}

type MediaStruct struct {
	Phone       string            `json:"phone"`
	Url         string            `json:"url"`
	Type        string            `json:"type"`
	Duration    int32             `json:"duration"`
	Caption     string            `json:"caption"`
	Filename    string            `json:"filename"`
	Id          string            `json:"id"`
	ContextInfo waE2E.ContextInfo `json:"contextInfo"`
}

type PollStruct struct {
	Phone       string            `json:"phone"`
	Question    string            `json:"question"`
	MaxAnswer   int               `json:"maxAnswer"`
	Options     []string          `json:"options"`
	ContextInfo waE2E.ContextInfo `json:"contextInfo"`
}

type StickerStruct struct {
	Phone        string            `json:"phone"`
	Sticker      string            `json:"sticker"`
	Id           string            `json:"id"`
	PngThumbnail string            `json:"pngThumbnail"`
	ContextInfo  waE2E.ContextInfo `json:"contextInfo"`
}

type LocationStruct struct {
	Phone       string            `json:"phone"`
	Id          string            `json:"id"`
	Name        string            `json:"name"`
	Latitude    float64           `json:"latitude"`
	Longitude   float64           `json:"longitude"`
	ContextInfo waE2E.ContextInfo `json:"contextInfo"`
}

type ContactStruct struct {
	Phone       string            `json:"phone"`
	Id          string            `json:"id"`
	Vcard       utils.VCardStruct `json:"vcard"`
	ContextInfo waE2E.ContextInfo `json:"contextInfo"`
}

type listStruct struct {
	Title string `json:"title"`
	Desc  string `json:"desc"`
	RowId string `json:"rowId"`
}

type ListStruct struct {
	Phone       string `json:"phone"`
	Id          string `json:"id"`
	ButtonText  string `json:"buttonText"`
	Desc        string `json:"desc"`
	TopText     string `json:"topText"`
	List        []listStruct
	ContextInfo waE2E.ContextInfo `json:"contextInfo"`
}

func validateMessageFields(phone string, stanzaID *string, participant *string) (types.JID, error) {

	recipient, ok := utils.ParseJID(phone)
	if !ok {
		return types.NewJID("", types.DefaultUserServer), errors.New("could not parse phone")
	}

	if stanzaID != nil {
		if participant == nil {
			return types.NewJID("", types.DefaultUserServer), errors.New("missing participant in contextinfo")
		}
	}

	if participant != nil {
		if stanzaID == nil {
			return types.NewJID("", types.DefaultUserServer), errors.New("missing stanzaid in contextinfo")
		}
	}

	return recipient, nil
}

func findURL(text string) string {
	urlRegex := `http[s]?://(?:[a-zA-Z]|[0-9]|[$-_@.&+]|[!*\\(\\),]|(?:%[0-9a-fA-F][0-9a-fA-F]))+`
	re := regexp.MustCompile(urlRegex)
	urls := re.FindAllString(text, -1)
	if len(urls) > 0 {
		return urls[0]
	}
	return ""
}

func (s *sendService) SendText(data *TextStruct, instance *instance_model.Instance) (string, string, error) {
	if s.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	var ts time.Time

	recipient, err := validateMessageFields(data.Phone, data.ContextInfo.StanzaID, data.ContextInfo.Participant)
	if err != nil {
		logger.LogError("Error validating message fields: %v", err)
		return "", "", err
	}

	msgId := s.clientPointer[instance.Id].WAClient.GenerateMessageID()

	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &data.Text,
		},
	}

	if data.ContextInfo.StanzaID != nil {
		msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      proto.String(*data.ContextInfo.StanzaID),
			Participant:   proto.String(*data.ContextInfo.Participant),
			QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
		}
	}

	_, err = s.clientPointer[instance.Id].WAClient.SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{
		ID: msgId,
	})
	if err != nil {
		return "", "", err
	}

	logger.LogInfo("Message sent to %s", data.Phone)

	return msgId, ts.String(), nil
}

func (s *sendService) SendLink(data *LinkStruct, instance *instance_model.Instance) (string, string, error) {
	if s.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	var ts time.Time

	recipient, err := validateMessageFields(data.Phone, data.ContextInfo.StanzaID, data.ContextInfo.Participant)
	if err != nil {
		logger.LogError("Error validating message fields: %v", err)
		return "", "", err
	}

	msgId := s.clientPointer[instance.Id].WAClient.GenerateMessageID()

	var fileData []byte
	if data.ImgUrl != "" {
		resp, err := http.Get(data.ImgUrl)
		if err != nil {
			return "", "", err
		}
		defer resp.Body.Close()
		fileData, _ = io.ReadAll(resp.Body)
	}

	matchedText := findURL(data.Text)

	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:          &data.Text,
			Title:         &data.Title,
			CanonicalURL:  &data.Url,
			MatchedText:   &matchedText,
			JPEGThumbnail: fileData,
			Description:   &data.Description,
		},
	}

	if data.ContextInfo.StanzaID != nil {
		msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      proto.String(*data.ContextInfo.StanzaID),
			Participant:   proto.String(*data.ContextInfo.Participant),
			QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
		}
	}

	_, err = s.clientPointer[instance.Id].WAClient.SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{
		ID: msgId,
	})
	if err != nil {
		return "", "", err
	}

	logger.LogInfo("Message sent to %s", data.Phone)

	return msgId, ts.String(), nil
}

func convertAudioToOpus(inputData []byte) ([]byte, error) {
	cmd := exec.Command("ffmpeg", "-i", "pipe:0", "-ac", "1", "-ar", "16000", "-c:a", "libopus", "-f", "ogg", "pipe:1")

	var outBuffer bytes.Buffer
	var errBuffer bytes.Buffer

	cmd.Stdin = bytes.NewReader(inputData)
	cmd.Stdout = &outBuffer
	cmd.Stderr = &errBuffer

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error during conversion: %v, details: %s", err, errBuffer.String())
	}

	convertedData := outBuffer.Bytes()

	return convertedData, nil
}

func (s *sendService) SendMediaUrl(data *MediaStruct, instance *instance_model.Instance) (string, string, error) {
	if s.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	var ts time.Time

	recipient, err := validateMessageFields(data.Phone, data.ContextInfo.StanzaID, data.ContextInfo.Participant)
	if err != nil {
		logger.LogError("Error validating message fields: %v", err)
		return "", "", err
	}

	msgId := s.clientPointer[instance.Id].WAClient.GenerateMessageID()

	var uploaded whatsmeow.UploadResponse
	var fileData []byte

	resp, err := http.Get(data.Url)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	fileData, err = io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	mime, _ := mimetype.DetectReader(bytes.NewReader(fileData))

	mimeType := mime.String()

	var uploadType whatsmeow.MediaType

	if data.Type == "image" {
		if mimeType != "image/jpeg" && mimeType != "image/png" {
			errMsg := fmt.Sprintf("Invalid file format: '%s'. Only 'image/jpeg' and 'image/png' are accepted", mimeType)
			return "", "", errors.New(errMsg)
		}
		uploadType = whatsmeow.MediaImage
	} else if data.Type == "video" {
		if mimeType != "video/mp4" {
			errMsg := fmt.Sprintf("Invalid file format: '%s'. Only 'video/mp4' are accepted", mimeType)
			return "", "", errors.New(errMsg)
		}
		uploadType = whatsmeow.MediaVideo
	} else if data.Type == "audio" {
		convertedData, err := convertAudioToOpus(fileData)
		if err != nil {
			return "", "", err
		}
		fileData = convertedData
		mimeType = "audio/ogg"
		uploadType = whatsmeow.MediaAudio
	} else if data.Type == "document" {
		uploadType = whatsmeow.MediaDocument
	} else {
		return "", "", errors.New("invalid media type")
	}

	uploaded, err = s.clientPointer[instance.Id].WAClient.Upload(context.Background(), fileData, uploadType)
	if err != nil {
		return "", "", err
	}

	var media *waE2E.Message

	switch data.Type {
	case "image":
		media = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			Caption:       proto.String(data.Caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(fileData))),
		}}
	case "video":
		media = &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
			Caption:       proto.String(data.Caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(fileData))),
		}}
	case "audio":
		media = &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
			URL:              proto.String(uploaded.URL),
			PTT:              proto.Bool(true),
			DirectPath:       proto.String(uploaded.DirectPath),
			MediaKey:         uploaded.MediaKey,
			Mimetype:         proto.String("audio/ogg; codecs=opus"),
			FileEncSHA256:    uploaded.FileEncSHA256,
			FileSHA256:       uploaded.FileSHA256,
			FileLength:       proto.Uint64(uint64(len(fileData))),
			StreamingSidecar: []byte(*proto.String("QpmXDsU7YLagdg==")),
			Waveform:         []byte(*proto.String("OjAnExISDgsKCAkJBwgkHAQEBBEFAwMNAxAcKCgkFzM0QUE4Jh4eKAoKChcLCwkeFgkJCQo3JiQmIiIRPz8/Ow==")),
		}}
	case "document":
		media = &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			FileName:      &data.Filename,
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(fileData))),
		}}
	default:
		return "", "", errors.New("invalid media type")
	}

	if data.ContextInfo.StanzaID != nil && data.ContextInfo.Participant != nil {
		contextInfo := &waE2E.ContextInfo{
			StanzaID:      proto.String(*data.ContextInfo.StanzaID),
			Participant:   proto.String(*data.ContextInfo.Participant),
			QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
		}
		switch data.Type {
		case "image":
			media.GetImageMessage().ContextInfo = contextInfo
		case "video":
			media.GetVideoMessage().ContextInfo = contextInfo
		case "audio":
			media.GetAudioMessage().ContextInfo = contextInfo
		case "document":
			media.GetDocumentMessage().ContextInfo = contextInfo
		}
	}

	_, err = s.clientPointer[instance.Id].WAClient.SendMessage(context.Background(), recipient, media, whatsmeow.SendRequestExtra{
		ID: msgId,
	})
	if err != nil {
		return "", "", err
	}

	logger.LogInfo("Message sent to %s", data.Phone)

	return msgId, ts.String(), nil
}

func (s *sendService) SendPoll(data *PollStruct, instance *instance_model.Instance) (string, string, error) {
	if s.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Phone)
	if !ok {
		return "", "", errors.New("could not parse phone")
	}

	msgId := s.clientPointer[instance.Id].WAClient.GenerateMessageID()

	_, err := s.clientPointer[instance.Id].WAClient.SendMessage(context.Background(), recipient,
		s.clientPointer[instance.Id].WAClient.BuildPollCreation(data.Question, data.Options, data.MaxAnswer), whatsmeow.SendRequestExtra{
			ID: msgId,
		})

	if err != nil {
		return "", "", err
	}

	logger.LogInfo("Message sent to %s", data.Phone)

	return msgId, ts.String(), nil
}

func (s *sendService) SendSticker(data *StickerStruct, instance *instance_model.Instance) (string, string, error) {
	if s.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	var ts time.Time

	recipient, err := validateMessageFields(data.Phone, data.ContextInfo.StanzaID, data.ContextInfo.Participant)
	if err != nil {
		logger.LogError("Error validating message fields: %v", err)
		return "", "", err
	}

	var msgId string

	if data.Id == "" {
		msgId = s.clientPointer[instance.Id].WAClient.GenerateMessageID()
	} else {
		msgId = data.Id
	}

	var uploaded whatsmeow.UploadResponse
	var filedata []byte

	if data.Sticker[0:4] == "data" {
		dataURL, err := dataurl.DecodeString(data.Sticker)
		if err != nil {
			return "", "", err
		} else {
			filedata = dataURL.Data
			uploaded, err = s.clientPointer[instance.Id].WAClient.Upload(context.Background(), filedata, whatsmeow.MediaImage)
			if err != nil {
				return "", "", err
			}
		}
	} else {
		return "", "", fmt.Errorf("data should start with \"data:mime/type;base64,\"")
	}

	msg := &waE2E.Message{StickerMessage: &waE2E.StickerMessage{
		URL:           proto.String(uploaded.URL),
		DirectPath:    proto.String(uploaded.DirectPath),
		MediaKey:      uploaded.MediaKey,
		Mimetype:      proto.String(http.DetectContentType(filedata)),
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    proto.Uint64(uint64(len(filedata))),
	}}

	if data.ContextInfo.StanzaID != nil {
		msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      proto.String(*data.ContextInfo.StanzaID),
			Participant:   proto.String(*data.ContextInfo.Participant),
			QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
		}
	}

	_, err = s.clientPointer[instance.Id].WAClient.SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgId})
	if err != nil {
		return "", "", err
	}

	logger.LogInfo("Message sent to %s", data.Phone)

	return msgId, ts.String(), nil
}

func (s *sendService) SendLocation(data *LocationStruct, instance *instance_model.Instance) (string, string, error) {
	if s.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	var ts time.Time

	recipient, err := validateMessageFields(data.Phone, data.ContextInfo.StanzaID, data.ContextInfo.Participant)
	if err != nil {
		logger.LogError("Error validating message fields: %v", err)
		return "", "", err
	}

	var msgId string

	if data.Id == "" {
		msgId = s.clientPointer[instance.Id].WAClient.GenerateMessageID()
	} else {
		msgId = data.Id
	}

	msg := &waE2E.Message{LocationMessage: &waE2E.LocationMessage{
		DegreesLatitude:  &data.Latitude,
		DegreesLongitude: &data.Longitude,
		Name:             &data.Name,
	}}

	if data.ContextInfo.StanzaID != nil {
		msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      proto.String(*data.ContextInfo.StanzaID),
			Participant:   proto.String(*data.ContextInfo.Participant),
			QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
		}
	}

	_, err = s.clientPointer[instance.Id].WAClient.SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgId})
	if err != nil {
		return "", "", err
	}

	logger.LogInfo("Message sent to %s", data.Phone)

	return msgId, ts.String(), nil
}

func (s *sendService) SendContact(data *ContactStruct, instance *instance_model.Instance) (string, string, error) {
	if s.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	VCstring := utils.GenerateVC(utils.VCardStruct{
		FullName:     data.Vcard.FullName,
		Phone:        data.Vcard.Phone,
		Organization: data.Vcard.Organization,
	})

	fmt.Println(VCstring)

	var ts time.Time

	recipient, err := validateMessageFields(data.Phone, data.ContextInfo.StanzaID, data.ContextInfo.Participant)
	if err != nil {
		logger.LogError("Error validating message fields: %v", err)
		return "", "", err
	}

	var msgId string

	if data.Id == "" {
		msgId = s.clientPointer[instance.Id].WAClient.GenerateMessageID()
	} else {
		msgId = data.Id
	}

	msg := &waE2E.Message{ContactMessage: &waE2E.ContactMessage{
		DisplayName: &data.Vcard.FullName,
		Vcard:       &VCstring,
	}}

	if data.ContextInfo.StanzaID != nil {
		msg.ContactMessage.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      proto.String(*data.ContextInfo.StanzaID),
			Participant:   proto.String(*data.ContextInfo.Participant),
			QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
		}
	}

	_, err = s.clientPointer[instance.Id].WAClient.SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgId})
	if err != nil {
		return "", "", err
	}

	logger.LogInfo("Message sent to %s", data.Phone)

	return msgId, ts.String(), nil
}

func (s *sendService) SendList(data *ListStruct, instance *instance_model.Instance) (string, string, error) {
	if s.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	var ts time.Time

	recipient, err := validateMessageFields(data.Phone, data.ContextInfo.StanzaID, data.ContextInfo.Participant)
	if err != nil {
		logger.LogError("Error validating message fields: %v", err)
		return "", "", err
	}

	var msgId string

	if data.Id == "" {
		msgId = s.clientPointer[instance.Id].WAClient.GenerateMessageID()
	} else {
		msgId = data.Id
	}

	list := []*waE2E.ListMessage_Row{}

	for _, v := range data.List {
		list = append(list, &waE2E.ListMessage_Row{
			Title:       proto.String(v.Title),
			Description: proto.String(v.Desc),
			RowID:       proto.String(v.RowId),
		})
	}

	msg := &waE2E.Message{
		ListMessage: &waE2E.ListMessage{
			Description: proto.String(data.Desc),
			ButtonText:  proto.String(data.ButtonText),
			ListType:    waE2E.ListMessage_SINGLE_SELECT.Enum(),
			Sections: []*waE2E.ListMessage_Section{
				{
					Title: proto.String(data.TopText),
					Rows:  list,
				},
			},
		},
	}

	if data.ContextInfo.StanzaID != nil {
		msg.ContactMessage.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      proto.String(*data.ContextInfo.StanzaID),
			Participant:   proto.String(*data.ContextInfo.Participant),
			QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
		}
	}

	_, err = s.clientPointer[instance.Id].WAClient.SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgId})
	if err != nil {
		return "", "", err
	}

	logger.LogInfo("Message sent to %s", data.Phone)

	return msgId, ts.String(), nil
}

func NewSendService(
	clientPointer map[int]whatsmeow_service.ClientInfo,
	whatsmeowService whatsmeow_service.WhatsmeowService,
) SendService {
	return &sendService{
		clientPointer:    clientPointer,
		whatsmeowService: whatsmeowService,
	}
}
