package cloud

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	cloud_model "github.com/felipeestevanatto/wamux/pkg/cloud/model"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
)

func (s *Service) publishMessage(inst *instance_model.Instance, value messageValue, msg inboundMessage, contactID string, contact webhookContact, ts time.Time, messageType, text string, media *mediaObject) {
	if s.events == nil || inst == nil {
		return
	}
	chatJID := phoneJID(msg.From)
	if chatJID == "" {
		chatJID = contactID
	}
	body := map[string]any{"conversation": text}
	if media != nil {
		body = map[string]any{
			messageType + "Message": map[string]any{
				"mimetype": media.MimeType, "caption": media.Caption,
				"fileName": media.Filename, "fileLength": media.FileSize,
				"mediaStatus": "pending",
			},
		}
	}
	payload := map[string]any{
		"event": "Message", "instanceId": inst.Id, "instanceName": inst.Name, "channel": "cloud",
		"data": map[string]any{
			"key":      map[string]any{"remoteJid": chatJID, "fromMe": false, "id": msg.ID},
			"pushName": contact.Profile.Name, "message": body, "messageType": messageType,
			"messageTimestamp": ts.Unix(), "source": sourceFor(msg.From, contactID),
			"contactId": contactID, "bsuid": msg.FromUserID, "parentBsuid": contact.ParentUserID,
			"username": contact.Profile.Username, "waId": msg.From,
		},
	}
	s.emit(inst, "message", payload)
}

func (s *Service) publishReceipt(inst *instance_model.Instance, status inboundStatus, apiStatus, code, message string) {
	if s.events == nil || inst == nil {
		return
	}
	chatJID := ""
	if strings.TrimSpace(status.RecipientID) != "" {
		chatJID = phoneJID(status.RecipientID)
	}
	if chatJID == "" {
		chatJID = status.RecipientUserID
	}
	data := map[string]any{
		"key":    map[string]any{"remoteJid": chatJID, "fromMe": true, "id": status.ID},
		"status": apiStatus, "messageId": status.ID, "recipientId": status.RecipientID,
		"recipientUserId": status.RecipientUserID, "channel": "cloud",
	}
	if code != "" || message != "" {
		data["error"] = map[string]any{"code": code, "message": message}
	}
	s.emit(inst, "receipt", map[string]any{
		"event": "Receipt", "instanceId": inst.Id, "instanceName": inst.Name,
		"channel": "cloud", "data": data,
	})
}

func (s *Service) publishAccount(inst *instance_model.Instance, field string, value json.RawMessage) {
	if s.events == nil || inst == nil {
		return
	}
	event := "CloudAccountEvent"
	if strings.HasPrefix(field, "message_template") || strings.HasPrefix(field, "template_") {
		event = "Template"
	}
	var data any
	_ = json.Unmarshal(value, &data)
	s.emit(inst, strings.ToLower(event), map[string]any{
		"event": event, "instanceId": inst.Id, "instanceName": inst.Name,
		"channel": "cloud", "field": field, "data": data,
	})
}

func (s *Service) emit(inst *instance_model.Instance, queueSuffix string, payload map[string]any) {
	if inst != nil && (s.cfg == nil || s.cfg.WebhookIncludeToken) {
		payload["instanceToken"] = inst.Token
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	go s.events.CallWebhook(inst, strings.ToLower(inst.Id+"."+queueSuffix), data)
	if s.events != nil {
		if event, _ := payload["event"].(string); event != "" {
			go s.events.SendToGlobalQueues(event, data, inst.Id)
		}
	}
}

// RunMediaWorker starts the configured number of media download workers.
func (s *Service) RunMediaWorker(ctx context.Context) {
	workers := 2
	if s.cfg != nil && s.cfg.CloudAPIMediaWorkers > 0 {
		workers = s.cfg.CloudAPIMediaWorkers
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.NewTicker(3 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					s.runMediaBatch(ctx)
				}
			}
		}()
	}
	wg.Wait()
}

func (s *Service) runMediaBatch(ctx context.Context) {
	jobs, err := s.repo.PendingMediaJobs(5)
	if err != nil {
		return
	}
	for i := range jobs {
		if ctx.Err() != nil {
			return
		}
		claimed, err := s.repo.ClaimMediaJob(jobs[i].ID)
		if err != nil || !claimed {
			continue
		}
		jobs[i].Status = "downloading"
		s.processMediaJob(ctx, &jobs[i])
	}
}

func (s *Service) processMediaJob(ctx context.Context, job *cloud_model.MediaJob) {
	_, token, err := s.credentialsToken(job.InstanceID)
	if err != nil {
		s.failMediaJob(job, err)
		return
	}
	inst, _ := s.instanceFor(job.InstanceID)
	data, info, err := s.downloadMedia(ctx, token, job.MediaID)
	if err != nil {
		s.failMediaJob(job, err)
		return
	}
	mime := firstNonEmpty(info.MimeType, job.MimeType)
	filename := safeMediaFilename(firstNonEmpty(job.Filename, info.ID), job.MessageID)
	url, err := s.storeInboundMedia(inst, job.MessageID, data, mime, filename)
	if err != nil {
		s.failMediaJob(job, err)
		return
	}
	if s.messages != nil {
		if row, err := s.messages.GetMessageByIDForInstance(job.InstanceID, job.MessageID); err == nil && row != nil {
			row.MediaUrl = url
			row.MediaMimetype = mime
			row.FileName = filename
			row.FileSize = int64(len(data))
			_ = s.messages.InsertMessage(*row)
		}
	}
	now := time.Now()
	job.Status = "stored"
	job.Attempts++
	job.LastError = ""
	job.CompletedAt = &now
	_ = s.repo.UpdateMediaJob(job)
}

func (s *Service) failMediaJob(job *cloud_model.MediaJob, err error) {
	job.Attempts++
	job.LastError = err.Error()
	if job.Attempts >= 5 {
		job.Status = "failed"
	} else {
		job.Status = "retry"
		backoff := time.Duration(1<<job.Attempts) * time.Minute
		job.NextAttemptAt = time.Now().Add(backoff)
	}
	_ = s.repo.UpdateMediaJob(job)
}

// StoreMedia is exposed for tests and future admin retry endpoints.
func (s *Service) StoreMedia(instance *instance_model.Instance, messageID string, data []byte, mime, filename string) (string, error) {
	return s.storeInboundMedia(instance, messageID, data, mime, filename)
}
