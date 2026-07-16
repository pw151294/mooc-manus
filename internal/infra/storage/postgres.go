package storage

import (
	"fmt"
	"mooc-manus/config"
	"mooc-manus/internal/infra/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const postgresDsn = "host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=Asia/Shanghai"

var sqlDB *gorm.DB

func GetPostgresClient() *gorm.DB {
	return sqlDB
}

func InitStorage() error {
	dbCfg := config.Cfg.Database
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  fmt.Sprintf(postgresDsn, dbCfg.Host, dbCfg.User, dbCfg.Password, dbCfg.Database, dbCfg.Port, dbCfg.Sslmode),
		PreferSimpleProtocol: true, // disables implicit prepared statement usage
	}))
	if err != nil {
		return err
	}
	sqlDB = db

	// 评测模块 AutoMigrate（按依赖顺序：snapshot/task 无外键 → instance 依赖它们 → result 依赖 instance）
	if err := db.AutoMigrate(
		&models.EvalCasePO{},
		&models.EvalTaskPO{},
		&models.EvalAgentSnapshotPO{},
		&models.EvalRunInstancePO{},
		&models.EvalResultPO{},
	); err != nil {
		return fmt.Errorf("eval AutoMigrate: %w", err)
	}

	// GIN 索引 post-hook
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_eval_case_tags_gin ON eval_case USING GIN (tags)`)

	return nil
}
