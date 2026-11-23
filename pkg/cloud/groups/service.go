// Package groups implements the WhatsApp Cloud Groups API as an opt-in program.
// It is OBA-only by Meta policy and is intentionally isolated from the direct
// message send path.
package groups

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	cloud_service "github.com/felipeestevanatto/wamux/pkg/cloud"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Group struct {
	ID               string          `json:"id"`
	Subject          string          `json:"subject,omitempty"`
	Description      string          `json:"description,omitempty"`
	Participants     json.RawMessage `json:"participants,omitempty"`
	InviteLink       string          `json:"invite_link,omitempty"`
	JoinApprovalMode string          `json:"join_approval_mode,omitempty"`
	CreatedAt        string          `json:"creation_timestamp,omitempty"`
}

type Participant struct {
	ID     string `json:"id"`
	WaID   string `json:"wa_id,omitempty"`
	UserID string `json:"user_id,omitempty"`
	Role   string `json:"role,omitempty"`
}

type JoinRequest struct {
	ID     string `json:"id"`
	WaID   string `json:"wa_id,omitempty"`
	UserID string `json:"user_id,omitempty"`
	Status string `json:"status,omitempty"`
}

// GroupEvent is a signed/verified group webhook retained for the manager.
type GroupEvent struct {
	ID         string    `json:"id" gorm:"column:id;type:uuid;primaryKey"`
	InstanceID string    `json:"instanceId" gorm:"column:instance_id;type:uuid;not null;index"`
	GroupID    string    `json:"groupId,omitempty" gorm:"column:group_id;index"`
	EventType  string    `json:"eventType" gorm:"column:event_type;not null"`
	Payload    string    `json:"payload" gorm:"column:payload;type:jsonb;not null"`
	ReceivedAt time.Time `json:"receivedAt" gorm:"column:received_at;autoCreateTime"`
}

func (GroupEvent) TableName() string { return "cloud_group_events" }

func (e *GroupEvent) BeforeCreate(_ *gorm.DB) error {
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
		return errors.New("groups: database is not configured")
	}
	return s.db.AutoMigrate(&GroupEvent{})
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

func (s *Service) List(ctx context.Context, instance *instance_model.Instance) ([]Group, error) {
	phoneID, err := s.phoneID(instance)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("fields", "id,subject,description,participants,invite_link,join_approval_mode,creation_timestamp")
	q.Set("limit", "100")
	var all []Group
	after := ""
	for page := 0; page < 50; page++ {
		path := phoneID + "/groups?" + q.Encode()
		if after != "" {
			path += "&after=" + url.QueryEscape(after)
		}
		raw, err := s.cloud.GraphRequest(ctx, instance.Id, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		var out struct {
			Data   []Group `json:"data"`
			Paging struct {
				Cursors struct {
					After string `json:"after"`
				} `json:"cursors"`
			} `json:"paging"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Data...)
		if out.Paging.Cursors.After == "" || out.Paging.Cursors.After == after {
			break
		}
		after = out.Paging.Cursors.After
	}
	return all, nil
}

func (s *Service) Get(ctx context.Context, instance *instance_model.Instance, groupID string) (Group, error) {
	q := url.Values{}
	q.Set("fields", "id,subject,description,participants,invite_link,join_approval_mode,creation_timestamp")
	raw, err := s.cloud.GraphRequest(ctx, instance.Id, http.MethodGet, groupID+"?"+q.Encode(), nil)
	if err != nil {
		return Group{}, err
	}
	var out Group
	return out, json.Unmarshal(raw, &out)
}

func (s *Service) Create(ctx context.Context, instance *instance_model.Instance, subject, description string, participants []string) (Group, error) {
	phoneID, err := s.phoneID(instance)
	if err != nil {
		return Group{}, err
	}
	if strings.TrimSpace(subject) == "" {
		return Group{}, &cloud_service.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "group subject is required"}
	}
	if len(participants) > 7 {
		return Group{}, &cloud_service.APIError{Status: http.StatusUnprocessableEntity, Code: "invalid_request", Message: "Cloud groups support at most 8 participants including the business number"}
	}
	payload := map[string]any{"subject": subject, "participants": participants}
	if description != "" {
		payload["description"] = description
	}
	raw, err := s.cloud.GraphRequest(ctx, instance.Id, http.MethodPost, phoneID+"/groups", payload)
	if err != nil {
		return Group{}, err
	}
	var out Group
	return out, json.Unmarshal(raw, &out)
}

func (s *Service) Delete(ctx context.Context, instance *instance_model.Instance, groupID string) error {
	_, err := s.cloud.GraphRequest(ctx, instance.Id, http.MethodDelete, groupID, nil)
	return err
}

func (s *Service) Participants(ctx context.Context, instance *instance_model.Instance, groupID string) (json.RawMessage, error) {
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodGet, groupID+"/participants", nil)
}

func (s *Service) AddParticipants(ctx context.Context, instance *instance_model.Instance, groupID string, participants []string) (json.RawMessage, error) {
	if len(participants) == 0 {
		return nil, &cloud_service.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "at least one participant is required"}
	}
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodPost, groupID+"/participants", map[string]any{"participants": participants})
}

func (s *Service) RemoveParticipant(ctx context.Context, instance *instance_model.Instance, groupID, participant string) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("participants", participant)
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodDelete, groupID+"/participants?"+q.Encode(), nil)
}

func (s *Service) InviteLink(ctx context.Context, instance *instance_model.Instance, groupID string) (json.RawMessage, error) {
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodPost, groupID+"/invite_link", map[string]any{})
}

func (s *Service) JoinRequests(ctx context.Context, instance *instance_model.Instance, groupID string) (json.RawMessage, error) {
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodGet, groupID+"/join_requests", nil)
}

func (s *Service) DecideJoinRequest(ctx context.Context, instance *instance_model.Instance, groupID, requestID, action string) (json.RawMessage, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "approve" && action != "reject" {
		return nil, &cloud_service.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "action must be approve or reject"}
	}
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodPost, groupID+"/join_requests/"+requestID, map[string]string{"action": action})
}

func (s *Service) UpdateSettings(ctx context.Context, instance *instance_model.Instance, groupID string, payload map[string]any) (json.RawMessage, error) {
	if len(payload) == 0 {
		return nil, &cloud_service.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "at least one setting is required"}
	}
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodPost, groupID+"/settings", payload)
}

// Send sends a text/media/template payload to a group. Interactive, commerce
// and calling messages are rejected by Meta's group matrix, so validate early.
func (s *Service) Send(ctx context.Context, instance *instance_model.Instance, groupID string, payload map[string]any) (json.RawMessage, error) {
	kind, _ := payload["type"].(string)
	switch kind {
	case "text", "image", "video", "audio", "document", "template":
	default:
		return nil, &cloud_service.APIError{Status: http.StatusUnprocessableEntity, Code: "unsupported_on_channel", Message: "Cloud groups support text, media and template messages only"}
	}
	if strings.TrimSpace(groupID) == "" {
		return nil, &cloud_service.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "group id is required"}
	}
	payload = cloneMap(payload)
	payload["messaging_product"] = "whatsapp"
	payload["recipient_type"] = "group"
	payload["to"] = groupID
	resp, err := s.cloud.SendGroupPayload(ctx, instance, payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(resp)
}

func (s *Service) ListEvents(instanceID, groupID string, limit int) ([]GroupEvent, error) {
	if s.db == nil {
		return nil, errors.New("groups: database is not configured")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := s.db.Where("instance_id = ?", instanceID)
	if groupID != "" {
		q = q.Where("group_id = ?", groupID)
	}
	var out []GroupEvent
	return out, q.Order("received_at DESC").Limit(limit).Find(&out).Error
}

func (s *Service) HandleCloudChange(instanceID, field string, raw []byte) {
	if s.db == nil {
		return
	}
	if !strings.HasPrefix(field, "group") && field != "groups" {
		return
	}
	groupID := ""
	var v map[string]any
	if json.Unmarshal(raw, &v) == nil {
		groupID = stringValue(v["group_id"])
		if groupID == "" {
			groupID = stringValue(v["id"])
		}
	}
	_ = s.db.Create(&GroupEvent{
		InstanceID: instanceID, GroupID: groupID, EventType: field, Payload: string(raw),
	}).Error
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
