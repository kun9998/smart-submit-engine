package main

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-sql-driver/mysql"
)

var mysqlDatabaseNameRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

type mysqlConnParams struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

func (p mysqlConnParams) normalizedHostPort() (host string, port int) {
	host = strings.TrimSpace(p.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	port = p.Port
	if port <= 0 {
		port = 3306
	}
	return host, port
}

func (p mysqlConnParams) cfg(db string) (mysql.Config, error) {
	host, port := p.normalizedHostPort()
	user := strings.TrimSpace(p.User)
	if user == "" {
		return mysql.Config{}, fmt.Errorf("请填写数据库用户名")
	}
	return mysql.Config{
		User:                 user,
		Passwd:               p.Password,
		Net:                  "tcp",
		Addr:                 fmt.Sprintf("%s:%d", host, port),
		DBName:               db,
		ParseTime:            true,
		AllowNativePasswords: true,
		Params:               map[string]string{"charset": "utf8mb4"},
	}, nil
}

func (p mysqlConnParams) DSN() (string, error) {
	db := strings.TrimSpace(p.Database)
	if db == "" {
		return "", fmt.Errorf("请填写数据库名称")
	}
	cfg, err := p.cfg(db)
	if err != nil {
		return "", err
	}
	return cfg.FormatDSN(), nil
}

func (p mysqlConnParams) adminDSN() (string, error) {
	cfg, err := p.cfg("")
	if err != nil {
		return "", err
	}
	return cfg.FormatDSN(), nil
}

func validateMySQLDatabaseName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("请填写数据库名称")
	}
	if !mysqlDatabaseNameRe.MatchString(name) {
		return fmt.Errorf("数据库名称仅允许字母、数字和下划线")
	}
	return nil
}

func ensureMySQLDatabase(ctx context.Context, p mysqlConnParams, database string) error {
	if err := validateMySQLDatabaseName(database); err != nil {
		return err
	}
	dsn, err := p.adminDSN()
	if err != nil {
		return err
	}
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer conn.Close()
	q := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		database,
	)
	if _, err := conn.ExecContext(ctx, q); err != nil {
		return fmt.Errorf("自动创建插件库失败: %w", err)
	}
	return nil
}

func normalizeMainTablePrefix(s string) string {
	s = strings.Trim(strings.TrimSpace(s), "_")
	if s == "" {
		return "love_learn"
	}
	return s
}

func normalizePluginTablePrefix(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "tj_"
	}
	if !strings.HasSuffix(s, "_") {
		s += "_"
	}
	return s
}
