package send_service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/felipeestevanatto/wamux/pkg/utils"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

type flowButton = waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton

func flowButtons(t *testing.T, msg *waE2E.InteractiveMessage) []*flowButton {
	t.Helper()
	flow := msg.GetNativeFlowMessage()
	if flow == nil {
		t.Fatalf("no native flow message in %+v", msg)
	}
	return flow.GetButtons()
}

func buttonParams(t *testing.T, button *flowButton) map[string]any {
	t.Helper()
	var params map[string]any
	if err := json.Unmarshal([]byte(button.GetButtonParamsJSON()), &params); err != nil {
		t.Fatalf("button params are not JSON: %v", err)
	}
	return params
}

func hasNode(extra []waBinary.Node, tag string) bool {
	for _, node := range extra {
		if node.Tag == tag {
			return true
		}
	}
	return false
}

// mediaServer serves one file per path, and 404 for anything else.
func mediaServer(t *testing.T, files map[string][]byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := files[r.URL.Path]
		if !ok {
			http.Error(w, "<html>not found</html>", http.StatusNotFound)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestSendButtonReply(t *testing.T) {
	h := newSendHarness(t)

	_, err := h.SendButton(&ButtonStruct{
		Number: testNumber, Title: "Offer", Description: "Details", Footer: "Shop", FormatJid: raw(),
		Buttons: []Button{{Type: "reply", DisplayText: "Yes", Id: "yes"}, {Type: "reply", DisplayText: `"No"`, Id: "no"}},
	}, h.instance)
	if err != nil {
		t.Fatalf("SendButton: %v", err)
	}

	sent := h.only(t)
	interactive := sent.msg.GetInteractiveMessage()
	if body := interactive.GetBody().GetText(); !strings.Contains(body, "*Offer*") || !strings.Contains(body, "Details") {
		t.Errorf("body = %q", body)
	}
	buttons := flowButtons(t, interactive)
	if len(buttons) != 2 || buttons[0].GetName() != "quick_reply" {
		t.Fatalf("buttons = %+v", buttons)
	}
	// A quote in a label must stay inside the JSON string.
	if got := buttonParams(t, buttons[1]); got["display_text"] != `"No"` || got["id"] != "no" {
		t.Errorf("params = %v", got)
	}
	if sent.extra.AdditionalNodes == nil || !hasNode(*sent.extra.AdditionalNodes, "biz") || !hasNode(*sent.extra.AdditionalNodes, "bot") {
		t.Errorf("relay nodes = %+v", sent.extra.AdditionalNodes)
	}
}

func TestSendButtonInGroupHasNoBotNode(t *testing.T) {
	h := newSendHarness(t)

	_, err := h.SendButton(&ButtonStruct{
		Number: "120363000000000000@g.us", Title: "T", Description: "D", Footer: "F", FormatJid: raw(),
		Buttons: []Button{{Type: "reply", DisplayText: "Yes", Id: "yes"}},
	}, h.instance)
	if err != nil {
		t.Fatalf("SendButton: %v", err)
	}

	if hasNode(*h.only(t).extra.AdditionalNodes, "bot") {
		t.Fatal("a group message carries the bot node")
	}
}

func TestSendButtonActionTypes(t *testing.T) {
	for _, tc := range []struct {
		name, flow, key, want string
		button                Button
	}{
		{"url", "cta_url", "url", "https://shop.test", Button{Type: "url", DisplayText: "Open", URL: "https://shop.test"}},
		{"call", "cta_call", "phone_number", "+5511999990002", Button{Type: "call", DisplayText: "Call", PhoneNumber: "+5511999990002"}},
		{"copy", "cta_copy", "copy_code", "PROMO", Button{Type: "copy", DisplayText: "Copy", CopyCode: "PROMO"}},
		{"copy falls back to the id", "cta_copy", "copy_code", "code-id", Button{Type: "copy", DisplayText: "Copy", Id: "code-id"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newSendHarness(t)

			_, err := h.SendButton(&ButtonStruct{Number: testNumber, Title: "T", Footer: "F", Buttons: []Button{tc.button}, FormatJid: raw()}, h.instance)
			if err != nil {
				t.Fatalf("SendButton: %v", err)
			}

			button := flowButtons(t, h.only(t).msg.GetInteractiveMessage())[0]
			if button.GetName() != tc.flow {
				t.Errorf("name = %q", button.GetName())
			}
			if got := buttonParams(t, button)[tc.key]; got != tc.want {
				t.Errorf("%s = %v", tc.key, got)
			}
		})
	}
}

func TestSendButtonPix(t *testing.T) {
	h := newSendHarness(t)

	_, err := h.SendButton(&ButtonStruct{
		Number: testNumber, Title: "Pay", Footer: "Shop", FormatJid: raw(),
		Buttons: []Button{{Type: "pix", Currency: "BRL", Name: "Shop", Key: "key@test", KeyType: "email"}},
	}, h.instance)
	if err != nil {
		t.Fatalf("SendButton: %v", err)
	}

	msg := h.only(t).msg
	// Wrapped in a viewOnceMessage, clients show "This message couldn't load".
	if msg.GetViewOnceMessage() != nil || msg.GetInteractiveMessage() == nil {
		t.Fatalf("message = %+v", msg)
	}
	flow := msg.GetInteractiveMessage().GetNativeFlowMessage()
	if flow.GetMessageParamsJSON() != "{}" || flow.GetMessageVersion() != 1 {
		t.Errorf("flow = %+v", flow)
	}
	button := flow.GetButtons()[0]
	if button.GetName() != "payment_info" || !strings.Contains(button.GetButtonParamsJSON(), "key@test") {
		t.Errorf("button = %+v", button)
	}
}

func TestSendButtonRejectsInvalidCombinations(t *testing.T) {
	reply := Button{Type: "reply", DisplayText: "R", Id: "r"}
	link := Button{Type: "url", DisplayText: "U", URL: "https://shop.test"}
	pix := Button{Type: "pix", Currency: "BRL"}

	for name, buttons := range map[string][]Button{
		"four replies":      {reply, reply, reply, reply},
		"reply with action": {reply, link},
		"three actions":     {link, link, link},
		"pix with another":  {pix, link},
	} {
		t.Run(name, func(t *testing.T) {
			h := newSendHarness(t)

			_, err := h.SendButton(&ButtonStruct{Number: testNumber, Title: "T", Footer: "F", Buttons: buttons, FormatJid: raw()}, h.instance)
			if err == nil {
				t.Fatal("want an error")
			}
			if len(h.sent) != 0 {
				t.Fatal("sent anyway")
			}
		})
	}
}

func TestSendList(t *testing.T) {
	h := newSendHarness(t)

	_, err := h.SendList(&ListStruct{
		Number: testNumber, Title: "Plans", Description: "Pick one", FooterText: "Shop", FormatJid: raw(),
		Sections: []Section{{Rows: []Row{{Title: "Basic", Description: "Cheap", RowId: "basic"}, {Title: "Auto id"}}}},
	}, h.instance)
	if err != nil {
		t.Fatalf("SendList: %v", err)
	}

	sent := h.only(t)
	// The legacy listMessage returns success and is never delivered.
	if sent.msg.GetListMessage() != nil {
		t.Fatal("sent a legacy listMessage")
	}
	interactive := sent.msg.GetInteractiveMessage()
	if interactive.GetHeader().GetTitle() != "Plans" || interactive.GetBody().GetText() != "Pick one" || interactive.GetFooter().GetText() != "Shop" {
		t.Errorf("interactive = %+v", interactive)
	}

	button := flowButtons(t, interactive)[0]
	if button.GetName() != "single_select" {
		t.Errorf("name = %q", button.GetName())
	}
	var params struct {
		Title    string `json:"title"`
		Sections []struct {
			Title string `json:"title"`
			Rows  []struct {
				Title string `json:"title"`
				ID    string `json:"id"`
			} `json:"rows"`
		} `json:"sections"`
	}
	if err := json.Unmarshal([]byte(button.GetButtonParamsJSON()), &params); err != nil {
		t.Fatal(err)
	}
	if params.Title != "Ver Menu" {
		t.Errorf("default button text = %q", params.Title)
	}
	rows := params.Sections[0].Rows
	if params.Sections[0].Title == "" || rows[0].ID != "basic" || rows[1].ID == "" {
		t.Errorf("sections = %+v", params.Sections)
	}
	if !hasNode(*sent.extra.AdditionalNodes, "biz") {
		t.Error("no biz node")
	}
}

func TestSendCarouselWithMedia(t *testing.T) {
	h := newSendHarness(t)
	server := mediaServer(t, map[string][]byte{"/a.png": testPNG(t), "/b.mp4": []byte("video bytes")})

	_, err := h.SendCarousel(&CarouselStruct{
		Number: testNumber, Body: "Offers", Footer: "Shop", FormatJid: raw(),
		Cards: []CarouselCardStruct{
			{
				Header: CarouselCardHeaderStruct{Title: "One", ImageUrl: server.URL + "/a.png"},
				Body:   CarouselCardBodyStruct{Text: "First"},
				Footer: "Limited",
				Buttons: []CarouselButtonStruct{
					{DisplayText: "Default", Id: "d"},
					{Type: "url", DisplayText: "Open", Id: "https://shop.test"},
					{Type: "CALL", DisplayText: "Call", Id: "+5511999990002"},
					{Type: "copy", DisplayText: `Co"py`, CopyCode: "PROMO"},
				},
			},
			{Header: CarouselCardHeaderStruct{Title: "Two", VideoUrl: server.URL + "/b.mp4"}, Body: CarouselCardBodyStruct{Text: "Second"}},
		},
	}, h.instance)
	if err != nil {
		t.Fatalf("SendCarousel: %v", err)
	}

	sent := h.only(t)
	if sent.to != testRecipient {
		t.Errorf("sent to %s", sent.to)
	}
	interactive := sent.msg.GetInteractiveMessage()
	if interactive.GetBody().GetText() != "Offers" || interactive.GetFooter().GetText() != "Shop" {
		t.Errorf("interactive = %+v", interactive)
	}
	// iOS needs this field, even empty, to show a carousel.
	if interactive.GetContextInfo() == nil {
		t.Error("no ContextInfo")
	}

	cards := interactive.GetCarouselMessage().GetCards()
	if len(cards) != 2 {
		t.Fatalf("cards = %d", len(cards))
	}
	if !cards[0].GetHeader().GetHasMediaAttachment() || cards[0].GetHeader().GetImageMessage().GetURL() == "" {
		t.Errorf("image card = %+v", cards[0].GetHeader())
	}
	if !cards[1].GetHeader().GetHasMediaAttachment() || cards[1].GetHeader().GetVideoMessage().GetMimetype() != "video/mp4" {
		t.Errorf("video card = %+v", cards[1].GetHeader())
	}
	if cards[0].GetFooter().GetText() != "Limited" {
		t.Errorf("footer = %q", cards[0].GetFooter().GetText())
	}

	buttons := flowButtons(t, cards[0])
	names := make([]string, 0, len(buttons))
	for _, button := range buttons {
		names = append(names, button.GetName())
	}
	if got := fmt.Sprint(names); got != "[quick_reply cta_url cta_call cta_copy]" {
		t.Errorf("buttons = %s", got)
	}
	if got := buttonParams(t, buttons[3])["display_text"]; got != `Co"py` {
		t.Errorf("escaped label = %v", got)
	}
}

// Sent as a one-card carousel, this does not render on iOS.
func TestSendCarouselSingleCardWithoutMediaIsAPlainMessage(t *testing.T) {
	h := newSendHarness(t)

	_, err := h.SendCarousel(&CarouselStruct{
		Number: testNumber, FormatJid: raw(),
		Cards: []CarouselCardStruct{{Header: CarouselCardHeaderStruct{Title: "One"}, Body: CarouselCardBodyStruct{Text: "Only"}}},
	}, h.instance)
	if err != nil {
		t.Fatalf("SendCarousel: %v", err)
	}

	sent := h.only(t)
	interactive := sent.msg.GetInteractiveMessage()
	if interactive.GetCarouselMessage() != nil || interactive.GetBody().GetText() != "Only" {
		t.Fatalf("interactive = %+v", interactive)
	}
	if sent.to != testRecipient {
		t.Errorf("sent to %s", sent.to)
	}
}

// A number sent with formatJid false must reach WhatsApp exactly as given.
func TestSendCarouselRespectsFormatJid(t *testing.T) {
	const number = "5541990000005" // Formatting would drop the Brazilian ninth digit.
	exact := testRecipient
	exact.User = number
	card := func(title string) CarouselCardStruct {
		return CarouselCardStruct{Header: CarouselCardHeaderStruct{Title: title}, Body: CarouselCardBodyStruct{Text: title}}
	}

	for name, cards := range map[string][]CarouselCardStruct{
		"carousel":    {card("a"), card("b")},
		"single card": {card("a")},
	} {
		t.Run(name, func(t *testing.T) {
			h := newSendHarness(t)

			if _, err := h.SendCarousel(&CarouselStruct{Number: number, Cards: cards, FormatJid: raw()}, h.instance); err != nil {
				t.Fatalf("SendCarousel: %v", err)
			}

			if got := h.only(t).to; got != exact {
				t.Fatalf("sent to %s, want %s", got, exact)
			}
		})
	}
}

func TestSendTextRespectsFormatJid(t *testing.T) {
	const number = "5541990000005"
	format := true

	for name, tc := range map[string]struct {
		formatJid *bool
		want      string
	}{
		"off":        {raw(), number},
		"on":         {&format, "554190000005"},
		"by default": {nil, "554190000005"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newSendHarness(t)

			if _, err := h.SendText(&TextStruct{Number: number, Text: "x", FormatJid: tc.formatJid}, h.instance); err != nil {
				t.Fatalf("SendText: %v", err)
			}

			if got := h.only(t).to.User; got != tc.want {
				t.Fatalf("sent to %s, want %s", got, tc.want)
			}
		})
	}
}

// Mexican and Argentine mobiles reach WhatsApp with every digit, formatted or not.
func TestSendTextKeepsMexicanAndArgentineMobiles(t *testing.T) {
	for _, number := range []string{"5215551234567", "5491123456789"} {
		t.Run(number, func(t *testing.T) {
			h := newSendHarness(t)

			if _, err := h.SendText(&TextStruct{Number: number, Text: "x"}, h.instance); err != nil {
				t.Fatalf("SendText: %v", err)
			}

			if got := h.only(t).to.User; got != number {
				t.Fatalf("sent to %s, want %s", got, number)
			}
		})
	}
}

// A vCard holds someone's name and phone number, so it stays out of stdout.
func TestSendContactPrintsNothing(t *testing.T) {
	h := newSendHarness(t)
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = write
	t.Cleanup(func() { os.Stdout = stdout })

	_, err = h.SendContact(&ContactStruct{Number: testRecipient.User, Vcard: utils.VCardStruct{FullName: "Secret Name", Phone: "5541900000005"}}, h.instance)
	os.Stdout = stdout
	write.Close()
	if err != nil {
		t.Fatalf("SendContact: %v", err)
	}

	printed, err := io.ReadAll(read)
	if err != nil {
		t.Fatal(err)
	}
	if len(printed) != 0 {
		t.Fatalf("printed %q", printed)
	}
}

func TestValidateMessageFieldsKeepsOtherAddressesWithFormatOff(t *testing.T) {
	off := false
	for input, want := range map[string]string{
		"120363000000000000@g.us":             "120363000000000000@g.us",
		"5215551234567@s.whatsapp.net":        "5215551234567@s.whatsapp.net",
		"100000000000000@lid":                 "100000000000000@lid",
		"120363000000000000000000@newsletter": "120363000000000000000000@newsletter",
	} {
		got, err := validateMessageFields(input, &off, nil, nil)
		if err != nil {
			t.Errorf("%s: %v", input, err)
			continue
		}
		if got.String() != want {
			t.Errorf("%s became %s", input, got)
		}
	}
}

// An error page is not the card's media, and uploading it delivers a broken card.
func TestSendCarouselSkipsMediaThatFailedToDownload(t *testing.T) {
	h := newSendHarness(t)
	server := mediaServer(t, nil)

	_, err := h.SendCarousel(&CarouselStruct{
		Number: testNumber, FormatJid: raw(),
		Cards: []CarouselCardStruct{
			{Header: CarouselCardHeaderStruct{ImageUrl: server.URL + "/gone.png"}, Body: CarouselCardBodyStruct{Text: "a"}},
			{Header: CarouselCardHeaderStruct{VideoUrl: server.URL + "/gone.mp4"}, Body: CarouselCardBodyStruct{Text: "b"}},
		},
	}, h.instance)
	if err != nil {
		t.Fatalf("SendCarousel: %v", err)
	}

	if len(h.uploads) != 0 {
		t.Fatalf("uploaded an error page: %v", h.uploads)
	}
	for i, card := range h.only(t).msg.GetInteractiveMessage().GetCarouselMessage().GetCards() {
		if card.GetHeader().GetHasMediaAttachment() {
			t.Errorf("card %d claims to have media", i)
		}
	}
}

func TestSendLinkPreview(t *testing.T) {
	h := newSendHarness(t)
	image := testPNG(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/thumb.png" {
			_, _ = w.Write(image)
			return
		}
		fmt.Fprintf(w, `<html><head><title>Fallback</title>
			<meta property="og:title" content="Big Sale"/>
			<meta property="og:description" content="All week"/>
			<meta property="og:image" content="%s/thumb.png"/></head>
			<body><svg><title>Icon</title></svg></body></html>`, server.URL)
	}))
	t.Cleanup(server.Close)
	link := server.URL + "/page"

	_, err := h.SendLink(&LinkStruct{Number: testNumber, Text: "See " + link, FormatJid: raw()}, h.instance)
	if err != nil {
		t.Fatalf("SendLink: %v", err)
	}

	preview := h.only(t).msg.GetExtendedTextMessage()
	if preview.GetText() != "See "+link || preview.GetMatchedText() != link {
		t.Errorf("text %q, matched %q", preview.GetText(), preview.GetMatchedText())
	}
	if preview.GetTitle() != "Big Sale" || preview.GetDescription() != "All week" {
		t.Errorf("title %q, description %q", preview.GetTitle(), preview.GetDescription())
	}
	if len(preview.GetJPEGThumbnail()) == 0 {
		t.Error("no thumbnail")
	}
}

// Inline SVG icons have titles too, and they must not replace the page's.
func TestLinkMetadataFallsBackToThePageTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><head><title>Page</title></head><body><svg><title>Icon</title></svg></body></html>`)
	}))
	t.Cleanup(server.Close)

	title, _, _, err := fetchLinkMetadata(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Page" {
		t.Fatalf("title = %q", title)
	}
}

func TestSendLinkWithoutAURLStillSends(t *testing.T) {
	h := newSendHarness(t)

	_, err := h.SendLink(&LinkStruct{Number: testNumber, Text: "no link here", Title: "Given", FormatJid: raw()}, h.instance)
	if err != nil {
		t.Fatalf("SendLink: %v", err)
	}

	preview := h.only(t).msg.GetExtendedTextMessage()
	if preview.GetText() != "no link here" || preview.GetTitle() != "Given" {
		t.Fatalf("preview = %+v", preview)
	}
}

func TestSendMediaUrlImage(t *testing.T) {
	h := newSendHarness(t)
	server := mediaServer(t, map[string][]byte{"/photo.png": testPNG(t)})

	_, err := h.SendMediaUrl(&MediaStruct{Number: testNumber, Url: server.URL + "/photo.png", Type: "image", Caption: "cap", FormatJid: raw()}, h.instance)
	if err != nil {
		t.Fatalf("SendMediaUrl: %v", err)
	}

	image := h.only(t).msg.GetImageMessage()
	if image.GetCaption() != "cap" || image.GetMimetype() != "image/png" || len(image.GetMediaKey()) == 0 {
		t.Fatalf("image = %+v", image)
	}
}

func TestSendMediaUrlDocument(t *testing.T) {
	h := newSendHarness(t)
	server := mediaServer(t, map[string][]byte{"/notes.txt": []byte("plain text")})

	_, err := h.SendMediaUrl(&MediaStruct{Number: testNumber, Url: server.URL + "/notes.txt", Type: "document", Filename: "notes.txt", FormatJid: raw()}, h.instance)
	if err != nil {
		t.Fatalf("SendMediaUrl: %v", err)
	}

	msg := h.only(t).msg
	doc := msg.GetDocumentMessage()
	if doc == nil {
		doc = msg.GetDocumentWithCaptionMessage().GetMessage().GetDocumentMessage()
	}
	if doc.GetFileName() != "notes.txt" {
		t.Fatalf("document = %+v", doc)
	}
}

func TestSendMediaUrlRefusesWhatIsNotTheMedia(t *testing.T) {
	server := mediaServer(t, map[string][]byte{"/fake.mp4": []byte("<html>access denied</html>")})

	for name, media := range map[string]MediaStruct{
		"error status":             {Url: server.URL + "/gone.png", Type: "image"},
		"page behind a media name": {Url: server.URL + "/fake.mp4", Type: "video"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newSendHarness(t)
			media.Number, media.FormatJid = testNumber, raw()

			if _, err := h.SendMediaUrl(&media, h.instance); err == nil {
				t.Fatal("want an error")
			}
			if len(h.uploads) != 0 || len(h.sent) != 0 {
				t.Fatalf("uploaded %d, sent %d", len(h.uploads), len(h.sent))
			}
		})
	}
}
