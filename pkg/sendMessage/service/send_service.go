package send_service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
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
		if mimeType != "audio/ogg" {
			errMsg := fmt.Sprintf("Invalid file format: '%s'. Only 'audio/ogg' and 'application/ogg' are accepted", mimeType)
			return "", "", errors.New(errMsg)
		}
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

	waveforms := [][]byte{
		{249, 221, 2, 102, 248, 229, 211, 45, 117, 106, 107, 213, 221, 10, 139, 146, 161, 117, 202, 35, 53, 71, 98, 183, 189, 170, 188, 187, 174, 29, 209, 188, 253, 200, 5, 56, 31, 232, 152, 65, 147, 137, 234, 85, 47, 110, 61, 191, 30, 243, 202, 71, 29, 2, 142, 201, 30, 195, 130, 71, 61, 5, 90, 198, 35, 208, 164, 210, 185, 253, 221, 40, 128, 43, 14, 13, 76, 126, 118, 225, 2, 253, 83, 104, 72, 28, 141, 87, 71, 227, 207, 105, 16, 28, 111, 67, 21, 95, 146, 106},
		{207, 79, 199, 5, 7, 82, 116, 174, 55, 147, 248, 156, 25, 116, 43, 239, 145, 108, 6, 10, 231, 235, 36, 242, 155, 226, 99, 41, 195, 248, 108, 145, 39, 55, 79, 234, 63, 238, 213, 230, 157, 101, 54, 70, 85, 69, 168, 174, 91, 183, 212, 238, 232, 128, 48, 189, 91, 215, 213, 130, 210, 105, 23, 23, 12, 148, 191, 6, 49, 16, 201, 7, 122, 32, 69, 123, 84, 108, 248, 251, 193, 5, 119, 139, 222, 204, 6, 34, 29, 79, 87, 55, 102, 18, 19, 123, 176, 176, 116, 59},
		{33, 186, 143, 170, 23, 6, 35, 215, 103, 106, 147, 137, 112, 128, 156, 190, 158, 203, 200, 92, 246, 130, 14, 84, 231, 122, 108, 16, 194, 252, 32, 187, 107, 95, 19, 190, 69, 29, 8, 207, 245, 71, 97, 134, 158, 175, 90, 62, 28, 74, 189, 81, 210, 15, 178, 157, 150, 149, 111, 73, 120, 72, 254, 234, 36, 204, 123, 104, 255, 183, 6, 147, 236, 211, 57, 42, 35, 5, 191, 106, 58, 17, 47, 148, 7, 134, 209, 72, 237, 89, 114, 150, 213, 141, 120, 25, 225, 32, 201, 114},
		{135, 59, 36, 38, 48, 14, 24, 28, 69, 221, 52, 26, 242, 163, 120, 117, 127, 161, 171, 189, 24, 219, 43, 143, 111, 106, 164, 83, 147, 91, 248, 123, 56, 122, 130, 241, 230, 20, 209, 198, 62, 239, 229, 127, 196, 125, 189, 14, 246, 134, 42, 43, 47, 6, 124, 24, 206, 143, 41, 18, 206, 46, 140, 214, 89, 3, 95, 6, 144, 54, 83, 218, 206, 28, 53, 189, 192, 185, 245, 141, 113, 50, 222, 142, 246, 45, 44, 233, 60, 233, 66, 70, 86, 209, 78, 54, 87, 127, 72, 92},
		{89, 157, 62, 124, 147, 97, 2, 150, 242, 133, 118, 180, 169, 237, 10, 17, 211, 161, 161, 162, 245, 91, 243, 86, 194, 248, 63, 129, 11, 63, 209, 67, 84, 252, 53, 252, 19, 44, 88, 42, 111, 172, 81, 129, 37, 147, 97, 238, 237, 209, 173, 211, 156, 104, 82, 222, 103, 216, 242, 126, 65, 204, 253, 210, 165, 15, 122, 228, 59, 117, 10, 159, 175, 130, 171, 58, 35, 5, 241, 49, 72, 94, 131, 130, 84, 237, 152, 62, 7, 119, 106, 234, 89, 35, 76, 139, 132, 26, 84, 170},
		{109, 254, 35, 233, 37, 217, 181, 122, 209, 107, 150, 246, 52, 243, 91, 105, 70, 43, 249, 171, 20, 169, 85, 114, 197, 99, 224, 75, 1, 4, 199, 33, 200, 183, 131, 152, 79, 55, 95, 10, 204, 15, 142, 0, 134, 7, 11, 204, 103, 245, 168, 238, 197, 23, 6, 116, 56, 136, 105, 76, 4, 107, 83, 54, 193, 117, 212, 79, 244, 148, 217, 217, 63, 127, 49, 56, 16, 242, 64, 51, 226, 112, 182, 233, 7, 199, 133, 33, 206, 126, 56, 35, 165, 190, 96, 230, 152, 120, 1, 138},
		{149, 140, 254, 15, 5, 187, 82, 170, 120, 151, 246, 129, 83, 252, 144, 127, 15, 12, 51, 246, 2, 4, 16, 223, 156, 190, 224, 86, 109, 187, 11, 71, 76, 147, 152, 230, 211, 144, 100, 42, 219, 78, 186, 100, 18, 244, 193, 130, 38, 58, 228, 27, 186, 141, 10, 62, 16, 124, 64, 255, 205, 100, 168, 250, 129, 140, 23, 227, 81, 140, 178, 164, 53, 118, 148, 220, 81, 243, 4, 122, 42, 99, 203, 4, 86, 155, 163, 207, 120, 8, 152, 32, 244, 219, 217, 226, 113, 149, 176, 183},
		{26, 78, 203, 92, 116, 186, 21, 231, 203, 147, 44, 126, 231, 170, 176, 150, 49, 1, 177, 78, 80, 131, 32, 180, 136, 190, 252, 220, 116, 116, 127, 226, 58, 71, 52, 52, 149, 156, 33, 102, 30, 15, 85, 190, 192, 2, 109, 210, 205, 133, 27, 131, 235, 121, 35, 153, 77, 228, 106, 169, 26, 173, 123, 200, 71, 169, 239, 182, 6, 24, 99, 138, 218, 4, 96, 196, 250, 1, 217, 64, 171, 150, 195, 250, 83, 149, 38, 176, 163, 166, 127, 109, 95, 240, 194, 5, 237, 247, 242, 68},
		{149, 165, 47, 252, 245, 239, 238, 220, 15, 58, 121, 126, 188, 76, 71, 81, 4, 110, 89, 54, 26, 188, 230, 85, 18, 70, 122, 45, 37, 34, 63, 58, 243, 178, 166, 195, 204, 208, 76, 21, 136, 47, 243, 1, 65, 170, 223, 11, 97, 230, 74, 89, 120, 186, 234, 140, 213, 110, 166, 165, 197, 226, 193, 246, 189, 1, 38, 85, 49, 223, 46, 73, 180, 197, 50, 41, 124, 30, 103, 54, 56, 94, 226, 178, 83, 206, 148, 55, 179, 145, 226, 67, 249, 2, 10, 118, 126, 121, 192, 52},
		{70, 164, 150, 93, 212, 174, 207, 105, 47, 8, 126, 254, 73, 223, 97, 92, 71, 28, 102, 176, 85, 9, 16, 12, 74, 169, 47, 100, 35, 172, 109, 50, 9, 179, 174, 148, 250, 169, 125, 157, 47, 182, 144, 97, 82, 176, 83, 190, 105, 254, 134, 181, 215, 41, 43, 43, 70, 214, 180, 97, 150, 9, 183, 72, 182, 186, 45, 253, 141, 44, 177, 32, 204, 93, 136, 161, 134, 82, 27, 71, 157, 53, 128, 163, 28, 60, 94, 204, 222, 98, 231, 86, 26, 170, 108, 158, 163, 251, 205, 19},
	}

	chosenIndex := rand.Intn(len(waveforms))
	selectedWaveform := waveforms[chosenIndex]

	numElementsToChange := int(float64(len(selectedWaveform)) * 0.3)
	for i := 0; i < numElementsToChange; i++ {
		randomIndex := rand.Intn(len(selectedWaveform))
		selectedWaveform[randomIndex] = byte(rand.Intn(251) + 3)
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
			URL:           proto.String(uploaded.URL),
			PTT:           proto.Bool(true),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String("audio/ogg; codecs=opus"),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(fileData))),
			Seconds:       proto.Uint32(uint32(data.Duration)),
			Waveform:      selectedWaveform,
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
