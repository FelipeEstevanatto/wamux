package cloud

import (
	"errors"
	"time"

	cloud_model "github.com/felipeestevanatto/wamux/pkg/cloud/model"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrNotFound is returned by repository lookups that have no row.
var ErrNotFound = errors.New("cloud: not found")

// Repository is the persistence boundary for Cloud API state. It deliberately
// does not expose GORM types to the service layer's callers.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) DB() *gorm.DB { return r.db }

func (r *Repository) SaveApp(app *cloud_model.App) error {
	if app.GraphVersion == "" {
		app.GraphVersion = "v27.0"
	}
	return r.db.Save(app).Error
}

func (r *Repository) GetApp(appID string) (*cloud_model.App, error) {
	var app cloud_model.App
	if err := r.db.Where("app_id = ?", appID).First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &app, nil
}

func (r *Repository) FindAppByVerifyHash(hash string) (*cloud_model.App, error) {
	var app cloud_model.App
	if err := r.db.Where("verify_token_hash = ?", hash).First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &app, nil
}

func (r *Repository) ListApps() ([]cloud_model.App, error) {
	var apps []cloud_model.App
	err := r.db.Order("app_id").Find(&apps).Error
	return apps, err
}

func (r *Repository) DeleteApp(appID string) error {
	return r.db.Delete(&cloud_model.App{}, "app_id = ?", appID).Error
}

func (r *Repository) SaveCredentials(c *cloud_model.Credentials) error {
	// The model intentionally has no GORM default tag for enforce_window:
	// otherwise an explicit false is replaced by the column default on insert.
	return r.db.Save(c).Error
}

func (r *Repository) GetCredentials(instanceID string) (*cloud_model.Credentials, error) {
	var c cloud_model.Credentials
	if err := r.db.Where("instance_id = ?", instanceID).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (r *Repository) GetCredentialsByPhone(phoneNumberID string) (*cloud_model.Credentials, error) {
	var c cloud_model.Credentials
	if err := r.db.Where("phone_number_id = ?", phoneNumberID).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (r *Repository) GetCredentialsByWABA(wabaID string) (*cloud_model.Credentials, error) {
	var c cloud_model.Credentials
	if err := r.db.Where("waba_id = ?", wabaID).Order("created_at").First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (r *Repository) ListCredentials() ([]cloud_model.Credentials, error) {
	var out []cloud_model.Credentials
	err := r.db.Order("created_at").Find(&out).Error
	return out, err
}

func (r *Repository) DeleteCredentials(instanceID string) error {
	return r.db.Delete(&cloud_model.Credentials{}, "instance_id = ?", instanceID).Error
}

func (r *Repository) UpsertContact(c *cloud_model.Contact) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "instance_id"}, {Name: "contact_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"bsuid", "parent_bsuid", "wa_id", "username", "display_name", "last_seen_at",
		}),
	}).Create(c).Error
}

func (r *Repository) GetContact(instanceID, contactID string) (*cloud_model.Contact, error) {
	var c cloud_model.Contact
	if err := r.db.Where("instance_id = ? AND contact_id = ?", instanceID, contactID).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (r *Repository) FindContactByBSUID(instanceID, bsuid string) (*cloud_model.Contact, error) {
	if bsuid == "" {
		return nil, ErrNotFound
	}
	var c cloud_model.Contact
	if err := r.db.Where("instance_id = ? AND bsuid = ?", instanceID, bsuid).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (r *Repository) FindContactByWAID(instanceID, waID string) (*cloud_model.Contact, error) {
	if waID == "" {
		return nil, ErrNotFound
	}
	var c cloud_model.Contact
	if err := r.db.Where("instance_id = ? AND wa_id = ?", instanceID, waID).Order("last_seen_at DESC").First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (r *Repository) UpsertTemplate(t *cloud_model.Template) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "instance_id"}, {Name: "name"}, {Name: "language"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"template_id", "category", "status", "components", "rejected_reason",
			"quality_score", "last_synced_at", "updated_at",
		}),
	}).Create(t).Error
}

func (r *Repository) GetTemplate(instanceID, name, language string) (*cloud_model.Template, error) {
	var t cloud_model.Template
	if err := r.db.Where("instance_id = ? AND name = ? AND language = ?", instanceID, name, language).First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *Repository) GetTemplateByID(instanceID, templateID string) (*cloud_model.Template, error) {
	var t cloud_model.Template
	if err := r.db.Where("instance_id = ? AND template_id = ?", instanceID, templateID).First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *Repository) ListTemplates(instanceID string) ([]cloud_model.Template, error) {
	var out []cloud_model.Template
	err := r.db.Where("instance_id = ?", instanceID).Order("name, language").Find(&out).Error
	return out, err
}

func (r *Repository) DeleteTemplate(instanceID, name, language string) error {
	return r.db.Where("instance_id = ? AND name = ? AND language = ?", instanceID, name, language).
		Delete(&cloud_model.Template{}).Error
}

func (r *Repository) UpsertWindow(w *cloud_model.ChatWindow) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "instance_id"}, {Name: "contact_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"chat_jid", "last_inbound_at", "expires_at", "last_message_id",
		}),
	}).Create(w).Error
}

func (r *Repository) GetWindow(instanceID, contactID string) (*cloud_model.ChatWindow, error) {
	var w cloud_model.ChatWindow
	if err := r.db.Where("instance_id = ? AND contact_id = ?", instanceID, contactID).First(&w).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &w, nil
}

func (r *Repository) ClaimWebhookEvent(e *cloud_model.WebhookEvent) (bool, error) {
	res := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(e)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func (r *Repository) MarkWebhookProcessed(eventKey string) error {
	now := time.Now()
	return r.db.Model(&cloud_model.WebhookEvent{}).
		Where("event_key = ?", eventKey).
		Updates(map[string]any{"processed_at": &now, "last_error": "", "attempts": gorm.Expr("attempts + 1")}).Error
}

func (r *Repository) ReleaseWebhookEvent(eventKey, errText string) error {
	return r.db.Where("event_key = ?", eventKey).Delete(&cloud_model.WebhookEvent{}).Error
}

func (r *Repository) CreateMediaJob(j *cloud_model.MediaJob) error {
	return r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(j).Error
}

func (r *Repository) PendingMediaJobs(limit int) ([]cloud_model.MediaJob, error) {
	if limit <= 0 {
		limit = 10
	}
	var out []cloud_model.MediaJob
	now := time.Now()
	err := r.db.Where("status IN ? AND next_attempt_at <= ?", []string{"pending", "retry"}, now).
		Order("next_attempt_at").Limit(limit).Find(&out).Error
	return out, err
}

func (r *Repository) ClaimMediaJob(id string) (bool, error) {
	res := r.db.Model(&cloud_model.MediaJob{}).
		Where("id = ? AND status IN ?", id, []string{"pending", "retry"}).
		Update("status", "downloading")
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func (r *Repository) UpdateMediaJob(j *cloud_model.MediaJob) error {
	return r.db.Save(j).Error
}

func (r *Repository) CountPendingMediaJobs() (int, error) {
	var n int64
	err := r.db.Model(&cloud_model.MediaJob{}).
		Where("status IN ? AND next_attempt_at <= ?", []string{"pending", "retry"}, time.Now()).Count(&n).Error
	return int(n), err
}

func (r *Repository) AddStatusEvent(s *cloud_model.StatusEvent) (bool, error) {
	res := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(s)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func (r *Repository) AddAccountEvent(e *cloud_model.AccountEvent) error {
	return r.db.Create(e).Error
}

func (r *Repository) UpsertUserPreference(p *cloud_model.UserPreference) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "instance_id"}, {Name: "contact_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"marketing_opt_out", "source_event_at", "updated_at"}),
	}).Create(p).Error
}

// DeleteInstanceData removes every Cloud row owned by one instance. It is used
// by the instance deletion path and is intentionally explicit so SQLite builds
// without foreign-key enforcement do not leave orphans.
func (r *Repository) DeleteInstanceData(instanceID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, table := range []any{
			&cloud_model.StatusEvent{},
			&cloud_model.AccountEvent{},
			&cloud_model.UserPreference{},
			&cloud_model.MediaJob{},
			&cloud_model.WebhookEvent{},
			&cloud_model.ChatWindow{},
			&cloud_model.Contact{},
			&cloud_model.Template{},
			&cloud_model.Credentials{},
		} {
			if err := tx.Where("instance_id = ?", instanceID).Delete(table).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Cleanup removes old Cloud event/status/account rows. It is called by the
// in-process retention worker; no external scheduler is required.
func (r *Repository) Cleanup(now time.Time, webhookDays, statusDays, accountDays, mediaDays int) (int64, error) {
	var total int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if webhookDays > 0 {
			res := tx.Where("received_at < ?", now.AddDate(0, 0, -webhookDays)).Delete(&cloud_model.WebhookEvent{})
			if res.Error != nil {
				return res.Error
			}
			total += res.RowsAffected
		}
		if statusDays > 0 {
			res := tx.Where("occurred_at < ?", now.AddDate(0, 0, -statusDays)).Delete(&cloud_model.StatusEvent{})
			if res.Error != nil {
				return res.Error
			}
			total += res.RowsAffected
		}
		if accountDays > 0 {
			res := tx.Where("received_at < ?", now.AddDate(0, 0, -accountDays)).Delete(&cloud_model.AccountEvent{})
			if res.Error != nil {
				return res.Error
			}
			total += res.RowsAffected
		}
		if mediaDays > 0 {
			res := tx.Where("status IN ? AND updated_at < ?", []string{"failed", "stored"}, now.AddDate(0, 0, -mediaDays)).
				Delete(&cloud_model.MediaJob{})
			if res.Error != nil {
				return res.Error
			}
			total += res.RowsAffected
		}
		return nil
	})
	return total, err
}

// IdentityMerge is the transaction input for a BSUID/phone identity change.
type IdentityMerge struct {
	InstanceID    string
	Canonical     *cloud_model.Contact
	OldContactID  string
	NewContactID  string
	OldBSUID      string
	NewBSUID      string
	ParentBSUID   string
	WAID          string
	SourceEventID string
	EventTime     time.Time
}

// MergeIdentity updates the current contact and moves all Cloud-owned rows to
// the canonical contact in one transaction. Historical message IDs are never
// changed.
func (r *Repository) MergeIdentity(m IdentityMerge) error {
	if m.Canonical == nil {
		return errors.New("cloud: identity merge requires a canonical contact")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "instance_id"}, {Name: "contact_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"bsuid", "parent_bsuid", "wa_id", "username", "display_name", "last_seen_at",
			}),
		}).Create(m.Canonical).Error; err != nil {
			return err
		}
		ids := uniqueStrings(m.OldContactID, m.NewContactID, m.Canonical.ContactID)
		if len(ids) > 0 {
			if err := tx.Model(&message_model.Message{}).
				Where("instance_id = ? AND contact_id IN ?", m.InstanceID, ids).
				Update("contact_id", m.Canonical.ContactID).Error; err != nil {
				return err
			}
			// Windows are merged rather than overwritten: the latest expiry
			// wins, so a BSUID change cannot close an open window.
			var windows []cloud_model.ChatWindow
			if err := tx.Where("instance_id = ? AND contact_id IN ?", m.InstanceID, ids).Find(&windows).Error; err != nil {
				return err
			}
			if len(windows) > 0 {
				best := windows[0]
				for _, w := range windows[1:] {
					if w.ExpiresAt.After(best.ExpiresAt) {
						best = w
					}
				}
				if err := tx.Where("instance_id = ? AND contact_id IN ?", m.InstanceID, ids).
					Delete(&cloud_model.ChatWindow{}).Error; err != nil {
					return err
				}
				best.InstanceID = m.InstanceID
				best.ContactID = m.Canonical.ContactID
				if best.ChatJID == "" {
					best.ChatJID = m.Canonical.WAID
				}
				if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&best).Error; err != nil {
					return err
				}
			}
			// Preserve the most restrictive marketing preference.
			var prefs []cloud_model.UserPreference
			if err := tx.Where("instance_id = ? AND contact_id IN ?", m.InstanceID, ids).Find(&prefs).Error; err != nil {
				return err
			}
			if len(prefs) > 0 {
				merged := cloud_model.UserPreference{
					InstanceID: m.InstanceID, ContactID: m.Canonical.ContactID,
					MarketingOptOut: false,
				}
				for _, p := range prefs {
					if p.MarketingOptOut {
						merged.MarketingOptOut = true
					}
					if merged.SourceEventAt == nil || (p.SourceEventAt != nil && p.SourceEventAt.After(*merged.SourceEventAt)) {
						merged.SourceEventAt = p.SourceEventAt
					}
				}
				if err := tx.Where("instance_id = ? AND contact_id IN ?", m.InstanceID, ids).
					Delete(&cloud_model.UserPreference{}).Error; err != nil {
					return err
				}
				if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&merged).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&cloud_model.StatusEvent{}).
				Where("instance_id = ? AND recipient_user_id = ?", m.InstanceID, m.OldBSUID).
				Update("recipient_user_id", m.NewBSUID).Error; err != nil {
				return err
			}
			deleteIDs := make([]string, 0, 2)
			for _, id := range uniqueStrings(m.OldContactID, m.NewContactID) {
				if id != m.Canonical.ContactID {
					deleteIDs = append(deleteIDs, id)
				}
			}
			if len(deleteIDs) > 0 {
				if err := tx.Where("instance_id = ? AND contact_id IN ?", m.InstanceID, deleteIDs).
					Delete(&cloud_model.Contact{}).Error; err != nil {
					return err
				}
			}
		}
		// Close the old identity interval and record the new/phone identities.
		closeAt := m.EventTime
		if err := tx.Model(&cloud_model.ContactIdentity{}).
			Where("instance_id = ? AND identity_value = ? AND valid_to IS NULL", m.InstanceID, m.OldBSUID).
			Update("valid_to", &closeAt).Error; err != nil {
			return err
		}
		for _, id := range []struct{ typ, value string }{
			{"bsuid", m.NewBSUID}, {"parent_bsuid", m.ParentBSUID}, {"wa_id", digits(m.WAID)},
		} {
			if id.value == "" {
				continue
			}
			var count int64
			if err := tx.Model(&cloud_model.ContactIdentity{}).
				Where("instance_id = ? AND identity_type = ? AND identity_value = ? AND valid_to IS NULL",
					m.InstanceID, id.typ, id.value).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				if err := tx.Create(&cloud_model.ContactIdentity{
					InstanceID: m.InstanceID, ContactID: m.Canonical.ContactID,
					IdentityType: id.typ, IdentityValue: id.value,
					ValidFrom: m.EventTime, SourceEventID: m.SourceEventID,
				}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func uniqueStrings(values ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func (r *Repository) AddIdentityEvent(e *cloud_model.IdentityEvent) error {
	return r.db.Create(e).Error
}

func (r *Repository) MarkIdentityApplied(id string) error {
	now := time.Now()
	return r.db.Model(&cloud_model.IdentityEvent{}).Where("id = ?", id).
		Updates(map[string]any{"applied_at": &now, "error": ""}).Error
}

func (r *Repository) ListIdentityEvents(instanceID string, limit int) ([]cloud_model.IdentityEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []cloud_model.IdentityEvent
	err := r.db.Where("instance_id = ?", instanceID).Order("received_at DESC").Limit(limit + 1).Find(&out).Error
	if len(out) > limit {
		out = out[:limit]
	}
	return out, err
}

func (r *Repository) ListContactIdentities(instanceID, contactID string) ([]cloud_model.ContactIdentity, error) {
	var out []cloud_model.ContactIdentity
	err := r.db.Where("instance_id = ? AND contact_id = ?", instanceID, contactID).
		Order("valid_from").Find(&out).Error
	return out, err
}

func (r *Repository) CloudHealthCounts() (HealthSnapshot, error) {
	var snap HealthSnapshot
	if err := r.db.Model(&cloud_model.Credentials{}).Count(&snap.Credentials).Error; err != nil {
		return snap, err
	}
	if err := r.db.Model(&cloud_model.Credentials{}).Where("registered = ?", true).Count(&snap.Registered).Error; err != nil {
		return snap, err
	}
	if err := r.db.Model(&cloud_model.Credentials{}).Where("last_verified_at IS NOT NULL").Count(&snap.Verified).Error; err != nil {
		return snap, err
	}
	if err := r.db.Model(&cloud_model.Contact{}).Count(&snap.Contacts).Error; err != nil {
		return snap, err
	}
	if err := r.db.Model(&cloud_model.Template{}).Count(&snap.Templates).Error; err != nil {
		return snap, err
	}
	return snap, nil
}

func (r *Repository) CountMessagesByContact(instanceID, contactID string) (int64, error) {
	var n int64
	err := r.db.Model(&message_model.Message{}).
		Where("instance_id = ? AND contact_id = ?", instanceID, contactID).Count(&n).Error
	return n, err
}

func (r *Repository) ListAccountEvents(instanceID, field string, limit, offset int) ([]cloud_model.AccountEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	q := r.db.Where("instance_id = ?", instanceID)
	if field != "" {
		q = q.Where("field = ?", field)
	}
	var out []cloud_model.AccountEvent
	err := q.Order("received_at DESC").Limit(limit).Offset(offset).Find(&out).Error
	return out, err
}

func (r *Repository) GetAccountEvent(instanceID, eventID string) (*cloud_model.AccountEvent, error) {
	var e cloud_model.AccountEvent
	if err := r.db.Where("instance_id = ? AND id = ?", instanceID, eventID).First(&e).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &e, nil
}

func (r *Repository) LatestWebhookAt(instanceID string) (time.Time, error) {
	var latest *time.Time
	q := r.db.Model(&cloud_model.WebhookEvent{}).Select("MAX(processed_at)")
	if instanceID != "" {
		q = q.Where("instance_id = ?", instanceID)
	}
	if err := q.Scan(&latest).Error; err != nil || latest == nil {
		return time.Time{}, err
	}
	return *latest, nil
}
