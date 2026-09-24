// Package docmodels holds documentation-only structs.
//
// None of these types are used at runtime. They exist purely so swaggo/swag can
// render realistic response payloads (field examples, nested shapes) in the
// generated Swagger document. Handlers keep returning gin.H; the @Success /
// @Failure annotations reference these types via model composition, e.g.
//
//	// @Success 200 {object} docmodels.SendResponse{data=docmodels.MessageSend}
//
// Every API response in this project is wrapped as {"message": "...", "data": ...}
// (success) or {"error": "..."} (failure), which Envelope and ErrorResponse model.
package docmodels

// Envelope is the generic success wrapper: {"message":"success","data":<elem>}.
// Use it via composition, overriding data with the concrete payload type:
//
//	@Success 200 {object} docmodels.Envelope{data=docmodels.Instance}
type Envelope struct {
	Data    interface{} `json:"data"`
	Message string      `json:"message" example:"success"`
}

// ErrorResponse is the failure wrapper returned by every handler.
type ErrorResponse struct {
	Error string `json:"error" example:"phone number is required"`
}

// MessageSend mirrors send_service.MessageSendStruct. The real struct embeds
// whatsmeow protobuf types, which render as a huge, unreadable model; this
// documentation type shows only the fields clients actually read.
type MessageSend struct {
	Info    MessageInfo `json:"Info"`
	Message MessageBody `json:"Message"`
}

// MessageInfo is the subset of WhatsApp message metadata clients consume.
type MessageInfo struct {
	ID        string `json:"ID" example:"3EB0A1B2C3D4E5F6A7B8C9"`
	Chat      string `json:"Chat" example:"5511999999999@s.whatsapp.net"`
	Sender    string `json:"Sender" example:"5511999999999@s.whatsapp.net"`
	IsFromMe  bool   `json:"IsFromMe" example:"true"`
	IsGroup   bool   `json:"IsGroup" example:"false"`
	Timestamp string `json:"Timestamp" example:"2026-01-15T10:30:00Z"`
}

// MessageBody is the outgoing message content (text shown for a text send).
type MessageBody struct {
	Conversation string `json:"conversation" example:"Ola, tudo bem?"`
}

// Instance mirrors the instance model returned by the instance endpoints.
type Instance struct {
	ID               string `json:"id" example:"11111111-2222-3333-4444-555555555555"`
	Name             string `json:"name" example:"Minha Instancia"`
	Token            string `json:"token" example:"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"`
	Webhook          string `json:"webhook" example:"https://webhook.example.com/evolution"`
	RabbitmqEnable   string `json:"rabbitmqEnable" example:""`
	WebsocketEnable  string `json:"websocketEnable" example:""`
	NatsEnable       string `json:"natsEnable" example:""`
	JID              string `json:"jid" example:"5511999999999:7@s.whatsapp.net"`
	QRCode           string `json:"qrcode" example:""`
	Connected        bool   `json:"connected" example:"true"`
	Expiration       int    `json:"expiration" example:"0"`
	DisconnectReason string `json:"disconnect_reason" example:""`
	Events           string `json:"events" example:"MESSAGE"`
	OSName           string `json:"os_name" example:"Evolution GO"`
	Proxy            string `json:"proxy" example:""`
	ClientName       string `json:"client_name" example:"evolution"`
	CreatedAt        string `json:"createdAt" example:"2026-01-15T10:30:00.000000-03:00"`
	AlwaysOnline     bool   `json:"alwaysOnline" example:"false"`
	RejectCall       bool   `json:"rejectCall" example:"false"`
	MsgRejectCall    string `json:"msgRejectCall" example:""`
	ReadMessages     bool   `json:"readMessages" example:"false"`
	IgnoreGroups     bool   `json:"ignoreGroups" example:"false"`
	IgnoreStatus     bool   `json:"ignoreStatus" example:"false"`
}

// ConnectionStatus is the /instance/status payload.
type ConnectionStatus struct {
	Connected bool   `json:"Connected" example:"true"`
	LoggedIn  bool   `json:"LoggedIn" example:"true"`
	Name      string `json:"Name" example:"Minha Instancia"`
}

// QRCode is the /instance/qr payload (a fresh pairing code).
type QRCode struct {
	QRCode string `json:"qrcode" example:"2@AbCdEfGhIjKlMnOpQrStUvWxYz0123456789+/=,AbCdEfGhIjKlMnOpQrStUvWxYz0123456789+/=,AbCdEfGhIjKlMnOpQrStUvWxYz0123456789+/=,AbCdEfGhIjKlMnOpQrStUvWxYz0123456789+/="`
}

// PairResult is the /instance/pair payload (pairing code for a phone number).
type PairResult struct {
	PairingCode string `json:"pairingCode" example:"ABCD-EFGH"`
}

// Limits is the /instance/limits payload.
type Limits struct {
	ReachoutTimelock ReachoutTimelock `json:"reachoutTimelock"`
	NewChatCapping   NewChatCapping   `json:"newChatCapping"`
}

// ReachoutTimelock describes the reachout timelock state.
type ReachoutTimelock struct {
	IsActive            bool   `json:"isActive" example:"false"`
	TimeEnforcementEnds int64  `json:"timeEnforcementEnds" example:"0"`
	EnforcementType     string `json:"enforcementType" example:""`
}

// NewChatCapping describes the new-chat capping quota.
type NewChatCapping struct {
	CappingStatus string `json:"cappingStatus" example:"NONE"`
	TotalQuota    int    `json:"totalQuota" example:"0"`
	UsedQuota     int    `json:"usedQuota" example:"0"`
	CycleEnds     int64  `json:"cycleEnds" example:"1"`
}

// InstanceOverview is the /instance/overview payload.
type InstanceOverview struct {
	Connected     bool   `json:"connected" example:"true"`
	Platform      string `json:"platform" example:"android"`
	ProfileName   string `json:"profileName" example:"Minha Instancia"`
	ProfilePicURL string `json:"profilePicUrl" example:"https://pps.whatsapp.net/v/t61.24694-24/example.jpg"`
	ContactsCount int    `json:"contactsCount" example:"344"`
	ChatsCount    int    `json:"chatsCount" example:"3"`
	MessagesCount int    `json:"messagesCount" example:"19"`
}

// LogEntry is one element of the /instance/logs array.
type LogEntry struct {
	Timestamp  string `json:"timestamp" example:"2026-01-15T10:30:00.676Z"`
	Level      string `json:"level" example:"INFO"`
	InstanceID string `json:"instance_id" example:"11111111-2222-3333-4444-555555555555"`
	Message    string `json:"message" example:"Client successfully validated - Connected: true"`
}

// ProxySet is the /instance/proxy (POST) payload. The password is never echoed;
// hasAuth reports whether a username+password pair was stored.
type ProxySet struct {
	Protocol string `json:"protocol" example:"http"`
	Host     string `json:"host" example:"proxy.example.com"`
	Port     string `json:"port" example:"8080"`
	HasAuth  bool   `json:"hasAuth" example:"true"`
}

// ProxyGet is the /instance/proxy (GET) payload when a proxy is configured. The
// password is never returned; hasPassword reports whether one is stored.
type ProxyGet struct {
	Protocol    string `json:"protocol" example:"http"`
	Host        string `json:"host" example:"proxy.example.com"`
	Port        string `json:"port" example:"8080"`
	Username    string `json:"username" example:"proxyuser"`
	HasPassword bool   `json:"hasPassword" example:"true"`
}

// -----------------------------------------------------------------------------
// User
// -----------------------------------------------------------------------------

// Contact is one element of GET /user/contacts.
type Contact struct {
	JID          string `json:"Jid" example:"5511999999999@s.whatsapp.net"`
	Found        bool   `json:"Found" example:"true"`
	FirstName    string `json:"FirstName" example:"Alice"`
	FullName     string `json:"FullName" example:"Alice Souza"`
	PushName     string `json:"PushName" example:"Alice"`
	BusinessName string `json:"BusinessName" example:""`
}

// UserInfo is one entry of the /user/info Users map.
type UserInfo struct {
	VerifiedName interface{} `json:"VerifiedName"`
	Status       string      `json:"Status" example:"Disponivel"`
	PictureID    string      `json:"PictureID" example:"1062917621"`
	PictureURL   string      `json:"PictureURL" example:"https://pps.whatsapp.net/v/t61.24694-24/example.jpg"`
	Devices      []string    `json:"Devices" example:"5511999999999@s.whatsapp.net,5511999999999:38@s.whatsapp.net"`
	LID          string      `json:"LID" example:"1234567890@lid"`
}

// UserCollection is the /user/info data payload.
type UserCollection struct {
	Users map[string]UserInfo `json:"Users"`
}

// CheckUser is one element of /user/check.
type CheckUser struct {
	Query        string      `json:"Query" example:"+5511999999999@s.whatsapp.net"`
	IsInWhatsapp bool        `json:"IsInWhatsapp" example:"true"`
	JID          string      `json:"JID" example:"1234567890@lid"`
	RemoteJID    string      `json:"RemoteJID" example:"1234567890@lid"`
	LID          interface{} `json:"LID"`
	VerifiedName string      `json:"VerifiedName" example:""`
}

// CheckUserCollection is the /user/check data payload.
type CheckUserCollection struct {
	Users []CheckUser `json:"Users"`
}

// Avatar is the /user/avatar payload (WhatsApp profile-picture info).
type Avatar struct {
	URL        string `json:"url" example:"https://pps.whatsapp.net/v/t61.24694-24/example.jpg"`
	ID         string `json:"id" example:"1062917621"`
	Type       string `json:"type" example:"image"`
	DirectPath string `json:"direct_path" example:"/v/t61.24694-24/example.jpg"`
	Hash       string `json:"hash" example:"mprE5+0w7jtC7PY4NhW3O2qZpvyOo9T+n8kgPbY7L9s="`
}

// Privacy is the /user/privacy payload.
type Privacy struct {
	GroupAdd     string `json:"GroupAdd" example:"all"`
	LastSeen     string `json:"LastSeen" example:"contacts"`
	Status       string `json:"Status" example:"contacts"`
	Profile      string `json:"Profile" example:"all"`
	ReadReceipts string `json:"ReadReceipts" example:"all"`
	CallAdd      string `json:"CallAdd" example:"all"`
	Online       string `json:"Online" example:"all"`
	Messages     string `json:"Messages" example:"all"`
	Defense      string `json:"Defense" example:"off"`
	Stickers     string `json:"Stickers" example:"contacts"`
}

// Blocklist is the /user/blocklist and block/unblock payload.
type Blocklist struct {
	DHash string   `json:"DHash" example:""`
	JIDs  []string `json:"JIDs" example:"5511999999999@s.whatsapp.net"`
}

// ResolveLid is the /user/lid payload.
type ResolveLid struct {
	LID         string `json:"lid" example:"1234567890@lid"`
	PhoneNumber string `json:"phoneNumber" example:"5511999999999"`
	JID         string `json:"jid" example:"5511999999999@s.whatsapp.net"`
}

// ProfilePicture is the /user/profilePicture payload.
type ProfilePicture struct {
	Image string `json:"image" example:"data:image/jpeg;base64,/9j/4AAQSkZJRgABAQ..." `
}

// ProfileName is the /user/profileName payload.
type ProfileName struct {
	Name string `json:"name" example:"Minha Loja"`
}

// ProfileStatus is the /user/profileStatus payload.
type ProfileStatus struct {
	Status string `json:"status" example:"Disponivel para atendimento"`
}

// -----------------------------------------------------------------------------
// Group
// -----------------------------------------------------------------------------

// GroupParticipant is one member of a group.
type GroupParticipant struct {
	JID          string      `json:"JID" example:"1234567890@lid"`
	PhoneNumber  string      `json:"PhoneNumber" example:"5511999999999@s.whatsapp.net"`
	LID          string      `json:"LID" example:"1234567890@lid"`
	IsAdmin      bool        `json:"IsAdmin" example:"true"`
	IsSuperAdmin bool        `json:"IsSuperAdmin" example:"false"`
	DisplayName  string      `json:"DisplayName" example:""`
	Error        int         `json:"Error" example:"0"`
	AddRequest   interface{} `json:"AddRequest"`
}

// Group is a single group as returned by /group/list, /group/info and /group/myall.
type Group struct {
	JID                           string             `json:"JID" example:"120363000000000000@g.us"`
	OwnerJID                      string             `json:"OwnerJID" example:"1234567890@lid"`
	OwnerPN                       string             `json:"OwnerPN" example:"5511999999999@s.whatsapp.net"`
	Name                          string             `json:"Name" example:"Equipe Vendas"`
	NameSetAt                     string             `json:"NameSetAt" example:"2026-01-10T20:21:38-03:00"`
	NameSetBy                     string             `json:"NameSetBy" example:"1234567890@lid"`
	NameSetByPN                   string             `json:"NameSetByPN" example:"5511999999999@s.whatsapp.net"`
	Topic                         string             `json:"Topic" example:""`
	TopicID                       string             `json:"TopicID" example:""`
	TopicSetAt                    string             `json:"TopicSetAt" example:"0001-01-01T00:00:00Z"`
	TopicSetBy                    string             `json:"TopicSetBy" example:""`
	TopicSetByPN                  string             `json:"TopicSetByPN" example:""`
	TopicDeleted                  bool               `json:"TopicDeleted" example:"false"`
	IsLocked                      bool               `json:"IsLocked" example:"false"`
	IsAnnounce                    bool               `json:"IsAnnounce" example:"false"`
	AnnounceVersionID             string             `json:"AnnounceVersionID" example:"1790119298733550"`
	IsEphemeral                   bool               `json:"IsEphemeral" example:"true"`
	DisappearingTimer             int                `json:"DisappearingTimer" example:"0"`
	IsIncognito                   bool               `json:"IsIncognito" example:"false"`
	IsParent                      bool               `json:"IsParent" example:"false"`
	DefaultMembershipApprovalMode string             `json:"DefaultMembershipApprovalMode" example:""`
	LinkedParentJID               string             `json:"LinkedParentJID" example:""`
	IsDefaultSubGroup             bool               `json:"IsDefaultSubGroup" example:"false"`
	IsJoinApprovalRequired        bool               `json:"IsJoinApprovalRequired" example:"false"`
	AddressingMode                string             `json:"AddressingMode" example:"lid"`
	GroupCreated                  string             `json:"GroupCreated" example:"2026-01-10T20:21:38-03:00"`
	CreatorCountryCode            string             `json:"CreatorCountryCode" example:"BR"`
	ParticipantVersionID          string             `json:"ParticipantVersionID" example:"1790119298733550"`
	Participants                  []GroupParticipant `json:"Participants"`
	ParticipantCount              int                `json:"ParticipantCount" example:"2"`
	MemberAddMode                 string             `json:"MemberAddMode" example:"all_member_add"`
	Suspended                     bool               `json:"Suspended" example:"false"`
}

// GroupCreateResult is the /group/create payload.
type GroupCreateResult struct {
	JID    string   `json:"jid" example:"120363000000000000@g.us"`
	Name   string   `json:"name" example:"Equipe Vendas"`
	Owner  string   `json:"owner" example:"5511999999999@s.whatsapp.net"`
	Added  []string `json:"added" example:"5511999999999@s.whatsapp.net,5511888888888@s.whatsapp.net"`
	Failed []string `json:"failed" example:""`
}

// -----------------------------------------------------------------------------
// Chat / Message
// -----------------------------------------------------------------------------

// ChatActionResult is the generic payload returned by chat actions
// (pin/unpin/archive/unarchive/mute/unmute). The timestamp is a Go time value
// serialized into the JSON string below; it is the zero time when the action
// does not return one.
type ChatActionResult struct {
	Timestamp string `json:"timestamp" example:"0001-01-01 00:00:00 +0000 UTC"`
}

// HistorySyncResult is the /chat/history-sync payload: the receipt of the
// on-demand history-sync request sent to the device.
type HistorySyncResult struct {
	ID        string `json:"ID" example:"3EB0A1B2C3D4E5F6A7B8C9"`
	Timestamp string `json:"Timestamp" example:"2026-01-15T10:30:00-03:00"`
}

// MessageActionResult is the payload returned by /message/presence,
// /message/markread and /message/markplayed. The timestamp is a Go time value
// serialized into a JSON string (the zero time when unavailable).
type MessageActionResult struct {
	Timestamp string `json:"timestamp" example:"0001-01-01 00:00:00 +0000 UTC"`
}

// MessageMutationResult is the payload returned by /message/delete and
// /message/edit: the (possibly new) message id and a Go time string.
type MessageMutationResult struct {
	MessageID string `json:"messageId" example:"3EB0A1B2C3D4E5F6A7B8C9"`
	Timestamp string `json:"timestamp" example:"2026-01-15 10:31:00 -0300 -03"`
}

// MessageStatus is the /message/status payload: the stored message row plus the
// time the receipt was served.
type MessageStatus struct {
	Result    MessageStatusRow `json:"result"`
	Timestamp string           `json:"timestamp" example:"0001-01-01 00:00:00 +0000 UTC"`
}

// MessageStatusRow is the persisted message record returned by /message/status.
type MessageStatusRow struct {
	ID         string `json:"id" example:"57c59a33-33f3-4e30-9b0d-07536b5f8e4b"`
	MessageID  string `json:"message_id" example:"3EB0A1B2C3D4E5F6A7B8C9"`
	InstanceID string `json:"instance_id" example:"11111111-2222-3333-4444-555555555555"`
	Timestamp  string `json:"timestamp" example:"2026-01-15 10:30:00"`
	Status     string `json:"status" example:"Sent"`
	Source     string `json:"source" example:"1234567890"`
}

// DownloadMedia is the /message/downloadmedia payload: the media bytes as a
// data URL plus the media type.
type DownloadMedia struct {
	Base64 string `json:"base64" example:"data:image/jpeg;base64,/9j/4AAQSkZJRgABAQ..."`
}

// -----------------------------------------------------------------------------
// Label
// -----------------------------------------------------------------------------

// Label is one element of GET /label/list.
type Label struct {
	ID           string `json:"id" example:"1"`
	Name         string `json:"name" example:"Cliente VIP"`
	Color        int32  `json:"color" example:"1"`
	PredefinedID string `json:"predefinedId" example:""`
	Deleted      bool   `json:"deleted" example:"false"`
}

// -----------------------------------------------------------------------------
// Community
// -----------------------------------------------------------------------------

// Community is the /community/create payload.
type Community struct {
	JID  string `json:"JID" example:"120363000000000000@g.us"`
	Name string `json:"Name" example:"Minha Comunidade"`
}

// CommunityMutation is the /community/add and /community/remove payload.
type CommunityMutation struct {
	JID          string `json:"JID" example:"120363000000000000@g.us"`
	Participants []struct {
		JID   string `json:"JID" example:"120363000000000001@g.us"`
		Error int    `json:"Error" example:"0"`
	} `json:"Participants"`
}

// -----------------------------------------------------------------------------
// Newsletter
// -----------------------------------------------------------------------------

// Newsletter is a newsletter object returned by the newsletter endpoints
// (create/info/link). The shape mirrors WhatsApp's NewsletterMetadata.
type Newsletter struct {
	ID             string                   `json:"id" example:"120363000000000000@newsletter"`
	State          NewsletterState          `json:"state"`
	ThreadMetadata NewsletterThreadMetadata `json:"thread_metadata"`
	ViewerMetadata NewsletterViewerMetadata `json:"viewer_metadata"`
}

// NewsletterState is the newsletter lifecycle state.
type NewsletterState struct {
	Type string `json:"type" example:"active"`
}

// NewsletterThreadMetadata is the public metadata of a newsletter.
type NewsletterThreadMetadata struct {
	CreationTime     string              `json:"creation_time" example:"1790227149"`
	Invite           string              `json:"invite" example:"0029Vb8UfYvBadmcTuuhZC28"`
	Name             NewsletterTextField `json:"name"`
	Description      NewsletterTextField `json:"description"`
	SubscribersCount string              `json:"subscribers_count" example:"0"`
	Verification     string              `json:"verification" example:"unverified"`
	Picture          NewsletterMedia     `json:"picture"`
	Preview          NewsletterMedia     `json:"preview"`
	Settings         NewsletterSettings  `json:"settings"`
}

// NewsletterTextField is an editable text field plus its version metadata.
type NewsletterTextField struct {
	Text       string `json:"text" example:"Novidades da Loja"`
	ID         string `json:"id" example:"1790227149344380"`
	UpdateTime string `json:"update_time" example:"1790227149344380"`
}

// NewsletterMedia is a newsletter picture/preview descriptor.
type NewsletterMedia struct {
	URL        string      `json:"url" example:""`
	ID         string      `json:"id" example:"1790227157396014"`
	Type       string      `json:"type" example:"IMAGE"`
	DirectPath string      `json:"direct_path" example:""`
	Hash       interface{} `json:"hash"`
}

// NewsletterSettings holds newsletter feature settings.
type NewsletterSettings struct {
	ReactionCodes struct {
		Value string `json:"value" example:"ALL"`
	} `json:"reaction_codes"`
}

// NewsletterViewerMetadata is the caller's relationship to the newsletter.
type NewsletterViewerMetadata struct {
	Mute string `json:"mute" example:"on"`
	Role string `json:"role" example:"owner"`
}

// NewsletterMessage is one element of /newsletter/messages.
type NewsletterMessage struct {
	ID        string `json:"id" example:"3EB0A1B2C3D4E5F6A7B8C9"`
	ServerID  int64  `json:"serverId" example:"123456789"`
	Timestamp string `json:"timestamp" example:"2026-01-15T10:30:00-03:00"`
	Message   struct {
		Conversation string `json:"conversation" example:"Confira nossa promocao de hoje!"`
	} `json:"message"`
}

// -----------------------------------------------------------------------------
// Typebot
// -----------------------------------------------------------------------------

// TypebotStatusChange is the /typebot/changeStatus payload.
type TypebotStatusChange struct {
	Success   bool   `json:"success" example:"true"`
	RemoteJID string `json:"remoteJid" example:"5511999999999@s.whatsapp.net"`
	Status    string `json:"status" example:"paused"`
}

// TypebotSuccess is the bare {"success":true} payload returned by the Typebot
// delete endpoints and by PUT /typebot/sessions/{id}/status.
type TypebotSuccess struct {
	Success bool `json:"success" example:"true"`
}

// -----------------------------------------------------------------------------
// Server
// -----------------------------------------------------------------------------

// ServerOK is the /server/ok payload.
type ServerOK struct {
	Status string `json:"status" example:"ok"`
}

// MessageStats is the messages section of /server/stats.
type MessageStats struct {
	ByDay []struct {
		Key   string `json:"key" example:"2026-01-15"`
		Count int    `json:"count" example:"169"`
	} `json:"byDay"`
	ByStatus []struct {
		Key   string `json:"key" example:"Read"`
		Count int    `json:"count" example:"127"`
	} `json:"byStatus"`
	TopSources []struct {
		Key   string `json:"key" example:"5511999999999"`
		Name  string `json:"name" example:"Alice Souza"`
		Phone string `json:"phone" example:"5511999999999"`
		Count int    `json:"count" example:"171"`
	} `json:"topSources"`
	Total int `json:"total" example:"184"`
}

// StorageStats is the storage section of /server/stats.
type StorageStats struct {
	DataDir         string  `json:"dataDir" example:"/app/data"`
	DataFiles       int     `json:"dataFiles" example:"14"`
	DataUsedMB      float64 `json:"dataUsedMB" example:"1.57"`
	DBMessagesMB    float64 `json:"dbMessagesMB" example:"0.11"`
	DBTotalMB       float64 `json:"dbTotalMB" example:"8.05"`
	DiskAvailableMB float64 `json:"diskAvailableMB" example:"884839.43"`
	DiskPath        string  `json:"diskPath" example:"/app/data"`
	DiskTotalMB     float64 `json:"diskTotalMB" example:"1031018.42"`
	DiskUsedMB      float64 `json:"diskUsedMB" example:"93734.19"`
	DiskUsedPct     float64 `json:"diskUsedPct" example:"9.09"`
	MediaEnabled    bool    `json:"mediaEnabled" example:"false"`
}

// SystemStats is the system section of /server/stats.
type SystemStats struct {
	GoVersion          string  `json:"goVersion" example:"go1.26.8"`
	Goroutines         int     `json:"goroutines" example:"43"`
	HeapInuseMB        float64 `json:"heapInuseMB" example:"17.85"`
	HostMemAvailableMB float64 `json:"hostMemAvailableMB" example:"12475.51"`
	HostMemTotalMB     float64 `json:"hostMemTotalMB" example:"15954.18"`
	HostMemUsedPct     float64 `json:"hostMemUsedPct" example:"21.8"`
	LoadAvg1           float64 `json:"loadAvg1" example:"4.47"`
	LoadAvg15          float64 `json:"loadAvg15" example:"1.49"`
	LoadAvg5           float64 `json:"loadAvg5" example:"2.22"`
	MemAllocMB         float64 `json:"memAllocMB" example:"14.92"`
	MemSysMB           float64 `json:"memSysMB" example:"35.45"`
	NumCPU             int     `json:"numCpu" example:"16"`
	NumGC              int     `json:"numGC" example:"60"`
	UptimeSeconds      int64   `json:"uptimeSeconds" example:"6789"`
	Version            string  `json:"version" example:"0.8.1"`
}

// ServerStats is the /server/stats payload.
type ServerStats struct {
	Messages MessageStats `json:"messages"`
	Storage  StorageStats `json:"storage"`
	System   SystemStats  `json:"system"`
}

// -----------------------------------------------------------------------------
// Passkey ceremony (public browser flow)
// -----------------------------------------------------------------------------

// PasskeyCeremony is the state polled by the Passkey Helper extension.
type PasskeyCeremony struct {
	Stage         string      `json:"stage" example:"awaiting-response"`
	SkipHandoffUX bool        `json:"skipHandoffUX" example:"false"`
	PublicKey     interface{} `json:"publicKey,omitempty"`
	Code          string      `json:"code,omitempty" example:"ABCD-EFGH"`
	Error         string      `json:"error,omitempty" example:""`
}

// PasskeyOK is the {"ok": true} acknowledgement returned by the response and
// confirm steps.
type PasskeyOK struct {
	OK bool `json:"ok" example:"true"`
}
