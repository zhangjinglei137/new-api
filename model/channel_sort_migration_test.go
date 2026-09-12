package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// testChannelSortMigration 验证 Channel.Sort 字段在三种数据库下的迁移兼容性：
// 新建库场景（空表 AutoMigrate）与升级存量库场景（无 sort 列的旧表 AutoMigrate），
// 各自重复 AutoMigrate 两次确认幂等。
func testChannelSortMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	tableName := fmt.Sprintf("channel_sort_migration_%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = db.Migrator().DropTable(tableName) })

	t.Run("fresh_database", func(t *testing.T) {
		// 新建库场景：空表直接 AutoMigrate，sort 列应被创建
		for range 2 {
			require.NoError(t, db.Table(tableName).AutoMigrate(&Channel{}))
		}
		assert.True(t, db.Migrator().HasColumn(tableName, "sort"))
	})

	t.Run("legacy_database", func(t *testing.T) {
		legacyTableName := tableName + "_legacy"
		t.Cleanup(func() { _ = db.Migrator().DropTable(legacyTableName) })

		// 升级存量库场景：手动创建无 sort 列的旧版 channels 结构子集（id/key/name）
		require.NoError(t, db.Exec(
			"CREATE TABLE ? ("+
				"? integer PRIMARY KEY, "+
				"? text NOT NULL, "+
				"? text)",
			clause.Table{Name: legacyTableName},
			clause.Column{Name: "id"},
			clause.Column{Name: "key"},
			clause.Column{Name: "name"},
		).Error)
		assert.False(t, db.Migrator().HasColumn(legacyTableName, "sort"))

		// AutoMigrate 应补齐 sort 列，重复两次确认幂等
		for range 2 {
			require.NoError(t, db.Table(legacyTableName).AutoMigrate(&Channel{}))
		}
		assert.True(t, db.Migrator().HasColumn(legacyTableName, "sort"))
	})
}

func TestChannelSortMigrationSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	testChannelSortMigration(t, db)
}

func TestChannelSortMigrationMySQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not configured")
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	testChannelSortMigration(t, db)
}

func TestChannelSortMigrationPostgreSQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not configured")
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	testChannelSortMigration(t, db)
}
