// Package flows implements the WhatsApp Cloud Flows API as a separate program.
// It owns Flow JSON lifecycle, assets and signed endpoint callbacks; it does not
// change the core messaging send path.
package flows

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/felipeestevanatto/wamux/pkg/channel"
	cloud_service "github.com/felipeestevanatto/wamux/pkg/cloud"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Flow is the non-secret Meta Flow representation.
type Flow struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Status           string          `json:"status"`
	Categories       json.RawMessage `json:"categories,omitempty"`
	ValidationErrors json.RawMessage `json:"validation_errors,omitempty"`
	JSONVersion      string          `json:"json_version,omitempty"`
	DataAPIVersion   string          `json:"data_api_version,omitempty"`
	EndpointURI      string          `json:"endpoint_uri,omitempty"`
	Preview          json.RawMessage `json:"preview,omitempty"`
}

// FlowEvent is one signed Flow endpoint/status callback.
type FlowEvent struct {
	ID          string    `json:"id" gorm:"column:id;type:uuid;primaryKey"`
	InstanceID  string    `json:"instanceId" gorm:"column:instance_id;type:uuid;not null;index"`
	FlowID      string    `json:"flowId,omitempty" gorm:"column:flow_id;index"`
	EventType   string    `json:"eventType" gorm:"column:event_type;not null"`
	Payload     string    `json:"payload" gorm:"column:payload;type:jsonb;not null"`
	SignatureOK bool      `json:"signatureOk" gorm:"column:signature_ok;not null"`
	ReceivedAt  time.Time `json:"receivedAt" gorm:"column:received_at;autoCreateTime"`
}

func (FlowEvent) TableName() string { return "cloud_flow_events" }

func (e *FlowEvent) BeforeCreate(_ *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	return nil
}

// Service owns the Flows Graph boundary for one Cloud service.
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

type flowPage struct {
	Data   []Flow `json:"data"`
	Paging struct {
		Cursors struct {
			After string `json:"after"`
		} `json:"cursors"`
	} `json:"paging"`
}

func (s *Service) wabaID(instance *instance_model.Instance) (string, error) {
	bc, err := s.cloud.BusinessContext(instance)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(bc.WABAID) == "" {
		return "", &cloud_service.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "WABA id is not configured"}
	}
	return bc.WABAID, nil
}

func (s *Service) List(ctx context.Context, instance *instance_model.Instance) ([]Flow, error) {
	wabaID, err := s.wabaID(instance)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("fields", "id,name,status,categories,validation_errors,json_version,data_api_version,endpoint_uri,preview")
	q.Set("limit", "100")
	var all []Flow
	after := ""
	for page := 0; page < 50; page++ {
		path := wabaID + "/flows?" + q.Encode()
		if after != "" {
			path += "&after=" + url.QueryEscape(after)
		}
		raw, err := s.cloud.GraphRequest(ctx, instance.Id, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		var out flowPage
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

func (s *Service) Get(ctx context.Context, instance *instance_model.Instance, flowID string) (Flow, error) {
	if strings.TrimSpace(flowID) == "" {
		return Flow{}, &cloud_service.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "flow id is required"}
	}
	q := url.Values{}
	q.Set("fields", "id,name,status,categories,validation_errors,json_version,data_api_version,endpoint_uri,preview")
	raw, err := s.cloud.GraphRequest(ctx, instance.Id, http.MethodGet, flowID+"?"+q.Encode(), nil)
	if err != nil {
		return Flow{}, err
	}
	var out Flow
	return out, json.Unmarshal(raw, &out)
}

func (s *Service) Create(ctx context.Context, instance *instance_model.Instance, payload map[string]any) (Flow, error) {
	wabaID, err := s.wabaID(instance)
	if err != nil {
		return Flow{}, err
	}
	if strings.TrimSpace(asString(payload["name"])) == "" {
		return Flow{}, &cloud_service.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "flow name is required"}
	}
	raw, err := s.cloud.GraphRequest(ctx, instance.Id, http.MethodPost, wabaID+"/flows", payload)
	if err != nil {
		return Flow{}, err
	}
	var out Flow
	return out, json.Unmarshal(raw, &out)
}

func (s *Service) Update(ctx context.Context, instance *instance_model.Instance, flowID string, payload map[string]any) (Flow, error) {
	raw, err := s.cloud.GraphRequest(ctx, instance.Id, http.MethodPost, flowID, payload)
	if err != nil {
		return Flow{}, err
	}
	var out Flow
	return out, json.Unmarshal(raw, &out)
}

func (s *Service) Publish(ctx context.Context, instance *instance_model.Instance, flowID string) (json.RawMessage, error) {
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodPost, flowID+"/publish", map[string]any{})
}

func (s *Service) Deprecate(ctx context.Context, instance *instance_model.Instance, flowID string) (json.RawMessage, error) {
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodPost, flowID+"/deprecate", map[string]any{})
}

func (s *Service) Delete(ctx context.Context, instance *instance_model.Instance, flowID string) error {
	_, err := s.cloud.GraphRequest(ctx, instance.Id, http.MethodDelete, flowID, nil)
	return err
}

func (s *Service) Assets(ctx context.Context, instance *instance_model.Instance, flowID string) (json.RawMessage, error) {
	return s.cloud.GraphRequest(ctx, instance.Id, http.MethodGet, flowID+"/assets", nil)
}

func (s *Service) ListEvents(instanceID, flowID string, limit int) ([]FlowEvent, error) {
	if s.db == nil {
		return nil, errors.New("flows: database is not configured")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := s.db.Where("instance_id = ?", instanceID)
	if flowID != "" {
		q = q.Where("flow_id = ?", flowID)
	}
	var out []FlowEvent
	return out, q.Order("received_at DESC").Limit(limit).Find(&out).Error
}

// HandleCloudChange persists Flow status changes delivered through the main
// WABA webhook. Endpoint callbacks use HandleEndpointCallback instead.
func (s *Service) HandleCloudChange(instanceID, field string, raw []byte) {
	if s.db == nil || (field != "flows" && !strings.HasPrefix(field, "flow_")) {
		return
	}
	flowID := ""
	var v map[string]any
	if json.Unmarshal(raw, &v) == nil {
		flowID = asString(v["flow_id"])
		if flowID == "" {
			flowID = asString(v["id"])
		}
	}
	_ = s.db.Create(&FlowEvent{
		InstanceID: instanceID, FlowID: flowID, EventType: field,
		Payload: string(validJSON(raw)), SignatureOK: true,
	}).Error
}

func (s *Service) RecordEndpointEvent(instanceID, flowID, eventType string, raw []byte) error {
	if s.db == nil {
		return errors.New("flows: database is not configured")
	}
	return s.db.Create(&FlowEvent{
		InstanceID: instanceID, FlowID: flowID, EventType: eventType,
		Payload: string(validJSON(raw)), SignatureOK: true,
	}).Error
}

func (s *Service) Migrate() error {
	if s.db == nil {
		return errors.New("flows: database is not configured")
	}
	return s.db.AutoMigrate(&FlowEvent{})
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// Send opens a Flow through the core Cloud send path so window, rate limit and
// metrics rules remain identical to other interactive messages.
func (s *Service) Send(ctx context.Context, instance *instance_model.Instance, flowID string, msg channel.FlowMessage) (channel.Result, error) {
	if msg.FlowID == "" {
		msg.FlowID = flowID
	}
	return s.cloud.SendFlow(ctx, instance, msg)
}

func validJSON(raw []byte) []byte {
	if len(raw) == 0 || !json.Valid(raw) {
		return []byte("{}")
	}
	return raw
}
