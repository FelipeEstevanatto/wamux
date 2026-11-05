package whatsmeow_service

import (
	"errors"
	"testing"
	"time"

	"go.mau.fi/util/jsontime"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"github.com/felipeestevanatto/wamux/pkg/killswitch"
)

func receipt(t *testing.T, kind types.ReceiptType, ids ...string) *events.Receipt {
	t.Helper()
	contact := mustJID(t, testContact)
	return &events.Receipt{
		MessageSource: types.MessageSource{Chat: contact, Sender: contact},
		MessageIDs:    ids,
		Timestamp:     testTime,
		Type:          kind,
	}
}

func TestReadReceiptUpdatesEveryMessage(t *testing.T) {
	h := newEventHarness(t)
	got := h.one(t, receipt(t, types.ReceiptTypeRead, "A", "B"))
	wantField(t, got, "Receipt", "event")
	wantField(t, got, "Read", "state")

	stored := h.messages.stored()
	if len(stored) != 2 || stored[0].MessageID != "A" || stored[1].MessageID != "B" {
		t.Fatalf("stored %+v", stored)
	}
	if stored[0].Status != "Read" || stored[0].Source != "5511999990001" {
		t.Errorf("stored %+v", stored[0])
	}
}

// A repeat changes nothing in the database, but the webhook still fires. Delivered
// receipts below behave differently: a repeat sends nothing at all.
func TestRepeatedReadReceiptIsStoredOnce(t *testing.T) {
	h := newEventHarness(t)
	h.one(t, receipt(t, types.ReceiptTypeRead, "A"))
	h.one(t, receipt(t, types.ReceiptTypeRead, "A"))
	if n := len(h.messages.stored()); n != 1 {
		t.Errorf("stored %d times", n)
	}
}

func TestReadSelfIsDeliveredButNotStored(t *testing.T) {
	h := newEventHarness(t)
	got := h.one(t, receipt(t, types.ReceiptTypeReadSelf, "A"))
	wantField(t, got, "ReadSelf", "state")
	if n := len(h.messages.stored()); n != 0 {
		t.Errorf("stored %d messages", n)
	}
}

func TestDeliveredReceiptIsDeduplicated(t *testing.T) {
	h := newEventHarness(t)
	got := h.one(t, receipt(t, types.ReceiptTypeDelivered, "A"))
	wantField(t, got, "Delivered", "state")
	h.none(t, receipt(t, types.ReceiptTypeDelivered, "A"))
	if n := len(h.messages.stored()); n != 1 {
		t.Errorf("stored %d times", n)
	}
}

func TestDeliveredReceiptUpdatesEveryMessage(t *testing.T) {
	h := newEventHarness(t)
	h.one(t, receipt(t, types.ReceiptTypeDelivered, "A", "B"))
	stored := h.messages.stored()
	if len(stored) != 2 || stored[0].MessageID != "A" || stored[1].MessageID != "B" {
		t.Fatalf("stored %+v", stored)
	}
	for _, m := range stored {
		if m.Status != "Delivered" || m.InstanceId != testInstanceID || m.Source != "5511999990001" {
			t.Errorf("stored %+v", m)
		}
	}
}

// A receipt mixing old and new IDs is news: only the old ones are skipped.
func TestPartlyRepeatedDeliveredReceipt(t *testing.T) {
	h := newEventHarness(t)
	h.one(t, receipt(t, types.ReceiptTypeDelivered, "A"))
	h.one(t, receipt(t, types.ReceiptTypeDelivered, "A", "B"))
	stored := h.messages.stored()
	if len(stored) != 2 || stored[1].MessageID != "B" {
		t.Fatalf("stored %+v", stored)
	}
}

func TestReceiptsThatAreDropped(t *testing.T) {
	groupReceipt := func(t *testing.T) *events.Receipt {
		r := receipt(t, types.ReceiptTypeRead, "A")
		r.Chat = mustJID(t, testGroup)
		return r
	}
	cases := []struct {
		name  string
		setup func(*eventHarness)
		event func(*testing.T) *events.Receipt
	}{
		{"no message ids", func(*eventHarness) {}, func(t *testing.T) *events.Receipt { return receipt(t, types.ReceiptTypeRead) }},
		{"other receipt type", func(*eventHarness) {}, func(t *testing.T) *events.Receipt { return receipt(t, types.ReceiptTypeRetry, "A") }},
		{"instance ignores groups", func(h *eventHarness) { h.cli.Instance.IgnoreGroups = true }, groupReceipt},
		{"global ignores groups", func(h *eventHarness) { h.cli.config.EventIgnoreGroup = true }, groupReceipt},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newEventHarness(t)
			tc.setup(h)
			h.none(t, tc.event(t))
			if n := len(h.messages.stored()); n != 0 {
				t.Errorf("stored %d messages", n)
			}
		})
	}
}

func TestReceiptsAreNotStoredWhenHistoryIsOff(t *testing.T) {
	h := newEventHarness(t)
	h.cli.config.DatabaseSaveMessages = false
	h.one(t, receipt(t, types.ReceiptTypeRead, "A"))
	h.one(t, receipt(t, types.ReceiptTypeDelivered, "B"))
	if n := len(h.messages.stored()); n != 0 {
		t.Errorf("stored %d messages", n)
	}
}

func TestPresence(t *testing.T) {
	from := mustJID(t, testContact)
	cases := []struct {
		name     string
		event    *events.Presence
		state    string
		lastSeen any
	}{
		{"online", &events.Presence{From: from}, "online", nil},
		{"offline", &events.Presence{From: from, Unavailable: true}, "offline", nil},
		{"offline with last seen", &events.Presence{From: from, Unavailable: true, LastSeen: testTime}, "offline", float64(testTime.Unix())},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newEventHarness(t)
			got := h.one(t, tc.event)
			wantField(t, got, testContact, "from")
			wantField(t, got, tc.state, "state")
			if field(got, "lastSeen") != tc.lastSeen {
				t.Errorf("lastSeen = %v, want %v", field(got, "lastSeen"), tc.lastSeen)
			}
		})
	}
}

func TestConnectedOpensTheInstance(t *testing.T) {
	h := newEventHarness(t)
	h.cli.Instance.DisconnectReason = "old reason"

	got := h.one(t, &events.Connected{})
	wantField(t, got, "Connected", "event")
	wantField(t, got, "open", "data", "status")
	wantField(t, got, testOwnJID, "data", "jid")
	wantField(t, got, "Test Bot", "data", "pushName")

	if !h.cli.Instance.Connected || h.cli.Instance.DisconnectReason != "" {
		t.Errorf("instance %+v", h.cli.Instance)
	}
	if len(h.instances.connected) != 1 || h.instances.connected[0] != (connectedUpdate{true, ""}) {
		t.Errorf("connected updates = %+v", h.instances.connected)
	}
	if len(h.instances.qrcodes) != 1 || h.instances.qrcodes[0] != "" {
		t.Errorf("qrcode updates = %+v", h.instances.qrcodes)
	}
}

func TestConnectedForgetsReconnectBackoff(t *testing.T) {
	h := newEventHarness(t)
	reconnectMu.Lock()
	reconnectTrack[testInstanceID] = &reconnectState{}
	reconnectMu.Unlock()

	h.one(t, &events.Connected{})

	reconnectMu.Lock()
	defer reconnectMu.Unlock()
	if _, tracked := reconnectTrack[testInstanceID]; tracked {
		t.Error("backoff survived a real connection")
	}
}

// Until WhatsApp sends the push name there is nothing to announce:
// PushNameSetting finishes the job.
func TestConnectedWaitsForAPushName(t *testing.T) {
	h := newEventHarness(t)
	h.cli.WAClient.Store.PushName = ""
	h.none(t, &events.Connected{})
	if len(h.instances.connected) != 0 {
		t.Errorf("connected updates = %+v", h.instances.connected)
	}

	h.cli.WAClient.Store.PushName = "Test Bot"
	got := h.one(t, &events.PushNameSetting{})
	wantField(t, got, "Connected", "event")
}

func TestPairSuccessRecordsTheDevice(t *testing.T) {
	h := newEventHarness(t)
	h.cli.passkeyCeremony.Start(testInstanceID, []byte(`{}`))

	got := h.one(t, &events.PairSuccess{ID: mustJID(t, testOwnJID), Platform: "android"})
	wantField(t, got, "PairSuccess", "event")
	wantField(t, got, "open", "data", "status")
	wantField(t, got, testOwnJID, "data", "jid")
	wantField(t, got, "Test Bot", "data", "pushName")

	if len(h.instances.settings) != 1 {
		t.Fatalf("settings updates = %+v", h.instances.settings)
	}
	update := h.instances.settings[0]
	if update["jid"] != testOwnJID || update["connected"] != true || update["auto_reconnect"] != true || update["qrcode"] != "" {
		t.Errorf("update = %+v", update)
	}
	if h.cli.Instance.Jid != testOwnJID || !h.cli.Instance.Connected {
		t.Errorf("instance %+v", h.cli.Instance)
	}
	if h.cli.passkeyCeremony.HasActiveByInstance(testInstanceID) {
		t.Error("passkey ceremony survived pairing")
	}
}

func TestPairError(t *testing.T) {
	h := newEventHarness(t)
	h.cli.passkeyCeremony.Start(testInstanceID, []byte(`{}`))
	got := h.one(t, &events.PairError{Platform: "android", Error: errors.New("store failed")})
	wantField(t, got, "PairError", "event")
	wantField(t, got, "store failed", "data", "error")
	wantField(t, got, "error", "data", "stage")

	_, state, ok := h.cli.passkeyCeremony.StateByInstance(testInstanceID)
	if !ok || state.Error != "store failed" {
		t.Errorf("ceremony state = %+v, %v", state, ok)
	}
}

func TestPasskeyRequestStartsACeremony(t *testing.T) {
	t.Setenv("PASSKEY_PUBLIC_URL", "https://wamux.example")
	h := newEventHarness(t)
	got := h.one(t, &events.PairPasskeyRequest{})
	wantField(t, got, "PasskeyRequest", "event")
	wantField(t, got, "challenge", "data", "stage")

	token, _, ok := h.cli.passkeyCeremony.StateByInstance(testInstanceID)
	if !ok {
		t.Fatal("no ceremony started")
	}
	wantField(t, got, token, "data", "ceremonyToken")
}

// The confirmation is always manual, whatever the server suggests.
func TestPasskeyConfirmationIsNeverSkipped(t *testing.T) {
	h := newEventHarness(t)
	h.cli.passkeyCeremony.Start(testInstanceID, []byte(`{}`))
	got := h.one(t, &events.PairPasskeyConfirmation{Code: "123456", SkipHandoffUX: true})
	wantField(t, got, "PasskeyConfirmation", "event")
	wantField(t, got, "123456", "data", "code")

	_, state, ok := h.cli.passkeyCeremony.StateByInstance(testInstanceID)
	if !ok || state.SkipHandoffUX {
		t.Errorf("ceremony state = %+v, %v", state, ok)
	}
}

func TestPasskeyError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"with a cause", errors.New("rejected"), "rejected"},
		{"without a cause", nil, "unknown passkey error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newEventHarness(t)
			got := h.one(t, &events.PairPasskeyError{Error: tc.err})
			wantField(t, got, "PasskeyError", "event")
			wantField(t, got, tc.want, "data", "error")
		})
	}
}

func TestMediaRetryGoesToTheService(t *testing.T) {
	h := newEventHarness(t)
	evt := &events.MediaRetry{MessageID: "MSG1"}
	h.none(t, evt)
	select {
	case got := <-h.svc.mediaRetries:
		if got != evt {
			t.Errorf("got %+v", got)
		}
	case <-testTimeout():
		t.Fatal("media retry not forwarded")
	}
}

func TestStreamReplacedClearsThePasskeyCeremony(t *testing.T) {
	h := newEventHarness(t)
	h.cli.passkeyCeremony.Start(testInstanceID, []byte(`{}`))
	h.none(t, &events.StreamReplaced{})
	if h.cli.passkeyCeremony.HasActiveByInstance(testInstanceID) {
		t.Error("ceremony survived")
	}
}

func TestTemporaryBan(t *testing.T) {
	h := newEventHarness(t)
	got := h.one(t, &events.TemporaryBan{Code: events.TempBanSentToTooManyPeople, Expire: time.Hour})
	wantField(t, got, "TemporaryBan", "event")
	wantField(t, got, events.TempBanSentToTooManyPeople.String(), "data", "reason")
	wantField(t, got, float64(time.Hour), "data", "expire")
}

func TestLoggedOutClosesTheInstance(t *testing.T) {
	h := newEventHarness(t)
	h.cli.Instance.Jid = testOwnJID
	h.cli.Instance.Connected = true
	h.cli.userInfoCache.Set(testToken, "cached", 0)

	h.cli.processEvent(&events.LoggedOut{Reason: events.ConnectFailureLoggedOut})
	got := h.deliveries(t, 1)[0].event(t)
	wantField(t, got, "LoggedOut", "event")
	wantField(t, got, events.ConnectFailureLoggedOut.String(), "data", "reason")

	if h.cli.Instance.Connected || h.cli.Instance.Jid != "" {
		t.Errorf("instance %+v", h.cli.Instance)
	}
	if h.cli.Instance.DisconnectReason != events.ConnectFailureLoggedOut.String() {
		t.Errorf("reason = %q", h.cli.Instance.DisconnectReason)
	}
	if len(h.instances.jids) != 1 || h.instances.jids[0] != "" {
		t.Errorf("jid updates = %+v", h.instances.jids)
	}
	if _, found := h.cli.userInfoCache.Get(testToken); found {
		t.Error("user cache not cleared")
	}
	if !h.cli.kill.Fired() || h.cli.kill.Mode() != killswitch.Restart {
		t.Errorf("kill fired=%v", h.cli.kill.Fired())
	}
}

func TestLoggedOutReachesGlobalQueuesOnce(t *testing.T) {
	h := newEventHarness(t)
	h.cli.config.AmqpGlobalEnabled = true
	h.one(t, &events.LoggedOut{Reason: events.ConnectFailureLoggedOut})
	select {
	case got := <-h.svc.globals:
		if got != "LoggedOut" {
			t.Errorf("global event = %q", got)
		}
	case <-testTimeout():
		t.Fatal("nothing reached the global queues")
	}
	select {
	case <-h.svc.globals:
		t.Fatal("sent to the global queues twice")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestConnectFailureMarksTheInstanceDown(t *testing.T) {
	h := newEventHarness(t)
	h.cli.Instance.Connected = true
	h.cli.userInfoCache.Set(testToken, "cached", 0)

	got := h.one(t, &events.ConnectFailure{Reason: events.ConnectFailureMainDeviceGone})
	wantField(t, got, "ConnectFailure", "event")

	want := connectedUpdate{false, events.ConnectFailureMainDeviceGone.String()}
	if len(h.instances.connected) != 1 || h.instances.connected[0] != want {
		t.Errorf("updates = %+v", h.instances.connected)
	}
	if _, found := h.cli.userInfoCache.Get(testToken); found {
		t.Error("user cache not cleared")
	}
}

func (h *eventHarness) wantReconnect(t *testing.T) {
	t.Helper()
	select {
	case id := <-h.svc.reconnects:
		if id != testInstanceID {
			t.Errorf("reconnected %q", id)
		}
	case <-testTimeout():
		t.Fatal("no reconnect")
	}
}

func TestDisconnectedReconnects(t *testing.T) {
	h := newEventHarness(t)
	h.cli.Instance.Connected = true
	h.cli.passkeyCeremony.Start(testInstanceID, []byte(`{}`))

	got := h.one(t, &events.Disconnected{})
	wantField(t, got, "Disconnected", "event")
	h.wantReconnect(t)

	if h.cli.Instance.Connected {
		t.Error("instance still connected")
	}
	if len(h.instances.connected) != 1 || h.instances.connected[0].connected {
		t.Errorf("updates = %+v", h.instances.connected)
	}
	if h.cli.passkeyCeremony.HasActiveByInstance(testInstanceID) {
		t.Error("ceremony survived")
	}
}

func TestStreamErrorReconnectsWithoutAWebhook(t *testing.T) {
	h := newEventHarness(t)
	h.none(t, &events.StreamError{Code: "503"})
	h.wantReconnect(t)
}

// One missed ping is a blip; only the second drops the socket.
func TestKeepAliveTimeoutWaitsForASecondMiss(t *testing.T) {
	h := newEventHarness(t)
	h.none(t, &events.KeepAliveTimeout{ErrorCount: 1})
	select {
	case <-h.svc.reconnects:
		t.Fatal("reconnected after one miss")
	case <-time.After(50 * time.Millisecond):
	}
	h.none(t, &events.KeepAliveTimeout{ErrorCount: 2})
	h.wantReconnect(t)
}

func TestCallOfferIsDelivered(t *testing.T) {
	h := newEventHarness(t)
	got := h.one(t, &events.CallOffer{BasicCallMeta: types.BasicCallMeta{CallID: "C1"}})
	wantField(t, got, "CallOffer", "event")
}

func TestRejectedCallSendsNoWebhook(t *testing.T) {
	h := newEventHarness(t)
	h.cli.Instance.RejectCall = true
	// MsgRejectCall stays empty: sending needs device stores this client lacks.
	h.none(t, &events.CallOffer{BasicCallMeta: types.BasicCallMeta{
		CallID:      "C1",
		CallCreator: mustJID(t, testContact),
	}})
}

func TestLabelEditIsStored(t *testing.T) {
	h := newEventHarness(t)
	got := h.one(t, &events.LabelEdit{
		LabelID: "7",
		Action:  &waSyncAction.LabelEditAction{Name: proto.String("VIP"), Color: proto.Int32(3), PredefinedID: proto.Int32(2)},
	})
	wantField(t, got, "LabelEdit", "event")
	if len(h.labels.labels) != 1 {
		t.Fatalf("labels = %+v", h.labels.labels)
	}
	l := h.labels.labels[0]
	if l.InstanceID != testInstanceID || l.LabelID != "7" || l.LabelName != "VIP" || l.LabelColor != "3" || l.PredefinedId != "2" {
		t.Errorf("label = %+v", l)
	}
}

func TestArchiveCarriesTheChat(t *testing.T) {
	h := newEventHarness(t)
	got := h.one(t, &events.Archive{
		JID:       mustJID(t, testContact),
		Timestamp: testTime,
		Action:    &waSyncAction.ArchiveChatAction{Archived: proto.Bool(true)},
	})
	wantField(t, got, "Archive", "event")
	wantField(t, got, testContact, "data", "JID")
	wantField(t, got, true, "data", "Action", "archived")
}

func TestAccountTimelock(t *testing.T) {
	h := newEventHarness(t)
	got := h.one(t, &events.NotifyAccountReachoutTimelock{IsActive: true, TimeEnforcementEnds: jsontime.UnixString{Time: testTime}})
	wantField(t, got, "AccountReachoutTimelock", "event")
	wantField(t, got, true, "data", "isActive")
	wantField(t, got, float64(testTime.Unix()), "data", "timeEnforcementEnds")
}

func TestBlocklistChanges(t *testing.T) {
	h := newEventHarness(t)
	got := h.one(t, &events.Blocklist{
		Action: events.BlocklistActionDefault,
		DHash:  "hash",
		Changes: []events.BlocklistChange{{
			JID:    mustJID(t, testContact),
			Action: events.BlocklistChangeActionBlock,
		}},
	})
	changes, ok := field(got, "data", "changes").([]any)
	if !ok || len(changes) != 1 {
		t.Fatalf("changes = %v", field(got, "data", "changes"))
	}
	wantField(t, changes[0].(map[string]any), testContact, "jid")
	wantField(t, got, "hash", "data", "dhash")
}

func TestPassThroughEvents(t *testing.T) {
	cases := map[string]interface{}{
		"ChatPresence":            &events.ChatPresence{},
		"CallAccept":              &events.CallAccept{},
		"CallTerminate":           &events.CallTerminate{},
		"CallOfferNotice":         &events.CallOfferNotice{},
		"CallRelayLatency":        &events.CallRelayLatency{},
		"OfflineSyncCompleted":    &events.OfflineSyncCompleted{},
		"PrivacySettings":         &events.PrivacySettings{},
		"NewsletterLiveUpdate":    &events.NewsletterLiveUpdate{},
		"NewsletterMuteChange":    &events.NewsletterMuteChange{},
		"LabelAssociationChat":    &events.LabelAssociationChat{},
		"LabelAssociationMessage": &events.LabelAssociationMessage{},
		"Contact":                 &events.Contact{},
		"PushName":                &events.PushName{},
		"Picture":                 &events.Picture{},
		"UserAbout":               &events.UserAbout{},
		"GroupInfo":               &events.GroupInfo{},
		"JoinedGroup":             &events.JoinedGroup{},
		"NewsletterJoin":          &events.NewsletterJoin{},
		"NewsletterLeave":         &events.NewsletterLeave{},
		"HistorySync":             &events.HistorySync{Data: &waHistorySync.HistorySync{}},
	}
	for name, evt := range cases {
		t.Run(name, func(t *testing.T) {
			h := newEventHarness(t)
			got := h.one(t, evt)
			wantField(t, got, name, "event")
			wantField(t, got, testInstanceID, "instanceId")
		})
	}
}

func TestEventsWithNoWebhook(t *testing.T) {
	cases := map[string]interface{}{
		"AppState":               &events.AppState{},
		"AppStateSyncComplete":   &events.AppStateSyncComplete{Name: appstate.WAPatchRegular},
		"app state recovery":     &events.AppStateSyncComplete{Name: appstate.WAPatchCriticalBlock, Recovery: true},
		"IdentityChange":         &events.IdentityChange{},
		"undecryptable, ignored": &events.UndecryptableMessage{Info: types.MessageInfo{ID: "ABC"}},
		"unhandled type":         struct{}{},
	}
	for name, evt := range cases {
		t.Run(name, func(t *testing.T) {
			newEventHarness(t).none(t, evt)
		})
	}
}

func TestViewOnceUndecryptableIsForwardedAsMessage(t *testing.T) {
	h := newEventHarness(t)
	got := h.one(t, &events.UndecryptableMessage{
		Info:            types.MessageInfo{ID: "ABC"},
		UnavailableType: "view_once",
	})
	wantField(t, got, "Message", "event")
	wantField(t, got, "ABC", "data", "Info", "ID")
}

// With a pre-pairing client there is a QR to show. The first code is enough here:
// rotation sleeps for a minute between codes.
func TestQRMaxCountEndsPairing(t *testing.T) {
	h := newEventHarness(t)
	h.cli.WAClient.Store.ID = nil
	h.cli.config.QrcodeMaxCount = 1

	h.cli.processEvent(&events.QR{Codes: []string{"code-1"}})
	got := h.deliveries(t, 1)[0].event(t)
	wantField(t, got, "QRTimeout", "event")
	wantField(t, got, "Maximum QR code count (1) reached", "data", "reason")

	if !h.cli.kill.Fired() || h.cli.kill.Mode() != killswitch.Restart {
		t.Errorf("kill fired=%v", h.cli.kill.Fired())
	}
}

func TestQRIsIgnoredOncePaired(t *testing.T) {
	newEventHarness(t).none(t, &events.QR{Codes: []string{"code-1"}})
}
