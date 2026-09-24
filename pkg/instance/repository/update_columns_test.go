package instance_repository

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newInstanceMockRepo(t *testing.T) (*instanceRepository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	return NewInstanceRepository(gormDB).(*instanceRepository), mock
}

// UpdateConnectSettings must emit a partial UPDATE: only the columns present in
// the map are written. This is what the pairing path relies on instead of the
// full-row Update/Save.
func TestUpdateConnectSettingsIsPartial(t *testing.T) {
	repo, mock := newInstanceMockRepo(t)

	// Only the four pairing columns may appear; no "proxy"/"name"/"client_name".
	mock.ExpectExec(`UPDATE "instances" SET "connected"=\$1,"disconnect_reason"=\$2,"jid"=\$3,"qrcode"=\$4 WHERE id = \$5`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.UpdateConnectSettings("11111111-1111-1111-1111-111111111111", map[string]interface{}{
		"qrcode":            "",
		"connected":         true,
		"disconnect_reason": "",
		"jid":               "5511999999999@s.whatsapp.net",
	})
	if err != nil {
		t.Fatalf("UpdateConnectSettings: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expected a partial UPDATE, got: %v", err)
	}
}

// Update() is still a full-row write (kept for callers that genuinely replace
// the row), so a partial update must go through UpdateConnectSettings.
func TestUpdateRemainsFullRow(t *testing.T) {
	repo, mock := newInstanceMockRepo(t)

	// Match the whole column list to document that Update is a full-row Save.
	mock.ExpectExec(`UPDATE "instances" SET "name"=\$1,.*"ignore_status"=\$22 WHERE "id" = \$23`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	inst := &instance_model.Instance{Id: "11111111-1111-1111-1111-111111111111", Name: "n", Token: "t"}
	if err := repo.Update(inst); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("Update is expected to stay a full-row write: %v", err)
	}
}
