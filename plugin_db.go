package main

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

//go:embed plugin_install.sql
var pluginInstallSQL string

const defaultPluginTablePrefix = "tj_"

var pluginTablePrefix = defaultPluginTablePrefix

func setPluginTablePrefix(prefix string) {
	pluginTablePrefix = normalizePluginTablePrefix(prefix)
}

var (
	pluginDB  *sql.DB
	pluginDSN string
)

func resolvePluginDSN(fc *fileConfig) string {
	if fc == nil {
		loaded, err := loadConfigFile()
		if err != nil {
			return ""
		}
		fc = &loaded
	}
	return fc.resolvedPluginDSN()
}

func pluginTable(name string) string {
	return pluginTablePrefix + name
}

func openPluginDB(dsn string) (*sql.DB, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, fmt.Errorf("插件数据库 DSN 为空")
	}
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(10)
	conn.SetMaxIdleConns(3)
	conn.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func initPluginDBFromFile() {
	fc, err := loadConfigFile()
	if err != nil {
		log.Printf("读取 config.yaml 失败: %v", err)
		return
	}
	initPluginDB(&fc)
}

func initPluginDB(fc *fileConfig) {
	pluginDSN = resolvePluginDSN(fc)
	if pluginDSN == "" {
		log.Printf("插件数据库未配置，管理端首次访问将引导安装")
		return
	}
	conn, err := openPluginDB(pluginDSN)
	if err != nil {
		log.Printf("插件数据库连接失败: %v", err)
		return
	}
	pluginDB = conn
	ensureSystemMetaSchema(context.Background())
	ensureAdminUserProfileSchema(context.Background())
	ensureHuoyuanRuntimeSchema(context.Background())
	if err := reloadRuntimeConfigFromPluginDB(context.Background()); err != nil {
		log.Printf("加载运行时配置失败: %v", err)
	}
	loadAIConfigFromPluginDB(context.Background())
	loadOpsConfigFromPluginDB(context.Background())
	loadInternalEnqueueSecretFromPluginDB(context.Background())
	loadOpsDailyReportFromMeta(context.Background())
	log.Printf("插件数据库已连接")
}

func setPluginDB(conn *sql.DB, dsn string) {
	if pluginDB != nil && pluginDB != conn {
		_ = pluginDB.Close()
	}
	pluginDB = conn
	pluginDSN = dsn
}

func pluginDBReady() bool {
	return pluginDB != nil
}

func appendDSNParam(dsn, key, value string) string {
	if strings.Contains(dsn, key+"=") {
		return dsn
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + key + "=" + value
}

func pluginInstallSQLForPrefix(tablePrefix string) string {
	prefix := normalizePluginTablePrefix(tablePrefix)
	if prefix == defaultPluginTablePrefix {
		return pluginInstallSQL
	}
	return strings.ReplaceAll(pluginInstallSQL, "`"+defaultPluginTablePrefix, "`"+prefix)
}

func runPluginInstallSQL(ctx context.Context, dsn string, tablePrefix string) (*sql.DB, error) {
	dsn = appendDSNParam(dsn, "multiStatements", "true")
	conn, err := openPluginDB(dsn)
	if err != nil {
		return nil, err
	}
	sqlText := pluginInstallSQLForPrefix(tablePrefix)
	if _, err := conn.ExecContext(ctx, sqlText); err != nil {
		conn.Close()
		return nil, fmt.Errorf("执行 plugin_install.sql 失败: %w", err)
	}
	return conn, nil
}

func isPluginInstalled(ctx context.Context) bool {
	if pluginDB != nil {
		var n int
		err := pluginDB.QueryRowContext(ctx,
			fmt.Sprintf(`SELECT COUNT(*) FROM %s`, pluginTable("admin_user")),
		).Scan(&n)
		return err == nil && n > 0
	}
	return detectPluginInstalledOnDisk()
}

// detectPluginInstalledOnDisk 不依赖全局 pluginDB，用于启动前授权判定。
// 只要插件库核心表已存在（哪怕管理员被删），也视为已安装，必须授权。
func detectPluginInstalledOnDisk() bool {
	fc, err := loadConfigFile()
	if err != nil {
		return false
	}
	dsn := strings.TrimSpace(fc.resolvedPluginDSN())
	if dsn == "" {
		return false
	}
	prefix := strings.TrimSpace(fc.PluginTablePrefix)
	if prefix == "" {
		prefix = defaultPluginTablePrefix
	}
	conn, err := openPluginDB(dsn)
	if err != nil {
		return false
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	prefix = normalizePluginTablePrefix(prefix)
	for _, name := range []string{"admin_user", "submit_platform", "system_meta", "huoyuan_runtime"} {
		if pluginTableExists(ctx, conn, prefix+name) {
			return true
		}
	}
	return false
}

func pluginTableExists(ctx context.Context, conn *sql.DB, table string) bool {
	var n int
	err := conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, table).Scan(&n)
	return err == nil && n > 0
}

func importPluginSeedPlatforms(ctx context.Context) error {
	for _, row := range pluginSeedPlatforms() {
		ruleJSON, err := json.Marshal(row.RuleConfig)
		if err != nil {
			continue
		}
		enabled := 0
		if row.Enabled {
			enabled = 1
		}
		q := fmt.Sprintf(`INSERT IGNORE INTO %s (platform_type, display_name, enabled, rule_config, version, remark)
			VALUES (?,?,?,?,1,?)`, pluginTable("submit_platform"))
		_, _ = pluginDB.ExecContext(ctx, q, row.PlatformType, row.DisplayName, enabled, ruleJSON, row.Remark)
	}
	return nil
}
