package instance_repository

import (
	"fmt"

	applog "github.com/evolution-foundation/evolution-go/pkg/applog"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/tokencrypt"
	"github.com/google/uuid"
	"gorm.io/gorm"

	label_model "github.com/evolution-foundation/evolution-go/pkg/label/model"
	label_repository "github.com/evolution-foundation/evolution-go/pkg/label/repository"

	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	message_repository "github.com/evolution-foundation/evolution-go/pkg/message/repository"
)

type InstanceRepository interface {
	Create(instance instance_model.Instance) (*instance_model.Instance, error)
	GetInstanceByID(instanceId string) (*instance_model.Instance, error)
	GetConnectedInstanceByID(instanceId string) (*instance_model.Instance, error)
	GetInstanceByToken(token string) (*instance_model.Instance, error)
	// GetInstanceByTokenHash looks an instance up by the deterministic HMAC of
	// its token, so the plaintext credential need not be stored or compared.
	GetInstanceByTokenHash(tokenHash string) (*instance_model.Instance, error)
	// UpdateToken rewrites an instance's token columns (hash + ciphertext).
	UpdateToken(instanceId, tokenHash, tokenEnc string) error
	GetInstanceByName(name string) (*instance_model.Instance, error)
	Update(*instance_model.Instance) error
	UpdateConnected(userId string, status bool, disconnectReason string) error
	UpdateQrcode(userId string, qr string) error
	UpdateProxy(userId string, proxy string) error
	UpdateJid(userId string, jid string) error
	UpdateHmacKey(userId string, encryptedKey string) error
	UpdateS3Config(instanceId string, updates map[string]interface{}) error
	UpdateName(instanceId string, name string) error
	UpdateConnectSettings(instanceId string, updates map[string]interface{}) error
	GetAllConnectedInstances() ([]*instance_model.Instance, error)
	GetAllConnectedInstancesByClientName(clientName string) ([]*instance_model.Instance, error)
	GetAllPairedInstances() ([]*instance_model.Instance, error)
	GetAllPairedInstancesByClientName(clientName string) ([]*instance_model.Instance, error)
	GetAll(clientName string) ([]*instance_model.Instance, error)
	Delete(instanceId string) error
	GetAdvancedSettings(instanceId string) (*instance_model.AdvancedSettings, error)
	UpdateAdvancedSettings(instanceId string, settings *instance_model.AdvancedSettings) error
}

type instanceRepository struct {
	db          *gorm.DB
	labelRepo   label_repository.LabelRepository
	messageRepo message_repository.MessageRepository
	// tokenCodec decrypts the stored token ciphertext into the in-memory Token
	// field on every read, so callers keep using it normally while the database
	// only ever holds the hash + ciphertext. Nil keeps the legacy plaintext path.
	tokenCodec *tokencrypt.Codec
}

// hydrate decrypts the at-rest token into the in-memory field. Best-effort: a row
// written before encryption existed already has Token populated and no TokenEnc,
// so it passes through unchanged.
func (i *instanceRepository) hydrate(instance *instance_model.Instance) {
	if instance == nil || i.tokenCodec == nil {
		return
	}
	if instance.Token != "" || instance.TokenEnc == "" {
		return
	}
	if plaintext, err := i.tokenCodec.Decrypt(instance.TokenEnc); err == nil {
		instance.Token = plaintext
	}
}

func (i *instanceRepository) hydrateAll(list []*instance_model.Instance) {
	for _, inst := range list {
		i.hydrate(inst)
	}
}

func (i *instanceRepository) Create(instance instance_model.Instance) (*instance_model.Instance, error) {
	if err := i.db.Create(&instance).Error; err != nil {
		return nil, err
	}
	return &instance, nil
}

func (i *instanceRepository) GetInstanceByToken(token string) (*instance_model.Instance, error) {
	var instance instance_model.Instance
	err := i.db.Where("token = ?", token).First(&instance).Error
	if err != nil {
		return nil, err
	}

	i.hydrate(&instance)
	return &instance, nil
}

// GetInstanceByTokenHash finds the instance whose stored token hash matches.
func (i *instanceRepository) GetInstanceByTokenHash(tokenHash string) (*instance_model.Instance, error) {
	var instance instance_model.Instance
	err := i.db.Where("token_hash = ?", tokenHash).First(&instance).Error
	if err != nil {
		return nil, err
	}

	i.hydrate(&instance)
	return &instance, nil
}

// UpdateToken rewrites the token columns for an instance.
func (i *instanceRepository) UpdateToken(instanceId, tokenHash, tokenEnc string) error {
	return i.db.Model(&instance_model.Instance{}).
		Where("id = ?", instanceId).
		Updates(map[string]interface{}{"token_hash": tokenHash, "token_enc": tokenEnc}).
		Error
}

func (i *instanceRepository) GetInstanceByName(name string) (*instance_model.Instance, error) {
	var instance instance_model.Instance
	err := i.db.Where("name = ?", name).First(&instance).Error
	if err != nil {
		return nil, err
	}

	i.hydrate(&instance)
	return &instance, nil
}

func (i *instanceRepository) GetInstanceByID(instanceId string) (*instance_model.Instance, error) {
	// Valida o formato do UUID
	if _, err := uuid.Parse(instanceId); err != nil {
		return nil, fmt.Errorf("invalid UUID format: %v", err)
	}

	var instance instance_model.Instance
	err := i.db.Where("id = ?", instanceId).First(&instance).Error
	if err != nil {
		return nil, err
	}

	i.hydrate(&instance)
	return &instance, nil
}

func (i *instanceRepository) GetConnectedInstanceByID(instanceId string) (*instance_model.Instance, error) {
	var instance instance_model.Instance
	err := i.db.Where("id = ? AND connected = ?", instanceId, true).First(&instance).Error
	if err != nil {
		return nil, err
	}

	i.hydrate(&instance)
	return &instance, nil
}

func (i *instanceRepository) Update(instance *instance_model.Instance) error {
	err := i.db.Save(&instance).Error
	if err != nil {
		applog.Logger.LogError("Error updating instance in DB: %v", err)
	}
	return err
}

func (i *instanceRepository) UpdateConnected(userId string, status bool, disconnectReason string) error {
	return i.db.Model(&instance_model.Instance{}).Where("id = ?", userId).Update("connected", status).Update("disconnect_reason", disconnectReason).Error
}

func (i *instanceRepository) UpdateQrcode(userId string, qr string) error {
	return i.db.Model(&instance_model.Instance{}).Where("id = ?", userId).Update("qrcode", qr).Error
}

func (i *instanceRepository) UpdateProxy(userId string, proxy string) error {
	return i.db.Model(&instance_model.Instance{}).Where("id = ?", userId).Update("proxy", proxy).Error
}

// UpdateHmacKey persists the (already encrypted) per-instance webhook signing
// key. An empty string clears it.
func (i *instanceRepository) UpdateHmacKey(userId string, encryptedKey string) error {
	return i.db.Model(&instance_model.Instance{}).Where("id = ?", userId).Update("hmac_key", encryptedKey).Error
}

func (i *instanceRepository) UpdateJid(userId string, jid string) error {
	return i.db.Model(&instance_model.Instance{}).Where("id = ?", userId).Update("jid", jid).Error
}

func (i *instanceRepository) UpdateName(instanceId string, name string) error {
	if _, err := uuid.Parse(instanceId); err != nil {
		return fmt.Errorf("invalid UUID format: %v", err)
	}
	return i.db.Model(&instance_model.Instance{}).Where("id = ?", instanceId).Update("name", name).Error
}

func (i *instanceRepository) UpdateConnectSettings(instanceId string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	err := i.db.Model(&instance_model.Instance{}).Where("id = ?", instanceId).Updates(updates).Error
	if err != nil {
		applog.Logger.LogError("Error updating connect settings in DB: %v", err)
	}
	return err
}

// UpdateS3Config writes the per-instance S3 columns (a partial update, so an
// omitted field is left untouched).
func (i *instanceRepository) UpdateS3Config(instanceId string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	err := i.db.Model(&instance_model.Instance{}).Where("id = ?", instanceId).Updates(updates).Error
	if err != nil {
		applog.Logger.LogError("Error updating S3 config in DB: %v", err)
	}
	return err
}

func (i *instanceRepository) GetAllConnectedInstances() ([]*instance_model.Instance, error) {
	var instances []*instance_model.Instance
	err := i.db.Where("connected = ?", true).Find(&instances).Error
	if err != nil {
		return nil, err
	}

	i.hydrateAll(instances)
	return instances, nil
}

func (i *instanceRepository) GetAllConnectedInstancesByClientName(clientName string) ([]*instance_model.Instance, error) {
	var instances []*instance_model.Instance
	err := i.db.Where("connected = ? AND client_name = ?", true, clientName).Find(&instances).Error
	if err != nil {
		return nil, err
	}

	i.hydrateAll(instances)
	return instances, nil
}

// GetAllPairedInstances returns instances that have already completed pairing
// (they have a JID). The Connected column is intentionally not used: it
// reflects the live socket, which is always false right after a restart, so
// relying on it would leave every paired session offline until a manual
// reconnect. StartClient safely skips instances whose session is no longer in
// the whatsmeow auth store, so a stale JID cannot start a QR loop here.
func (i *instanceRepository) GetAllPairedInstances() ([]*instance_model.Instance, error) {
	var instances []*instance_model.Instance
	err := i.db.Where("jid IS NOT NULL AND jid <> ''").Find(&instances).Error
	if err != nil {
		return nil, err
	}

	i.hydrateAll(instances)
	return instances, nil
}

func (i *instanceRepository) GetAllPairedInstancesByClientName(clientName string) ([]*instance_model.Instance, error) {
	var instances []*instance_model.Instance
	err := i.db.Where("jid IS NOT NULL AND jid <> '' AND client_name = ?", clientName).Find(&instances).Error
	if err != nil {
		return nil, err
	}

	i.hydrateAll(instances)
	return instances, nil
}

func (i *instanceRepository) GetAll(clientName string) ([]*instance_model.Instance, error) {
	var instances []*instance_model.Instance
	err := i.db.Where("client_name = ?", clientName).Find(&instances).Error
	if err != nil {
		return nil, err
	}

	i.hydrateAll(instances)
	return instances, nil
}

func (i *instanceRepository) Delete(instanceId string) error {
	return i.db.Transaction(func(tx *gorm.DB) error {
		// Deleta todas as labels associadas à instância
		if err := tx.Where("instance_id = ?", instanceId).Delete(&label_model.Label{}).Error; err != nil {
			return fmt.Errorf("erro ao deletar labels: %v", err)
		}

		// Deleta todas as mensagens associadas à instância. (This used to filter
		// on source, which holds the contact number, so it never matched.)
		if err := tx.Where("instance_id = ?", instanceId).Delete(&message_model.Message{}).Error; err != nil {
			return fmt.Errorf("erro ao deletar mensagens: %v", err)
		}

		// Deleta a instância
		if err := tx.Where("id = ?", instanceId).Delete(&instance_model.Instance{}).Error; err != nil {
			return fmt.Errorf("erro ao deletar instância: %v", err)
		}

		return nil
	})
}

func (i *instanceRepository) GetAdvancedSettings(instanceId string) (*instance_model.AdvancedSettings, error) {
	// Valida o formato do UUID
	if _, err := uuid.Parse(instanceId); err != nil {
		return nil, fmt.Errorf("invalid UUID format: %v", err)
	}

	var instance instance_model.Instance
	err := i.db.Select("always_online, reject_call, msg_reject_call, read_messages, ignore_groups, ignore_status").
		Where("id = ?", instanceId).First(&instance).Error
	if err != nil {
		return nil, err
	}

	settings := &instance_model.AdvancedSettings{
		AlwaysOnline:  instance_model.BoolPtr(instance.AlwaysOnline),
		RejectCall:    instance_model.BoolPtr(instance.RejectCall),
		MsgRejectCall: instance.MsgRejectCall,
		ReadMessages:  instance_model.BoolPtr(instance.ReadMessages),
		IgnoreGroups:  instance_model.BoolPtr(instance.IgnoreGroups),
		IgnoreStatus:  instance_model.BoolPtr(instance.IgnoreStatus),
	}

	return settings, nil
}

func (i *instanceRepository) UpdateAdvancedSettings(instanceId string, settings *instance_model.AdvancedSettings) error {
	// Valida o formato do UUID
	if _, err := uuid.Parse(instanceId); err != nil {
		return fmt.Errorf("invalid UUID format: %v", err)
	}

	updates := buildAdvancedSettingsUpdates(settings)
	if len(updates) == 0 {
		return nil
	}

	err := i.db.Model(&instance_model.Instance{}).Where("id = ?", instanceId).Updates(updates).Error
	if err != nil {
		applog.Logger.LogError("Error updating advanced settings in DB: %v", err)
		return err
	}

	return nil
}

// buildAdvancedSettingsUpdates only includes fields explicitly provided (*bool != nil).
// MsgRejectCall is always written on PUT so an empty string can clear the reject message.
func buildAdvancedSettingsUpdates(settings *instance_model.AdvancedSettings) map[string]interface{} {
	updates := map[string]interface{}{}
	if settings == nil {
		return updates
	}
	if settings.AlwaysOnline != nil {
		updates["always_online"] = *settings.AlwaysOnline
	}
	if settings.RejectCall != nil {
		updates["reject_call"] = *settings.RejectCall
	}
	if settings.ReadMessages != nil {
		updates["read_messages"] = *settings.ReadMessages
	}
	if settings.IgnoreGroups != nil {
		updates["ignore_groups"] = *settings.IgnoreGroups
	}
	if settings.IgnoreStatus != nil {
		updates["ignore_status"] = *settings.IgnoreStatus
	}
	if settings.MsgRejectCall != "" {
		updates["msg_reject_call"] = settings.MsgRejectCall
	}
	return updates
}

func NewInstanceRepository(db *gorm.DB, tokenCodec *tokencrypt.Codec) InstanceRepository {
	return &instanceRepository{
		db:         db,
		tokenCodec: tokenCodec,
	}
}
