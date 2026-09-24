package model

import "time"

// PollVote representa um voto em uma enquete do WhatsApp
type PollVote struct {
	ID              string    `json:"id" example:"11111111-2222-3333-4444-555555555555"`
	CompanyID       string    `json:"companyId" example:"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"`
	InstanceID      string    `json:"instanceId" example:"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"`
	PollMessageID   string    `json:"pollMessageId" example:"3EB0A1B2C3D4E5F6A7B8C9"`
	PollChatJid     string    `json:"pollChatJid" example:"120363000000000000@g.us"`
	VoteMessageID   string    `json:"voteMessageId" example:"3EB0B2C3D4E5F6A7B8C9D0"`
	VoterJid        string    `json:"voterJid" example:"5511999999999@s.whatsapp.net"`
	VoterPhone      string    `json:"voterPhone,omitempty" example:"5511999999999"`
	VoterName       string    `json:"voterName,omitempty" example:"Alice Souza"`
	SelectedOptions []string  `json:"selectedOptions" example:"a1b2c3d4e5f6"` // SHA-256 hashes
	VotedAt         time.Time `json:"votedAt" example:"2026-01-15T10:30:00Z"`
	ReceivedAt      time.Time `json:"receivedAt" example:"2026-01-15T10:30:01Z"`
}

// PollResults representa os resultados agregados de uma enquete
type PollResults struct {
	PollMessageID string         `json:"pollMessageId" example:"3EB0A1B2C3D4E5F6A7B8C9"`
	PollChatJid   string         `json:"pollChatJid" example:"120363000000000000@g.us"`
	TotalVotes    int            `json:"totalVotes" example:"3"`
	Votes         []PollVote     `json:"votes"`
	OptionCounts  map[string]int `json:"optionCounts" example:"a1b2c3d4e5f6:2"` // hash -> count
	Voters        []VoterInfo    `json:"voters"`
}

// VoterInfo representa informações de um votante
type VoterInfo struct {
	Jid             string    `json:"jid" example:"5511999999999@s.whatsapp.net"`
	Phone           string    `json:"phone,omitempty" example:"5511999999999"`
	Name            string    `json:"name,omitempty" example:"Alice Souza"`
	SelectedOptions []string  `json:"selectedOptions" example:"a1b2c3d4e5f6"`
	VotedAt         time.Time `json:"votedAt" example:"2026-01-15T10:30:00Z"`
}
