package storage

import (
	"fmt"
	"mooc-manus/config"

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
	return nil
}
