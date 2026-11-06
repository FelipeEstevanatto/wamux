package send_service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

const testNewsletter = "120363000000000000@newsletter"

var statusBroadcast = types.NewJID("status", "broadcast")

func TestSendStatusText(t *testing.T) {
	h := newSendHarness(t)

	got, err := h.SendStatusText(&StatusTextStruct{Text: "Good morning"}, h.instance)
	if err != nil {
		t.Fatalf("SendStatusText: %v", err)
	}

	sent := h.only(t)
	if sent.to != statusBroadcast || sent.msg.GetExtendedTextMessage().GetText() != "Good morning" {
		t.Fatalf("sent %+v to %s", sent.msg, sent.to)
	}
	if got.Info.ID == "" || got.Info.ID != sent.extra.ID {
		t.Errorf("info ID %q, sent ID %q", got.Info.ID, sent.extra.ID)
	}
	if event := h.event(t); event["event"] != "SendStatus" {
		t.Errorf("event = %v", event["event"])
	}
}

func TestSendStatusRejectsBadInput(t *testing.T) {
	server := mediaServer(t, nil)

	for name, send := range map[string]func(h *sendHarness) error{
		"empty text": func(h *sendHarness) error {
			_, err := h.SendStatusText(&StatusTextStruct{}, h.instance)
			return err
		},
		"no url": func(h *sendHarness) error {
			_, err := h.SendStatusMediaUrl(&StatusMediaStruct{Type: "image"}, h.instance)
			return err
		},
		"bad type on a url": func(h *sendHarness) error {
			_, err := h.SendStatusMediaUrl(&StatusMediaStruct{Type: "audio", Url: server.URL}, h.instance)
			return err
		},
		"bad type on a file": func(h *sendHarness) error {
			_, err := h.SendStatusMediaFile(&StatusMediaStruct{Type: "audio"}, []byte("x"), h.instance)
			return err
		},
		"error page": func(h *sendHarness) error {
			_, err := h.SendStatusMediaUrl(&StatusMediaStruct{Type: "image", Url: server.URL + "/gone.png"}, h.instance)
			return err
		},
		"text as an image": func(h *sendHarness) error {
			_, err := h.SendStatusMediaFile(&StatusMediaStruct{Type: "image"}, []byte("plain text"), h.instance)
			return err
		},
		"text as a video": func(h *sendHarness) error {
			_, err := h.SendStatusMediaFile(&StatusMediaStruct{Type: "video"}, []byte("plain text"), h.instance)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newSendHarness(t)

			if err := send(h); err == nil {
				t.Fatal("want an error")
			}
			if len(h.sent) != 0 || len(h.uploads) != 0 {
				t.Fatalf("sent %d, uploaded %d", len(h.sent), len(h.uploads))
			}
		})
	}
}

func TestSendStatusImage(t *testing.T) {
	file := testPNG(t)
	server := mediaServer(t, map[string][]byte{"/photo.png": file})

	for name, send := range map[string]func(h *sendHarness) error{
		"from a url": func(h *sendHarness) error {
			_, err := h.SendStatusMediaUrl(&StatusMediaStruct{Type: "image", Url: server.URL + "/photo.png", Caption: "cap"}, h.instance)
			return err
		},
		"from a file": func(h *sendHarness) error {
			_, err := h.SendStatusMediaFile(&StatusMediaStruct{Type: "image", Caption: "cap"}, file, h.instance)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newSendHarness(t)

			if err := send(h); err != nil {
				t.Fatal(err)
			}

			sent := h.only(t)
			image := sent.msg.GetImageMessage()
			if sent.to != statusBroadcast || image.GetCaption() != "cap" || image.GetWidth() != 8 {
				t.Fatalf("sent %+v to %s", image, sent.to)
			}
			if len(image.GetJPEGThumbnail()) == 0 {
				t.Error("no thumbnail, so the status shows a gray placeholder")
			}
		})
	}
}

// Newsletters are not end-to-end encrypted, so media goes up in the clear and is
// referenced by a handle instead of a key.
func TestSendNewsletterImage(t *testing.T) {
	h := newSendHarness(t)
	var plain []whatsmeow.MediaType
	h.upload = func(context.Context, *whatsmeow.Client, []byte, whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
		t.Error("used the encrypted upload")
		return whatsmeow.UploadResponse{}, nil
	}
	h.uploadNewsletter = func(_ context.Context, _ *whatsmeow.Client, data []byte, kind whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
		plain = append(plain, kind)
		return whatsmeow.UploadResponse{URL: "https://mmg.test/n", DirectPath: "/n", Handle: "handle-1", FileLength: uint64(len(data))}, nil
	}

	_, err := h.SendMediaFile(&MediaStruct{Number: testNewsletter, Type: "image", Caption: "news", FormatJid: raw()}, testPNG(t), h.instance)
	if err != nil {
		t.Fatalf("SendMediaFile: %v", err)
	}

	sent := h.only(t)
	if len(plain) != 1 || plain[0] != whatsmeow.MediaImage {
		t.Fatalf("newsletter uploads = %v", plain)
	}
	if sent.to.Server != types.NewsletterServer {
		t.Errorf("sent to %s", sent.to)
	}
	image := sent.msg.GetImageMessage()
	if image.GetCaption() != "news" || len(image.GetMediaKey()) != 0 || len(image.GetFileEncSHA256()) != 0 {
		t.Errorf("image = %+v", image)
	}
	if sent.extra.MediaHandle != "handle-1" {
		t.Errorf("handle = %q", sent.extra.MediaHandle)
	}
}

func TestSendNewsletterImageFromAURL(t *testing.T) {
	h := newSendHarness(t)
	server := mediaServer(t, map[string][]byte{"/a.png": testPNG(t)})
	h.uploadNewsletter = func(context.Context, *whatsmeow.Client, []byte, whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
		return whatsmeow.UploadResponse{URL: "https://mmg.test/n", Handle: "handle-2"}, nil
	}

	_, err := h.SendMediaUrl(&MediaStruct{Number: testNewsletter, Url: server.URL + "/a.png", Type: "image", FormatJid: raw()}, h.instance)
	if err != nil {
		t.Fatalf("SendMediaUrl: %v", err)
	}

	sent := h.only(t)
	if sent.extra.MediaHandle != "handle-2" || len(sent.msg.GetImageMessage().GetMediaKey()) != 0 {
		t.Fatalf("handle %q, image %+v", sent.extra.MediaHandle, sent.msg.GetImageMessage())
	}
}

// The server answers 479 to a poll in a channel, so it must not get that far.
func TestSendPollRefusesChannels(t *testing.T) {
	h := newSendHarness(t)

	_, err := h.SendPoll(&PollStruct{Number: testNewsletter, Question: "Q", MaxAnswer: 1, Options: []string{"A", "B"}, FormatJid: raw()}, h.instance)
	if err == nil || !strings.Contains(err.Error(), "canais") {
		t.Fatalf("err = %v", err)
	}
	if len(h.sent) != 0 {
		t.Fatal("sent anyway")
	}
}

func TestSendStickerConvertsOtherFormats(t *testing.T) {
	h := newSendHarness(t)
	server := mediaServer(t, map[string][]byte{"/s.png": testPNG(t)})

	_, err := h.SendSticker(&StickerStruct{Number: testNumber, Sticker: server.URL + "/s.png", FormatJid: raw()}, h.instance)
	if err != nil {
		t.Fatalf("SendSticker: %v", err)
	}

	sticker := h.only(t).msg.GetStickerMessage()
	if sticker.GetMimetype() != "image/webp" || sticker.GetIsAnimated() {
		t.Fatalf("sticker = %+v", sticker)
	}
}

func TestSendStickerKeepsAWebPUntouched(t *testing.T) {
	h := newSendHarness(t)
	webp := []byte("RIFF\x10\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00\x02\x00\x00\x00ANIM")
	var uploaded []byte
	h.upload = func(_ context.Context, _ *whatsmeow.Client, data []byte, _ whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
		uploaded = data
		return whatsmeow.UploadResponse{URL: "https://mmg.test/s", DirectPath: "/s"}, nil
	}
	server := mediaServer(t, map[string][]byte{"/s.webp": webp})

	_, err := h.SendSticker(&StickerStruct{Number: testNumber, Sticker: server.URL + "/s.webp", FormatJid: raw()}, h.instance)
	if err != nil {
		t.Fatalf("SendSticker: %v", err)
	}

	if !bytes.Equal(uploaded, webp) {
		t.Fatal("the WebP was re-encoded, which would flatten an animation")
	}
	if !h.only(t).msg.GetStickerMessage().GetIsAnimated() {
		t.Fatal("not marked animated")
	}
}

func TestSendEvent(t *testing.T) {
	h := newSendHarness(t)
	var data EventStruct
	body := `{"number":"` + testNumber + `","name":"Sales","description":"Quarterly","text":"Invite",
		"startTime":"2026-06-25T20:00:00-03:00","endTime":1782435600,"hasReminder":true,"reminderOffsetSec":900,
		"location":{"name":"HQ","latitude":-16.5,"longitude":-49.25}}`
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data.FormatJid = raw()

	if _, err := h.SendEvent(&data, h.instance); err != nil {
		t.Fatalf("SendEvent: %v", err)
	}

	if len(h.sent) != 2 {
		t.Fatalf("sent %d messages, want the text and the event", len(h.sent))
	}
	if h.sent[0].msg.GetExtendedTextMessage().GetText() != "Invite" {
		t.Errorf("first message = %+v", h.sent[0].msg)
	}
	msg := h.sent[1].msg
	event := msg.GetEventMessage()
	want := time.Date(2026, 6, 25, 23, 0, 0, 0, time.UTC).Unix()
	if event.GetName() != "Sales" || event.GetStartTime() != want || event.GetEndTime() != 1782435600 {
		t.Errorf("event = %+v", event)
	}
	if !event.GetHasReminder() || event.GetReminderOffsetSec() != 900 || event.GetLocation().GetName() != "HQ" {
		t.Errorf("event = %+v", event)
	}
	// Left nil they are dropped from the wire, and the server ignores the event.
	if event.IsCanceled == nil || event.IsScheduleCall == nil || event.ExtraGuestsAllowed == nil {
		t.Error("the booleans must be sent explicitly")
	}
	if len(msg.GetMessageContextInfo().GetMessageSecret()) != 32 {
		t.Error("no message secret, so replies cannot be decrypted")
	}
}

func TestEventTimeFormats(t *testing.T) {
	for input, want := range map[string]int64{
		`1782435600`:                  1782435600,
		`"1782435600"`:                1782435600,
		`"2026-06-25T23:00:00Z"`:      1782428400,
		`"2026-06-25T20:00:00-03:00"`: 1782428400,
		`""`:                          0,
		`null`:                        0,
	} {
		var got EventTime
		if err := json.Unmarshal([]byte(input), &got); err != nil {
			t.Errorf("%s: %v", input, err)
		}
		if got.Unix() != want {
			t.Errorf("%s = %d, want %d", input, got.Unix(), want)
		}
	}

	var bad EventTime
	if err := json.Unmarshal([]byte(`"tomorrow"`), &bad); err == nil {
		t.Error("accepted a date it cannot read")
	}
}

func TestSendProduct(t *testing.T) {
	file := testPNG(t)
	server := mediaServer(t, map[string][]byte{"/p.png": file})

	for name, source := range map[string]ProductStruct{
		"from base64":   {ImageBase64: base64.StdEncoding.EncodeToString(file)},
		"from data url": {ImageBase64: "data:image/png;base64," + base64.StdEncoding.EncodeToString(file)},
		"from a url":    {ImageURL: server.URL + "/p.png"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newSendHarness(t)
			data := source
			data.Number, data.FormatJid = testNumber, raw()
			data.ProductId, data.Title, data.Currency, data.Price = "123", "Shirt", "BRL", 10000

			if _, err := h.SendProduct(&data, h.instance); err != nil {
				t.Fatalf("SendProduct: %v", err)
			}

			product := h.only(t).msg.GetProductMessage()
			snapshot := product.GetProduct()
			if snapshot.GetTitle() != "Shirt" || snapshot.GetPriceAmount1000() != 10000 || snapshot.GetProductImage().GetURL() == "" {
				t.Errorf("snapshot = %+v", snapshot)
			}
			if product.GetBusinessOwnerJID() != "5511999990000@s.whatsapp.net" {
				t.Errorf("owner = %q", product.GetBusinessOwnerJID())
			}
		})
	}
}

func TestSendProductValidates(t *testing.T) {
	server := mediaServer(t, nil)
	valid := ProductStruct{Number: testNumber, ProductId: "1", Title: "T", Currency: "BRL", FormatJid: raw()}

	for name, change := range map[string]func(*ProductStruct){
		"no product id":  func(p *ProductStruct) { p.ProductId = "" },
		"no title":       func(p *ProductStruct) { p.Title = "" },
		"no currency":    func(p *ProductStruct) { p.Currency = "" },
		"no image":       func(*ProductStruct) {},
		"invalid base64": func(p *ProductStruct) { p.ImageBase64 = "!!!" },
		"image is gone":  func(p *ProductStruct) { p.ImageURL = server.URL + "/gone.png" },
	} {
		t.Run(name, func(t *testing.T) {
			h := newSendHarness(t)
			data := valid
			change(&data)

			if _, err := h.SendProduct(&data, h.instance); err == nil {
				t.Fatal("want an error")
			}
			if len(h.sent) != 0 {
				t.Fatal("sent anyway")
			}
		})
	}
}

func onWhatsApp(found bool, jid string) []types.IsOnWhatsAppResponse {
	parsed, _ := types.ParseJID(jid)
	return []types.IsOnWhatsAppResponse{{Query: jid, JID: parsed, IsIn: found}}
}

func TestSendChecksTheNumberAndUsesWhatWhatsAppReturns(t *testing.T) {
	h := newSendHarness(t)
	h.config.CheckUserExists = true
	calls := 0
	h.isOnWhatsApp = func(context.Context, *whatsmeow.Client, []string) ([]types.IsOnWhatsAppResponse, error) {
		calls++
		return onWhatsApp(true, "5511999990009@s.whatsapp.net"), nil
	}

	for range 2 {
		if _, err := h.SendText(&TextStruct{Number: testNumber, Text: "x"}, h.instance); err != nil {
			t.Fatalf("SendText: %v", err)
		}
	}

	if h.sent[0].to.User != "5511999990009" {
		t.Errorf("sent to %s", h.sent[0].to)
	}
	if calls != 1 {
		t.Errorf("checked %d times, want the second send to use the cache", calls)
	}
}

func TestSendRefusesANumberNotOnWhatsApp(t *testing.T) {
	h := newSendHarness(t)
	h.config.CheckUserExists = true
	calls := 0
	h.isOnWhatsApp = func(context.Context, *whatsmeow.Client, []string) ([]types.IsOnWhatsAppResponse, error) {
		calls++
		return onWhatsApp(false, testNumber+"@s.whatsapp.net"), nil
	}

	for range 2 {
		_, err := h.SendText(&TextStruct{Number: testNumber, Text: "x"}, h.instance)
		if err == nil || !strings.Contains(err.Error(), "not registered") {
			t.Fatalf("err = %v", err)
		}
	}

	if len(h.sent) != 0 {
		t.Fatal("sent anyway")
	}
	if calls != 2 {
		t.Errorf("checked %d times: both formats once, then the cache", calls)
	}
}

// A failed check says nothing about the number, so the send still goes out.
func TestSendContinuesWhenTheCheckFails(t *testing.T) {
	h := newSendHarness(t)
	h.config.CheckUserExists = true
	h.isOnWhatsApp = func(context.Context, *whatsmeow.Client, []string) ([]types.IsOnWhatsAppResponse, error) {
		return nil, errors.New("usync timeout")
	}

	if _, err := h.SendText(&TextStruct{Number: testNumber, Text: "x"}, h.instance); err != nil {
		t.Fatalf("SendText: %v", err)
	}

	h.only(t)
}

func TestSendSkipsTheCheckForGroups(t *testing.T) {
	h := newSendHarness(t)
	h.config.CheckUserExists = true
	h.isOnWhatsApp = func(context.Context, *whatsmeow.Client, []string) ([]types.IsOnWhatsAppResponse, error) {
		t.Error("checked a group")
		return nil, nil
	}
	h.groupInfo = func(context.Context, *whatsmeow.Client, types.JID) (*types.GroupInfo, error) {
		return &types.GroupInfo{}, nil
	}

	if _, err := h.SendText(&TextStruct{Number: "120363000000000000@g.us", Text: "x"}, h.instance); err != nil {
		t.Fatalf("SendText: %v", err)
	}
}

func groupWith(phones ...string) *types.GroupInfo {
	info := &types.GroupInfo{}
	for _, phone := range phones {
		jid := types.NewJID(phone, types.DefaultUserServer)
		info.Participants = append(info.Participants, types.GroupParticipant{JID: jid, PhoneNumber: jid})
	}
	return info
}

func TestSendMentionAllTagsEveryParticipant(t *testing.T) {
	h := newSendHarness(t)
	lookups := 0
	h.groupInfo = func(context.Context, *whatsmeow.Client, types.JID) (*types.GroupInfo, error) {
		lookups++
		info := groupWith("100", "200")
		info.DisappearingTimer = 604800
		return info, nil
	}

	// Its own group: the disappearing timer cache outlives each test.
	_, err := h.SendText(&TextStruct{Number: "120363000000000001@g.us", Text: "all", MentionAll: true, FormatJid: raw()}, h.instance)
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}

	text := h.only(t).msg.GetExtendedTextMessage()
	if got := text.GetText(); got != "all\n\n@100 @200" {
		t.Errorf("text = %q", got)
	}
	info := text.GetContextInfo()
	if got := info.GetMentionedJID(); len(got) != 2 || got[0] != "100@s.whatsapp.net" {
		t.Errorf("mentions = %v", got)
	}
	if info.GetExpiration() != 604800 {
		t.Errorf("expiration = %d", info.GetExpiration())
	}
	if lookups != 1 {
		t.Errorf("looked the group up %d times", lookups)
	}
}

func TestSendMentionAllLeavesOutTheSender(t *testing.T) {
	h := newSendHarness(t)
	h.groupInfo = func(context.Context, *whatsmeow.Client, types.JID) (*types.GroupInfo, error) {
		return groupWith("5511999990000", "200"), nil
	}

	_, err := h.SendText(&TextStruct{Number: "120363000000000002@g.us", Text: "all", MentionAll: true, FormatJid: raw()}, h.instance)
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}

	text := h.only(t).msg.GetExtendedTextMessage()
	if text.GetText() != "all\n\n@200" || len(text.GetContextInfo().GetMentionedJID()) != 1 {
		t.Fatalf("text %q, mentions %v", text.GetText(), text.GetContextInfo().GetMentionedJID())
	}
}

func TestSendMentionedJidWritesTheToken(t *testing.T) {
	const group = "120363000000000003@g.us"
	for name, tc := range map[string]struct{ text, jid, want string }{
		"added":           {"hi", "100@s.whatsapp.net", "hi\n\n@100"},
		"already there":   {"hi @100!", "100@s.whatsapp.net", "hi @100!"},
		"a longer number": {"hi @1000", "100@s.whatsapp.net", "hi @1000\n\n@100"},
		"lid":             {"hi", "200@lid", "hi\n\n@200"},
		"device suffix":   {"hi", "100:7@s.whatsapp.net", "hi\n\n@100"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newSendHarness(t)

			_, err := h.SendText(&TextStruct{Number: group, Text: tc.text, MentionedJID: []string{tc.jid}, FormatJid: raw()}, h.instance)
			if err != nil {
				t.Fatalf("SendText: %v", err)
			}

			if got := h.only(t).msg.GetExtendedTextMessage().GetText(); got != tc.want {
				t.Fatalf("text = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSendMentionsGoInMediaCaptions(t *testing.T) {
	h := newSendHarness(t)

	_, err := h.SendMediaFile(&MediaStruct{
		Number: "120363000000000003@g.us", Type: "image", Caption: "photo",
		MentionedJID: []string{"100@s.whatsapp.net"}, FormatJid: raw(),
	}, testPNG(t), h.instance)
	if err != nil {
		t.Fatalf("SendMediaFile: %v", err)
	}

	if got := h.only(t).msg.GetImageMessage().GetCaption(); got != "photo\n\n@100" {
		t.Fatalf("caption = %q", got)
	}
}

// A retry rebuilds the message from the request, which must stay as the caller sent it.
func TestSendMentionTokensDoNotPileUpOnRetry(t *testing.T) {
	h := newSendHarness(t)
	h.sendErr = func(attempt int) error {
		if attempt == 1 {
			return errors.New("client disconnected")
		}
		return nil
	}
	data := &TextStruct{Number: "120363000000000003@g.us", Text: "hi", MentionedJID: []string{"100@s.whatsapp.net"}, FormatJid: raw()}

	if _, err := h.sendTextWithRetry(data, h.instance, 2); err != nil {
		t.Fatal(err)
	}

	if got := h.sent[1].msg.GetExtendedTextMessage().GetText(); got != "hi\n\n@100" {
		t.Fatalf("text = %q", got)
	}
	if data.Text != "hi" {
		t.Fatalf("request changed to %q", data.Text)
	}
}

func TestSendMentionAllFailsWithoutTheGroup(t *testing.T) {
	h := newSendHarness(t)
	h.groupInfo = func(context.Context, *whatsmeow.Client, types.JID) (*types.GroupInfo, error) {
		return nil, errors.New("not a participant")
	}

	_, err := h.SendText(&TextStruct{Number: "120363000000000000@g.us", Text: "all", MentionAll: true, FormatJid: raw()}, h.instance)
	if err == nil {
		t.Fatal("want an error")
	}
	if len(h.sent) != 0 {
		t.Fatal("sent without the mentions")
	}
}

func TestSendMentionsOnlyApplyToGroups(t *testing.T) {
	h := newSendHarness(t)

	_, err := h.SendText(&TextStruct{Number: testNumber, Text: "x", MentionedJID: []string{"100@s.whatsapp.net"}, FormatJid: raw()}, h.instance)
	if err != nil {
		t.Fatal(err)
	}

	if got := h.only(t).msg.GetExtendedTextMessage().GetContextInfo().GetMentionedJID(); len(got) != 0 {
		t.Fatalf("mentions = %v", got)
	}
}

// ffmpegSample makes real media, so these tests need ffmpeg on the machine.
func ffmpegSample(t *testing.T, name string, args ...string) []byte {
	t.Helper()
	if _, err := exec.LookPath(ffmpegBin); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	path := filepath.Join(t.TempDir(), name)
	cmd := exec.Command(ffmpegBin, append(append([]string{"-loglevel", "error", "-y"}, args...), path)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func sampleVideo(t *testing.T) []byte {
	return ffmpegSample(t, "v.mp4", "-f", "lavfi", "-i", "testsrc=duration=2:size=64x48:rate=10", "-pix_fmt", "yuv420p")
}

func TestSendVideoFileCarriesItsSizeAndPreview(t *testing.T) {
	file := sampleVideo(t)
	h := newSendHarness(t)

	_, err := h.SendMediaFile(&MediaStruct{Number: testNumber, Type: "video", Caption: "clip", FormatJid: raw()}, file, h.instance)
	if err != nil {
		t.Fatalf("SendMediaFile: %v", err)
	}

	video := h.only(t).msg.GetVideoMessage()
	if video.GetMimetype() != "video/mp4" || video.GetWidth() != 64 || video.GetHeight() != 48 || video.GetSeconds() != 2 {
		t.Errorf("video = %+v", video)
	}
	if len(video.GetJPEGThumbnail()) == 0 {
		t.Error("no preview")
	}
}

func TestSendStatusVideo(t *testing.T) {
	file := sampleVideo(t)
	h := newSendHarness(t)

	if _, err := h.SendStatusMediaFile(&StatusMediaStruct{Type: "video", Caption: "clip"}, file, h.instance); err != nil {
		t.Fatalf("SendStatusMediaFile: %v", err)
	}

	sent := h.only(t)
	if sent.to != statusBroadcast || sent.msg.GetVideoMessage().GetWidth() != 64 {
		t.Fatalf("sent %+v to %s", sent.msg, sent.to)
	}
}

func TestSendAudioBecomesAVoiceNote(t *testing.T) {
	file := ffmpegSample(t, "a.wav", "-f", "lavfi", "-i", "sine=frequency=440:duration=2")
	h := newSendHarness(t)

	_, err := h.SendMediaFile(&MediaStruct{Number: testNumber, Type: "audio", FormatJid: raw()}, file, h.instance)
	if err != nil {
		t.Fatalf("SendMediaFile: %v", err)
	}

	audio := h.only(t).msg.GetAudioMessage()
	if !strings.HasPrefix(audio.GetMimetype(), "audio/ogg") || !audio.GetPTT() {
		t.Errorf("audio = %+v", audio)
	}
	if audio.GetSeconds() < 1 || audio.GetSeconds() > 3 {
		t.Errorf("seconds = %d", audio.GetSeconds())
	}
}

func TestSendAnimatedGifBecomesALoopingVideo(t *testing.T) {
	if _, err := exec.LookPath(ffmpegBin); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	palette := color.Palette{color.Black, color.White}
	animation := &gif.GIF{}
	for i := range 4 {
		frame := image.NewPaletted(image.Rect(0, 0, 16, 16), palette)
		frame.SetColorIndex(i, i, 1)
		animation.Image = append(animation.Image, frame)
		animation.Delay = append(animation.Delay, 10)
	}
	var file bytes.Buffer
	if err := gif.EncodeAll(&file, animation); err != nil {
		t.Fatal(err)
	}
	h := newSendHarness(t)

	_, err := h.SendMediaFile(&MediaStruct{Number: testNumber, Type: "gif", FormatJid: raw()}, file.Bytes(), h.instance)
	if err != nil {
		t.Fatalf("SendMediaFile: %v", err)
	}

	video := h.only(t).msg.GetVideoMessage()
	if video.GetMimetype() != "video/mp4" || !video.GetGifPlayback() {
		t.Fatalf("video = %+v", video)
	}
}
