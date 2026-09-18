package model

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sqlRecorder captures every statement GORM hands to the logger, which is how
// the tests below assert on the SQL emitted by the real download-stat helpers.
type sqlRecorder struct {
	mu         sync.Mutex
	statements []string
}

func (r *sqlRecorder) LogMode(logger.LogLevel) logger.Interface      { return r }
func (r *sqlRecorder) Info(context.Context, string, ...interface{})  {}
func (r *sqlRecorder) Warn(context.Context, string, ...interface{})  {}
func (r *sqlRecorder) Error(context.Context, string, ...interface{}) {}

func (r *sqlRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statements = append(r.statements, sql)
}

func (r *sqlRecorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.statements...)
}

func (r *sqlRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statements = nil
}

// DownloadStat.Key is a reserved word in MySQL, so every statement built from
// it must quote the column. These tests pin the SQL produced on MySQL,
// PostgreSQL and SQLite to catch a regression that would break the counter
// endpoints on MySQL only.
func newDownloadStatRecorderDB(t *testing.T, dialect string, dryRun bool) (*gorm.DB, *sqlRecorder) {
	t.Helper()

	recorder := &sqlRecorder{}
	config := &gorm.Config{
		Logger:                 recorder,
		DryRun:                 dryRun,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
	}

	var (
		db  *gorm.DB
		err error
	)
	switch dialect {
	case "mysql":
		db, err = gorm.Open(mysql.New(mysql.Config{
			DSN:                       "user:pass@tcp(127.0.0.1:3306)/db?charset=utf8mb4",
			SkipInitializeWithVersion: true,
		}), config)
	case "postgres":
		db, err = gorm.Open(postgres.New(postgres.Config{
			DSN: "host=127.0.0.1 user=user password=pass dbname=db port=5432 sslmode=disable",
		}), config)
	case "sqlite":
		dsn := "file:download_stat_" + dialect + "?mode=memory&cache=shared"
		db, err = gorm.Open(sqlite.Open(dsn), config)
	default:
		t.Fatalf("unsupported dialect %q", dialect)
	}
	require.NoError(t, err)

	return db, recorder
}

// withDownloadStatDB swaps model.DB for the duration of a single test.
func withDownloadStatDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })
}

func TestGetDownloadCountQuotesReservedKeyColumn(t *testing.T) {
	dialects := []string{"mysql", "postgres", "sqlite"}

	for _, dialect := range dialects {
		t.Run(dialect, func(t *testing.T) {
			// DryRun keeps the dialects free of any real connection: these
			// subtests only care about the statement GORM builds.
			db, recorder := newDownloadStatRecorderDB(t, dialect, true)
			withDownloadStatDB(t, db)

			_, err := GetDownloadCount("unicomp-desktop-windows")
			require.NoError(t, err)

			quoted := "`key`"
			if dialect == "postgres" {
				quoted = `"key"`
			}

			statements := recorder.all()
			require.NotEmpty(t, statements, "expected GetDownloadCount to build a statement")
			for _, sql := range statements {
				require.NotContains(t, sql, "WHERE key =", "unquoted reserved word in %s", dialect)
				require.Contains(t, sql, quoted, "expected quoted column in %s: %s", dialect, sql)
			}
		})
	}
}

func TestIncrementDownloadCountQuotesReservedKeyColumn(t *testing.T) {
	dialects := []string{"mysql", "postgres", "sqlite"}

	for _, dialect := range dialects {
		t.Run(dialect, func(t *testing.T) {
			db, recorder := newDownloadStatRecorderDB(t, dialect, true)
			withDownloadStatDB(t, db)

			_, err := IncrementDownloadCount("unicomp-desktop-windows")
			require.NoError(t, err)

			quoted := "`key`"
			if dialect == "postgres" {
				quoted = `"key"`
			}

			statements := recorder.all()
			require.NotEmpty(t, statements, "expected IncrementDownloadCount to build a statement")
			for _, sql := range statements {
				require.Contains(t, sql, quoted, "expected quoted column in %s: %s", dialect, sql)
			}
		})
	}
}

func TestGetDownloadCountReturnsZeroWhenUnset(t *testing.T) {
	db, _ := newDownloadStatRecorderDB(t, "sqlite", false)
	require.NoError(t, db.AutoMigrate(&DownloadStat{}))
	withDownloadStatDB(t, db)

	count, err := GetDownloadCount("unicomp-desktop-windows")

	require.NoError(t, err)
	require.Zero(t, count)
}

func TestIncrementDownloadCountIsCumulative(t *testing.T) {
	db, _ := newDownloadStatRecorderDB(t, "sqlite", false)
	require.NoError(t, db.AutoMigrate(&DownloadStat{}))
	withDownloadStatDB(t, db)

	for expected := int64(1); expected <= 3; expected++ {
		count, err := IncrementDownloadCount("unicomp-desktop-windows")
		require.NoError(t, err)
		require.EqualValues(t, expected, count)
	}

	other, err := GetDownloadCount("unicomp-ai-windows")
	require.NoError(t, err)
	require.Zero(t, other, "counters must stay independent per product key")
}
