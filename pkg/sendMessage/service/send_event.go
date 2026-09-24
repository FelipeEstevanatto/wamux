package send_service

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// ============================================================================
// WhatsApp event / calendar message (waE2E.EventMessage).
// Ported from upstream PR #90.
// ============================================================================

// EventTime accepts epoch seconds (number or string) OR an ISO 8601 (RFC3339)
// timestamp with timezone, normalising everything to epoch seconds.
type EventTime int64

func (t *EventTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		*t = EventTime(n)
		return nil
	}
	if tm, err := time.Parse(time.RFC3339, s); err == nil {
		*t = EventTime(tm.Unix())
		return nil
	}
	return fmt.Errorf("invalid time %q: use ISO 8601 (RFC3339) or epoch seconds", s)
}

// Unix returns the time in epoch seconds.
func (t EventTime) Unix() int64 { return int64(t) }

// EventLocationStruct is the optional location attached to the event.
type EventLocationStruct struct {
	Name      string  `json:"name,omitempty" example:"Headquarters"`
	Latitude  float64 `json:"latitude,omitempty" example:"-16.6869"`
	Longitude float64 `json:"longitude,omitempty" example:"-49.2648"`
	Address   string  `json:"address,omitempty" example:"Main St, 1000"`
}

// EventStruct is the body of POST /send/event.
//
// Sends a WhatsApp event/calendar message. `startTime`/`endTime` accept an ISO
// 8601 (RFC3339) string with timezone or epoch seconds. Only `number`, `name`
// and `startTime` are required. Usually sent to a group JID (...@g.us).
type EventStruct struct {
	Number string `json:"number" example:"120363000000000000@g.us"`
	Name   string `json:"name" example:"Sales meeting"`

	Description string `json:"description,omitempty" example:"Reuniao trimestral de vendas"`
	// Optional text sent BEFORE the event card (the event itself has no caption).
	// Respects mentionAll/mentionedJid/delay.
	Text string `json:"text,omitempty" example:"Segue o convite da reuniao"`

	StartTime EventTime `json:"startTime" swaggertype:"string" example:"2026-06-25T20:00:00-03:00"`
	EndTime   EventTime `json:"endTime,omitempty" swaggertype:"string" example:"2026-06-25T21:00:00-03:00"`

	Location *EventLocationStruct `json:"location,omitempty"`
	// Call link (only call.whatsapp.com; external links go in description).
	JoinLink string `json:"joinLink,omitempty" example:"https://call.whatsapp.com/video/AbCdEf123456"`

	ExtraGuestsAllowed bool  `json:"extraGuestsAllowed,omitempty" example:"true"`
	IsScheduleCall     bool  `json:"isScheduleCall,omitempty" example:"false"`
	HasReminder        bool  `json:"hasReminder,omitempty" example:"true"`
	ReminderOffsetSec  int64 `json:"reminderOffsetSec,omitempty" example:"900"`
	IsCanceled         bool  `json:"isCanceled,omitempty" example:"false"`

	Id           string       `json:"id,omitempty" example:"3EB0A1B2C3D4E5F6A7B8C9"`
	Delay        int32        `json:"delay,omitempty" example:"1200"`
	MentionedJID []string     `json:"mentionedJid,omitempty" example:"5511999999999@s.whatsapp.net"`
	MentionAll   bool         `json:"mentionAll,omitempty" example:"false"`
	FormatJid    *bool        `json:"formatJid,omitempty" example:"true"`
	Quoted       QuotedStruct `json:"quoted,omitempty"`
}

func (s *sendService) SendEvent(data *EventStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	_, err := s.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	// Optional caption text: EventMessage has no caption, so when `text` is set
	// it goes as a separate text message right before the card.
	if data.Text != "" {
		if _, err := s.SendText(&TextStruct{
			Number:       data.Number,
			Text:         data.Text,
			Delay:        data.Delay,
			MentionAll:   data.MentionAll,
			MentionedJID: data.MentionedJID,
			FormatJid:    data.FormatJid,
		}, instance); err != nil {
			return nil, fmt.Errorf("failed to send event text: %w", err)
		}
	}

	event := &waE2E.EventMessage{
		Name:      proto.String(data.Name),
		StartTime: proto.Int64(data.StartTime.Unix()),
	}
	if data.Description != "" {
		event.Description = proto.String(data.Description)
	}
	if data.EndTime.Unix() > 0 {
		event.EndTime = proto.Int64(data.EndTime.Unix())
	}
	if data.JoinLink != "" {
		event.JoinLink = proto.String(data.JoinLink)
	}
	// The official client always sends these booleans explicitly on creation; the
	// server expects them to be present (a nil *bool is dropped from the wire and
	// the event is silently ignored), so set them unconditionally.
	event.IsCanceled = proto.Bool(data.IsCanceled)
	event.IsScheduleCall = proto.Bool(data.IsScheduleCall)
	event.ExtraGuestsAllowed = proto.Bool(data.ExtraGuestsAllowed)
	if data.HasReminder {
		event.HasReminder = proto.Bool(true)
		if data.ReminderOffsetSec > 0 {
			event.ReminderOffsetSec = proto.Int64(data.ReminderOffsetSec)
		}
	}
	if data.Location != nil {
		event.Location = &waE2E.LocationMessage{
			DegreesLatitude:  proto.Float64(data.Location.Latitude),
			DegreesLongitude: proto.Float64(data.Location.Longitude),
			Name:             proto.String(data.Location.Name),
			Address:          proto.String(data.Location.Address),
		}
	}

	// MessageSecret (32 bytes) so event replies (going/not going) can be decrypted
	// -- same pattern as BuildPollCreation.
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("failed to generate event message secret: %w", err)
	}

	msg := &waE2E.Message{
		EventMessage:       event,
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: secret},
	}

	return s.SendMessage(instance, msg, "EventMessage", &SendDataStruct{
		Id:           data.Id,
		Number:       data.Number,
		Quoted:       data.Quoted,
		Delay:        data.Delay,
		MentionAll:   data.MentionAll,
		MentionedJID: data.MentionedJID,
		FormatJid:    data.FormatJid,
	})
}
