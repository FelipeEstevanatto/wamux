// Package calling implements the Cloud Calling control plane as an opt-in
// program. It deliberately does not own WebRTC/SIP media: a real deployment
// must place TURN/SFU and call-media handling behind this control plane.
package calling

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	cloud_service "github.com/felipeestevanatto/wamux/pkg/cloud"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CallEvent struct {
	ID         string    `json:"id" gorm:"column:id;type:uuid;primaryKey"`
	InstanceID string    `json:"instanceId" gorm:"column:instance_id;type:uuid;not null;index"`
	CallID     string    `json:"callId,omitempty" gorm:"column:call_id;index"`
	Event      string    `json:"event" gorm:"column:event;not null"`
	Status     string    `json:"status,omitempty" gorm:"column:status"`
	Direction  string    `json:"direction,omitempty" gorm:"column:direction"`
	Payload    string    `json:"payload" gorm:"column:payload;type:jsonb;not null"`
	ReceivedAt time.Time `json:"receivedAt" gorm:"column:received_at;autoCreateTime"`
}

func (CallEvent) TableName() string { return "cloud_call_events" }

func (e *CallEvent) BeforeCreate(_ *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	return nil
}

type Service struct {
	cloud *cloud_service.Service
	db    *gorm.DB
}

func New(cloudSvc *cloud_service.Service) *Service {
	s := &Service{cloud: cloudSvc}
	if cloudSvc != nil {
		s.db = cloudSvc.Repository().DB()
	}
	return s
}

func (s *Service) Migrate() error {
	if s.db == nil {
		return errors.New("calling: database is not configured")
	}
	return s.db.AutoMigrate(&CallEvent{})
}

func (s *Service) phoneID(instance *instance_model.Instance) (string, error) {
	bc, err := s.cloud.BusinessContext(instance)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(bc.PhoneID) == "" {
		return "", &cloud_service.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "phone number id is not configured"}
	}
	return bc.PhoneID, nil
}

// Start creates a call control request. Media is negotiated by the separate
// WebRTC/SIP plane; this endpoint only establishes the Meta call session.
func (s *Service) Start(ctx context.Context, instance *instance_model.Instance, payload map[string]any) (json.RawMessage, error) {
	phoneID, err := s.phoneID(instance)
	if err != nil {
		return nil, err
	}
	to, _ := payload["to"].(string)
	if strings.TrimSpace(to) == "" {
		return nil, &cloud_service.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "call recipient is required"}
	}
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodPost, phoneID+"/calls", payload)
}

func (s *Service) Get(ctx context.Context, instance *instance_model.Instance, callID string) (json.RawMessage, error) {
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodGet, callID, nil)
}

// Action accepts Meta's call-control action names. Media actions are rejected
// because they belong to the WebRTC/SIP plane, not this control API.
func (s *Service) Action(ctx context.Context, instance *instance_model.Instance, callID, action string, extra map[string]any) (json.RawMessage, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "pre_accept", "accept", "reject", "terminate":
	default:
		if action == "media" || action == "webrtc" || action == "sdp" {
			return nil, &cloud_service.APIError{Status: http.StatusNotImplemented, Code: "not_implemented_on_channel", Message: "call media is owned by the separate WebRTC/SIP service"}
		}
		return nil, &cloud_service.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "unsupported call action"}
	}
	payload := map[string]any{"action": action}
	for k, v := range extra {
		payload[k] = v
	}
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodPost, callID+"/actions", payload)
}

func (s *Service) Permissions(ctx context.Context, instance *instance_model.Instance) (json.RawMessage, error) {
	phoneID, err := s.phoneID(instance)
	if err != nil {
		return nil, err
	}
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodGet, phoneID+"/call_permissions", nil)
}

func (s *Service) UpdatePermissions(ctx context.Context, instance *instance_model.Instance, payload map[string]any) (json.RawMessage, error) {
	phoneID, err := s.phoneID(instance)
	if err != nil {
		return nil, err
	}
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodPost, phoneID+"/call_permissions", payload)
}

func (s *Service) Settings(ctx context.Context, instance *instance_model.Instance) (json.RawMessage, error) {
	phoneID, err := s.phoneID(instance)
	if err != nil {
		return nil, err
	}
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodGet, phoneID+"/call_settings", nil)
}

func (s *Service) UpdateSettings(ctx context.Context, instance *instance_model.Instance, payload map[string]any) (json.RawMessage, error) {
	phoneID, err := s.phoneID(instance)
	if err != nil {
		return nil, err
	}
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodPost, phoneID+"/call_settings", payload)
}

func (s *Service) ListEvents(instanceID, callID string, limit int) ([]CallEvent, error) {
	if s.db == nil {
		return nil, errors.New("calling: database is not configured")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := s.db.Where("instance_id = ?", instanceID)
	if callID != "" {
		q = q.Where("call_id = ?", callID)
	}
	var out []CallEvent
	return out, q.Order("received_at DESC").Limit(limit).Find(&out).Error
}

// HandleCloudChange stores the `calls` webhook field. The main Cloud webhook
// has already verified the app signature; this service only persists/emits the
// control event.
func (s *Service) HandleCloudChange(instanceID, field string, raw []byte) {
	if s.db == nil || (field != "calls" && !strings.HasPrefix(field, "call_")) {
		return
	}
	var v struct {
		CallID    string `json:"call_id"`
		ID        string `json:"id"`
		Event     string `json:"event"`
		Status    string `json:"status"`
		Direction string `json:"direction"`
	}
	_ = json.Unmarshal(raw, &v)
	callID := v.CallID
	if callID == "" {
		callID = v.ID
	}
	_ = s.db.Create(&CallEvent{
		InstanceID: instanceID, CallID: callID, Event: first(v.Event, field),
		Status: v.Status, Direction: v.Direction, Payload: string(raw),
	}).Error
}

func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "call_event"
}
