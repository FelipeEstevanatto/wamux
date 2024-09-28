package send_service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	"github.com/EvolutionAPI/evolution-go/pkg/utils"
	whatsmeow_service "github.com/EvolutionAPI/evolution-go/pkg/whatsmeow/service"
	"github.com/chai2010/webp"
	"github.com/gabriel-vasile/mimetype"
	"github.com/gomessguii/logger"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"golang.org/x/net/html"
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
}

type sendService struct {
	clientPointer    map[string]whatsmeow_service.ClientInfo
	whatsmeowService whatsmeow_service.WhatsmeowService
}

type SendDataStruct struct {
	Id           string
	Number       string
	Delay        int32
	MentionAll   bool
	MentionedJID string
	Quoted       QuotedStruct
}

type QuotedStruct struct {
	MessageID   string `json:"messageId"`
	Participant string `json:"participant"`
}

type TextStruct struct {
	Number       string       `json:"number"`
	Text         string       `json:"text"`
	Id           string       `json:"id"`
	Delay        int32        `json:"delay"`
	MentionedJID string       `json:"mentionedJid"`
	MentionAll   bool         `json:"mentionAll"`
	Quoted       QuotedStruct `json:"quoted"`
}

type LinkStruct struct {
	Number       string       `json:"number"`
	Text         string       `json:"text"`
	Title        string       `json:"title"`
	Url          string       `json:"url"`
	Description  string       `json:"description"`
	ImgUrl       string       `json:"imgUrl"`
	Id           string       `json:"id"`
	Delay        int32        `json:"delay"`
	MentionedJID string       `json:"mentionedJid"`
	MentionAll   bool         `json:"mentionAll"`
	Quoted       QuotedStruct `json:"quoted"`
}

type MediaStruct struct {
	Number       string       `json:"number"`
	Url          string       `json:"url"`
	Type         string       `json:"type"`
	Caption      string       `json:"caption"`
	Filename     string       `json:"filename"`
	Id           string       `json:"id"`
	Delay        int32        `json:"delay"`
	MentionedJID string       `json:"mentionedJid"`
	MentionAll   bool         `json:"mentionAll"`
	Quoted       QuotedStruct `json:"quoted"`
}

type PollStruct struct {
	Id           string       `json:"id"`
	Number       string       `json:"number"`
	Question     string       `json:"question"`
	MaxAnswer    int          `json:"maxAnswer"`
	Options      []string     `json:"options"`
	Delay        int32        `json:"delay"`
	MentionedJID string       `json:"mentionedJid"`
	MentionAll   bool         `json:"mentionAll"`
	Quoted       QuotedStruct `json:"quoted"`
}

type StickerStruct struct {
	Number       string       `json:"number"`
	Sticker      string       `json:"sticker"`
	Id           string       `json:"id"`
	Delay        int32        `json:"delay"`
	MentionedJID string       `json:"mentionedJid"`
	MentionAll   bool         `json:"mentionAll"`
	Quoted       QuotedStruct `json:"quoted"`
}

type LocationStruct struct {
	Number       string       `json:"number"`
	Id           string       `json:"id"`
	Name         string       `json:"name"`
	Latitude     float64      `json:"latitude"`
	Longitude    float64      `json:"longitude"`
	Address      string       `json:"address"`
	Delay        int32        `json:"delay"`
	MentionedJID string       `json:"mentionedJid"`
	MentionAll   bool         `json:"mentionAll"`
	Quoted       QuotedStruct `json:"quoted"`
}

type ContactStruct struct {
	Number       string            `json:"number"`
	Id           string            `json:"id"`
	Vcard        utils.VCardStruct `json:"vcard"`
	Delay        int32             `json:"delay"`
	MentionedJID string            `json:"mentionedJid"`
	MentionAll   bool              `json:"mentionAll"`
	Quoted       QuotedStruct      `json:"quoted"`
}

func validateMessageFields(phone string, messageID *string, participant *string) (types.JID, error) {

	recipient, ok := utils.ParseJID(phone)
	if !ok {
		return types.NewJID("", types.DefaultUserServer), errors.New("could not parse phone")
	}

	if messageID != nil {
		if participant == nil {
			return types.NewJID("", types.DefaultUserServer), errors.New("missing Participant in ContextInfo")
		}
	}

	if participant != nil {
		if messageID == nil {
			return types.NewJID("", types.DefaultUserServer), errors.New("missing StanzaId in ContextInfo")
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

	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &data.Text,
		},
	}

	msgId, ts, err := s.SendMessage(instance.Id, msg, "ExtendedTextMessage", &SendDataStruct{
		Id:           data.Id,
		Number:       data.Number,
		Quoted:       data.Quoted,
		Delay:        data.Delay,
		MentionAll:   data.MentionAll,
		MentionedJID: data.MentionedJID,
	})
	if err != nil {
		return "", "", err
	}

	return msgId, ts, nil
}

func fetchLinkMetadata(url string) (string, string, string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return "", "", "", err
	}

	var title, description, imgURL string

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "title" && n.FirstChild != nil {
				title = n.FirstChild.Data
			}
			if n.Data == "meta" {
				var property, content string
				for _, attr := range n.Attr {
					if attr.Key == "property" || attr.Key == "name" {
						property = attr.Val
					}
					if attr.Key == "content" {
						content = attr.Val
					}
				}

				if (property == "description" || property == "og:description") && content != "" {
					description = content
				}

				if property == "og:image" && content != "" {
					imgURL = content
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}

	f(doc)

	return title, description, imgURL, nil
}

func (s *sendService) SendLink(data *LinkStruct, instance *instance_model.Instance) (string, string, error) {
	if s.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	matchedText := findURL(data.Text)

	if matchedText != "" {
		title, description, imgUrl, err := fetchLinkMetadata(matchedText)
		if err != nil {
			return "", "", err
		}

		data.Title = title
		data.Description = description
		data.ImgUrl = imgUrl
	}

	var fileData []byte
	if data.ImgUrl != "" {
		resp, err := http.Get(data.ImgUrl)
		if err != nil {
			return "", "", err
		}
		defer resp.Body.Close()
		fileData, _ = io.ReadAll(resp.Body)
	}

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

	msgId, ts, err := s.SendMessage(instance.Id, msg, "ExtendedTextMessage", &SendDataStruct{
		Id:           data.Id,
		Number:       data.Number,
		Quoted:       data.Quoted,
		Delay:        data.Delay,
		MentionAll:   data.MentionAll,
		MentionedJID: data.MentionedJID,
	})
	if err != nil {
		return "", "", err
	}

	return msgId, ts, nil
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

func getAudioDurationFromBytes(data []byte) (int, error) {
	cmd := exec.Command("ffmpeg", "-i", "pipe:0", "-f", "null", "-")
	cmd.Stdin = bytes.NewReader(data)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, err
	}

	outputText := string(output)

	splitTime := strings.Split(outputText, "time=")

	if len(splitTime) < 2 {
		return 0, nil
	}

	re := regexp.MustCompile(`(\d+):(\d+):(\d+\.\d+)`)
	matches := re.FindStringSubmatch(string(splitTime[2]))
	if len(matches) != 4 {
		return 0, errors.New("formato de duração não encontrado")
	}

	hours, _ := strconv.ParseFloat(matches[1], 64)
	minutes, _ := strconv.ParseFloat(matches[2], 64)
	seconds, _ := strconv.ParseFloat(matches[3], 64)
	duration := int(hours*3600 + minutes*60 + seconds)

	return duration, nil
}

func (s *sendService) SendMediaUrl(data *MediaStruct, instance *instance_model.Instance) (string, string, error) {
	if s.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

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
	var duration int

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
		mimeType = "audio/ogg; codecs=opus"
		uploadType = whatsmeow.MediaAudio
		duration, err = getAudioDurationFromBytes(fileData)
		if err != nil {
			return "", "", err
		}
	} else if data.Type == "document" {
		uploadType = whatsmeow.MediaDocument
	} else {
		return "", "", errors.New("invalid media type")
	}

	uploaded, err = s.clientPointer[instance.Id].WAClient.Upload(context.Background(), fileData, uploadType)
	if err != nil {
		return "", "", err
	}

	logger.LogInfo("Media uploaded with %s", uploaded.FileLength)

	var media *waE2E.Message
	var mediaType string

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

		mediaType = "ImageMessage"
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

		mediaType = "VideoMessage"
	case "audio":
		media = &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
			URL:              proto.String(uploaded.URL),
			PTT:              proto.Bool(true),
			DirectPath:       proto.String(uploaded.DirectPath),
			MediaKey:         uploaded.MediaKey,
			Mimetype:         proto.String(mimeType),
			FileEncSHA256:    uploaded.FileEncSHA256,
			FileSHA256:       uploaded.FileSHA256,
			FileLength:       proto.Uint64(uploaded.FileLength),
			StreamingSidecar: []byte(*proto.String("QpmXDsU7YLagdg==")),
			Waveform:         []byte(*proto.String("OjAnExISDgsKCAkJBwgkHAQEBBEFAwMNAxAcKCgkFzM0QUE4Jh4eKAoKChcLCwkeFgkJCQo3JiQmIiIRPz8/Ow==")),
			Seconds:          proto.Uint32(uint32(duration)),
		}}

		mediaType = "AudioMessage"
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

		mediaType = "DocumentMessage"
	default:
		return "", "", errors.New("invalid media type")
	}

	msgId, ts, err := s.SendMessage(instance.Id, media, mediaType, &SendDataStruct{
		Id:           data.Id,
		Number:       data.Number,
		Quoted:       data.Quoted,
		Delay:        data.Delay,
		MentionAll:   data.MentionAll,
		MentionedJID: data.MentionedJID,
	})
	if err != nil {
		return "", "", err
	}

	return msgId, ts, nil
}

func (s *sendService) SendPoll(data *PollStruct, instance *instance_model.Instance) (string, string, error) {
	if s.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	msg := s.clientPointer[instance.Id].WAClient.BuildPollCreation(data.Question, data.Options, data.MaxAnswer)

	msgId, ts, err := s.SendMessage(instance.Id, msg, "PollCreationMessage", &SendDataStruct{
		Id:           data.Id,
		Number:       data.Number,
		Quoted:       data.Quoted,
		Delay:        data.Delay,
		MentionAll:   data.MentionAll,
		MentionedJID: data.MentionedJID,
	})
	if err != nil {
		return "", "", err
	}

	return msgId, ts, nil
}

func convertToWebP(imageData string) ([]byte, error) {
	var img image.Image
	var err error

	resp, err := http.Get(imageData)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image from URL: %v", err)
	}
	defer resp.Body.Close()

	img, _, err = image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}

	var webpBuffer bytes.Buffer
	err = webp.Encode(&webpBuffer, img, &webp.Options{Lossless: false, Quality: 80})
	if err != nil {
		return nil, fmt.Errorf("failed to encode image to WebP: %v", err)
	}

	return webpBuffer.Bytes(), nil
}

func (s *sendService) SendSticker(data *StickerStruct, instance *instance_model.Instance) (string, string, error) {
	if s.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	var uploaded whatsmeow.UploadResponse
	var filedata []byte

	if strings.HasPrefix(data.Sticker, "http") {
		webpData, err := convertToWebP(data.Sticker)
		if err != nil {
			return "", "", fmt.Errorf("failed to convert image to WebP: %v", err)
		}

		filedata = webpData

		uploaded, err = s.clientPointer[instance.Id].WAClient.Upload(context.Background(), filedata, whatsmeow.MediaImage)
		if err != nil {
			return "", "", fmt.Errorf("failed to upload sticker: %v", err)
		}
	} else {
		return "", "", fmt.Errorf("invalid sticker URL")
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

	msgId, ts, err := s.SendMessage(instance.Id, msg, "StickerMessage", &SendDataStruct{
		Id:           data.Id,
		Number:       data.Number,
		Quoted:       data.Quoted,
		Delay:        data.Delay,
		MentionAll:   data.MentionAll,
		MentionedJID: data.MentionedJID,
	})
	if err != nil {
		return "", "", err
	}

	return msgId, ts, nil
}

func (s *sendService) SendLocation(data *LocationStruct, instance *instance_model.Instance) (string, string, error) {
	if s.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	msg := &waE2E.Message{LocationMessage: &waE2E.LocationMessage{
		DegreesLatitude:  &data.Latitude,
		DegreesLongitude: &data.Longitude,
		Name:             &data.Name,
		Address:          &data.Address,
	}}

	msgId, ts, err := s.SendMessage(instance.Id, msg, "LocationMessage", &SendDataStruct{
		Id:           data.Id,
		Number:       data.Number,
		Quoted:       data.Quoted,
		Delay:        data.Delay,
		MentionAll:   data.MentionAll,
		MentionedJID: data.MentionedJID,
	})
	if err != nil {
		return "", "", err
	}

	return msgId, ts, nil
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

	msg := &waE2E.Message{ContactMessage: &waE2E.ContactMessage{
		DisplayName: &data.Vcard.FullName,
		Vcard:       &VCstring,
	}}

	msgId, ts, err := s.SendMessage(instance.Id, msg, "ContactMessage", &SendDataStruct{
		Id:           data.Id,
		Number:       data.Number,
		Quoted:       data.Quoted,
		Delay:        data.Delay,
		MentionAll:   data.MentionAll,
		MentionedJID: data.MentionedJID,
	})
	if err != nil {
		return "", "", err
	}

	return msgId, ts, nil
}

func (s *sendService) SendMessage(instanceId string, msg *waE2E.Message, messageType string, data *SendDataStruct) (string, string, error) {
	recipient, err := validateMessageFields(data.Number, &data.Quoted.MessageID, &data.Quoted.MessageID)
	if err != nil {
		logger.LogError("Error validating message fields: %v", err)
		return "", "", err
	}

	var msgId string
	if data.Id == "" {
		msgId = s.clientPointer[instanceId].WAClient.GenerateMessageID()
	} else {
		msgId = data.Id
	}

	if data.Delay > 0 {
		media := ""
		if messageType == "AudioMessage" {
			media = "audio"
		}

		err := s.clientPointer[instanceId].WAClient.SendChatPresence(recipient, types.ChatPresence("composing"), types.ChatPresenceMedia(media))
		if err != nil {
			return "", "", err
		}

		time.Sleep(time.Duration(data.Delay) * time.Millisecond)

		err = s.clientPointer[instanceId].WAClient.SendChatPresence(recipient, types.ChatPresence("paused"), types.ChatPresenceMedia(media))
		if err != nil {
			return "", "", err
		}
	}

	if data.Quoted.MessageID != "" {
		switch messageType {
		case "ExtendedTextMessage":
			msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
		case "ImageMessage":
			msg.ImageMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
		case "AudioMessage":
			msg.AudioMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
		case "DocumentMessage":
			msg.DocumentMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
		case "PollCreationMessage":
			msg.PollCreationMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
		case "StickerMessage":
			msg.StickerMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
		case "LocationMessage":
			msg.LocationMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
		case "ContactMessage":
			msg.ContactMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
		default:
			return "", "", fmt.Errorf("invalid messageType: %s", messageType)
		}
	}

	isGroup := strings.Contains(data.Number, "@g.us")
	if isGroup {
		switch messageType {
		case "ExtendedTextMessage":
			msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{}
		case "ImageMessage":
			msg.ImageMessage.ContextInfo = &waE2E.ContextInfo{}
		case "AudioMessage":
			msg.AudioMessage.ContextInfo = &waE2E.ContextInfo{}
		case "DocumentMessage":
			msg.DocumentMessage.ContextInfo = &waE2E.ContextInfo{}
		case "PollCreationMessage":
			msg.PollCreationMessage.ContextInfo = &waE2E.ContextInfo{}
		case "StickerMessage":
			msg.StickerMessage.ContextInfo = &waE2E.ContextInfo{}
		case "LocationMessage":
			msg.LocationMessage.ContextInfo = &waE2E.ContextInfo{}
		case "ContactMessage":
			msg.ContactMessage.ContextInfo = &waE2E.ContextInfo{}
		default:
			return "", "", fmt.Errorf("invalid messageType: %s", messageType)
		}

		if data.MentionAll {
			allParticipants, err := s.clientPointer[instanceId].WAClient.GetGroupRequestParticipants(recipient)
			if err != nil {
				return "", "", err
			}

			var mentionedJIDs []string
			for _, jid := range allParticipants {
				mentionedJIDs = append(mentionedJIDs, jid.String())
			}

			switch messageType {
			case "ExtendedTextMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "ImageMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "AudioMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "DocumentMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "PollCreationMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "StickerMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "LocationMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "ContactMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = mentionedJIDs
			}
		}

		if data.MentionedJID != "" {
			switch messageType {
			case "ExtendedTextMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = []string{data.MentionedJID}
			case "ImageMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = []string{data.MentionedJID}
			case "AudioMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = []string{data.MentionedJID}
			case "DocumentMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = []string{data.MentionedJID}
			case "PollCreationMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = []string{data.MentionedJID}
			case "StickerMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = []string{data.MentionedJID}
			case "LocationMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = []string{data.MentionedJID}
			case "ContactMessage":
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = []string{data.MentionedJID}
			}
		}
	}

	_, err = s.clientPointer[instanceId].WAClient.SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: msgId})
	if err != nil {
		return "", "", err
	}

	logger.LogInfo("Message sent to %s", data.Number)
	return msgId, time.Now().String(), nil
}

func NewSendService(
	clientPointer map[string]whatsmeow_service.ClientInfo,
	whatsmeowService whatsmeow_service.WhatsmeowService,
) SendService {
	return &sendService{
		clientPointer:    clientPointer,
		whatsmeowService: whatsmeowService,
	}
}
