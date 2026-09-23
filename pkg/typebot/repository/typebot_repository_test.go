package typebot_repository

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	typebot_model "github.com/evolution-foundation/evolution-go/pkg/typebot/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newMockRepo(t *testing.T) (*typebotRepository, sqlmock.Sqlmock) {
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
	return NewTypebotRepository(gormDB).(*typebotRepository), mock
}

func botRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "instance_id", "enabled", "url", "typebot"}).
		AddRow("bot-1", "inst-1", true, "https://viewer.example", "flow")
}

// ProcessMessage calls GetActiveBot for every inbound message. A second call
// must be served from cache: sqlmock fails on any query that was not expected.
func TestGetActiveBotIsCached(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT \* FROM "typebots" WHERE instance_id`).WillReturnRows(botRows())

	for i := 0; i < 3; i++ {
		bot, err := repo.GetActiveBot("inst-1")
		if err != nil {
			t.Fatalf("GetActiveBot %d: %v", i, err)
		}
		if bot == nil || bot.Id != "bot-1" {
			t.Fatalf("GetActiveBot %d returned %+v", i, bot)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected extra queries: %v", err)
	}
}

// An instance with no bot must cache the negative result too, otherwise every
// message still pays a query.
func TestGetActiveBotCachesNegativeResult(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT \* FROM "typebots" WHERE instance_id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "instance_id", "enabled"}))

	for i := 0; i < 3; i++ {
		bot, err := repo.GetActiveBot("inst-1")
		if err != nil {
			t.Fatalf("GetActiveBot %d: %v", i, err)
		}
		if bot != nil {
			t.Fatalf("expected nil bot, got %+v", bot)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected extra queries: %v", err)
	}
}

// The returned bot must be a copy so callers cannot mutate the cached row.
func TestGetActiveBotReturnsCopy(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT \* FROM "typebots" WHERE instance_id`).WillReturnRows(botRows())

	first, err := repo.GetActiveBot("inst-1")
	if err != nil {
		t.Fatal(err)
	}
	first.URL = "mutated"

	second, err := repo.GetActiveBot("inst-1")
	if err != nil {
		t.Fatal(err)
	}
	if second.URL != "https://viewer.example" {
		t.Fatalf("cached bot mutated through returned copy: %q", second.URL)
	}
}

// A bot update must invalidate the cached entry so the next message sees it.
func TestUpdateBotInvalidatesCache(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT \* FROM "typebots" WHERE instance_id`).WillReturnRows(botRows())
	mock.ExpectExec(`UPDATE "typebots"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT \* FROM "typebots" WHERE instance_id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "instance_id", "enabled", "url", "typebot"}).
			AddRow("bot-1", "inst-1", true, "https://new.example", "flow"))

	if _, err := repo.GetActiveBot("inst-1"); err != nil {
		t.Fatal(err)
	}

	if err := repo.UpdateBot(&typebot_model.Typebot{Id: "bot-1", InstanceID: "inst-1", Enabled: true, URL: "https://new.example"}); err != nil {
		t.Fatalf("UpdateBot: %v", err)
	}

	got, err := repo.GetActiveBot("inst-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://new.example" {
		t.Fatalf("cache not invalidated after update: %q", got.URL)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
