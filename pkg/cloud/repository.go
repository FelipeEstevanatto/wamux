package cloud

import (
	"errors"
	"time"

	cloud_model "github.com/felipeestevanatto/wamux/pkg/cloud/model"
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
