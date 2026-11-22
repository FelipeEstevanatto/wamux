package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	cloud_model "github.com/felipeestevanatto/wamux/pkg/cloud/model"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
)

// TemplateInput is the write shape for template create/update.
type TemplateInput struct {
	Name       string `json:"name"`
	Language   string `json:"language"`
	Category   string `json:"category"`
	Components any    `json:"components"`
}

func (s *Service) ListTemplates(instance *instance_model.Instance) ([]cloud_model.Template, error) {
	return s.repo.ListTemplates(instance.Id)
}

// SyncTemplates fetches every page using cursors. It never follows an absolute
// paging.next URL, so a compromised response cannot redirect the client to
// another host.
func (s *Service) SyncTemplates(instance *instance_model.Instance) ([]cloud_model.Template, error) {
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return nil, err
	}
	after := ""
	for page := 0; page < 50; page++ {
		items, next, err := s.client.ListTemplates(context.Background(), token, c.WABAID, after)
		if err != nil {
			return nil, MapGraphError(err)
		}
		now := time.Now()
		for _, item := range items {
			components, _ := json.Marshal(item.Components)
			quality, _ := json.Marshal(item.QualityScore)
			t := &cloud_model.Template{
				InstanceID: instance.Id, TemplateID: item.ID, Name: item.Name,
				Language: item.Language, Category: item.Category, Status: item.Status,
				Components: string(components), RejectedReason: item.RejectedReason,
				QualityScore: string(quality), LastSyncedAt: &now,
			}
			if err := s.repo.UpsertTemplate(t); err != nil {
				return nil, err
			}
		}
		if next == "" || next == after {
			break
		}
		after = next
	}
	return s.repo.ListTemplates(instance.Id)
}

func (s *Service) CreateTemplate(instance *instance_model.Instance, in TemplateInput) (*cloud_model.Template, error) {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Language) == "" {
		return nil, &APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "template name and language are required"}
	}
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"name": in.Name, "language": in.Language, "category": in.Category,
	}
	if in.Components != nil {
		payload["components"] = in.Components
	}
	item, err := s.client.CreateTemplate(context.Background(), token, c.WABAID, payload)
	if err != nil {
		return nil, MapGraphError(err)
	}
	now := time.Now()
	components, _ := json.Marshal(item.Components)
	t := &cloud_model.Template{
		InstanceID: instance.Id, TemplateID: item.ID, Name: firstNonEmpty(item.Name, in.Name),
		Language: firstNonEmpty(item.Language, in.Language), Category: firstNonEmpty(item.Category, in.Category),
		Status: item.Status, Components: string(components), RejectedReason: item.RejectedReason,
		LastSyncedAt: &now,
	}
	if err := s.repo.UpsertTemplate(t); err != nil {
		return nil, err
	}
	return s.repo.GetTemplate(instance.Id, t.Name, t.Language)
}

func (s *Service) UpdateTemplate(instance *instance_model.Instance, name, language string, in TemplateInput) (*cloud_model.Template, error) {
	_, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return nil, err
	}
	local, err := s.repo.GetTemplate(instance.Id, name, language)
	if err != nil || local.TemplateID == "" {
		return nil, &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "template is not synced or does not exist"}
	}
	payload := map[string]any{}
	if in.Category != "" {
		payload["category"] = in.Category
	}
	if in.Components != nil {
		payload["components"] = in.Components
	}
	item, err := s.client.UpdateTemplate(context.Background(), token, local.TemplateID, payload)
	if err != nil {
		return nil, MapGraphError(err)
	}
	now := time.Now()
	components, _ := json.Marshal(item.Components)
	local.Category = firstNonEmpty(item.Category, local.Category)
	local.Status = firstNonEmpty(item.Status, local.Status)
	local.Components = string(components)
	local.RejectedReason = item.RejectedReason
	local.LastSyncedAt = &now
	if err := s.repo.UpsertTemplate(local); err != nil {
		return nil, err
	}
	return s.repo.GetTemplate(instance.Id, name, language)
}

func (s *Service) DeleteTemplate(instance *instance_model.Instance, name, language string) error {
	if strings.TrimSpace(name) == "" {
		return &APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "template name is required"}
	}
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return err
	}
	if err := s.client.DeleteTemplate(context.Background(), token, c.WABAID, name); err != nil {
		return MapGraphError(err)
	}
	return s.repo.DeleteTemplate(instance.Id, name, language)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

var errNoTemplate = errors.New("template not found")
