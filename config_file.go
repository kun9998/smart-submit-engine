package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const configFileName = "config.yaml"

// AuthConfig 配置兼容字段（授权已移除，仅保留 yaml 兼容）。
type AuthConfig struct {
	Authcode string `yaml:"authcode,omitempty"`
	Domain   string `yaml:"domain,omitempty"`
	CertDir  string `yaml:"cert_dir,omitempty"`
}


func configFilePath() string {
	return filepath.Join(".", configFileName)
}

// normalizeAdminListenAddr 规范化管理端监听地址，支持 ":8091"、"8091"、"0.0.0.0:8091"。
func normalizeAdminListenAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ":8090"
	}
	if strings.HasPrefix(addr, ":") {
		return addr
	}
	if _, err := strconv.Atoi(addr); err == nil {
		return ":" + addr
	}
	if !strings.Contains(addr, ":") {
		return ":" + addr
	}
	return addr
}

func defaultFileConfig() fileConfig {
	fc := fileConfig{
		TablePrefix: "love_learn",
	}
	fc.Redis.Addr = "127.0.0.1:6379"
	fc.Admin.Enabled = true
	fc.Admin.Addr = ":8090"
	fc.PluginTablePrefix = defaultPluginTablePrefix
	return fc
}

func applyConfigDefaults(fc *fileConfig) {
	if fc == nil {
		return
	}
	if fc.Redis.Addr == "" {
		fc.Redis.Addr = "127.0.0.1:6379"
	}
	fc.Admin.Addr = normalizeAdminListenAddr(fc.Admin.Addr)
	if fc.TablePrefix == "" {
		fc.TablePrefix = "love_learn"
	}
	if fc.PluginTablePrefix == "" {
		fc.PluginTablePrefix = defaultPluginTablePrefix
	}
}

func isDefaultHTTPSecurity(hs HTTPSecurity) bool {
	return len(hs.HostWhitelist) == 0 && hs.BlockPrivateNetworks == nil && !hs.AllowInsecureHTTPToLAN
}

func prepareConfigForSave(fc *fileConfig) {
	if fc == nil {
		return
	}
	fc.Auth.Domain = strings.TrimSpace(fc.Auth.Domain)
	fc.Auth.CertDir = strings.TrimSpace(fc.Auth.CertDir)
	if fc.Auth.Domain == "" && fc.Auth.CertDir == "" {
		fc.Auth = AuthConfig{Authcode: fc.Auth.Authcode}
	}
	if isDefaultHTTPSecurity(fc.HTTPSecurity) {
		fc.HTTPSecurity = HTTPSecurity{}
	}
	if strings.TrimSpace(fc.PluginMySQLDSN) != "" || strings.TrimSpace(fc.MainMySQLDSN) != "" {
		fc.MySQLDSN = ""
	}
}

type legacyPluginConfig struct {
	MainMySQLDSN      string `yaml:"main_mysql_dsn"`
	MySQLDSN          string `yaml:"mysql_dsn,omitempty"`
	PluginMySQLDSN    string `yaml:"plugin_mysql_dsn"`
	TablePrefix       string `yaml:"table_prefix"`
	PluginTablePrefix string `yaml:"plugin_table_prefix"`
	RedisAddr         string `yaml:"redis_addr"`
	RedisPass         string `yaml:"redis_pass"`
	RedisDB           int    `yaml:"redis_db"`
	Installed         bool   `yaml:"installed"`
}

func migrateLegacyPluginConfig(fc *fileConfig, migrateInstalled bool) {
	if fc == nil || fc.Installed {
		return
	}
	b, err := os.ReadFile(filepath.Join(".", "plugin_config.yaml"))
	if err != nil {
		return
	}
	var legacy legacyPluginConfig
	if err := yaml.Unmarshal(b, &legacy); err != nil || !legacy.Installed {
		return
	}
	if s := strings.TrimSpace(legacy.MainMySQLDSN); s != "" {
		fc.MainMySQLDSN = s
	}
	if s := strings.TrimSpace(legacy.PluginMySQLDSN); s != "" {
		fc.PluginMySQLDSN = s
	}
	if s := strings.TrimSpace(legacy.MySQLDSN); s != "" && fc.PluginMySQLDSN == "" {
		fc.MySQLDSN = s
	}
	if s := strings.TrimSpace(legacy.TablePrefix); s != "" {
		fc.TablePrefix = s
	}
	if s := strings.TrimSpace(legacy.PluginTablePrefix); s != "" {
		fc.PluginTablePrefix = s
	}
	if s := strings.TrimSpace(legacy.RedisAddr); s != "" {
		fc.Redis.Addr = s
	}
	fc.Redis.Pass = legacy.RedisPass
	if legacy.RedisDB >= 0 {
		fc.Redis.DB = legacy.RedisDB
	}
	if migrateInstalled {
		fc.Installed = true
	}
}

func loadConfigFile() (fileConfig, error) {
	b, err := os.ReadFile(configFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			fc := defaultFileConfig()
			migrateLegacyPluginConfig(&fc, true)
			if fc.Installed {
				_ = saveConfigFile(fc)
			}
			return fc, nil
		}
		var fc fileConfig
		return fc, err
	}
	var fc fileConfig
	if err := yaml.Unmarshal(b, &fc); err != nil {
		return fc, err
	}
	applyConfigDefaults(&fc)
	migrateLegacyPluginConfig(&fc, false)
	return fc, nil
}

func saveConfigFile(fc fileConfig) error {
	applyConfigDefaults(&fc)
	prepareConfigForSave(&fc)
	out, err := yaml.Marshal(&fc)
	if err != nil {
		return fmt.Errorf("序列化 config.yaml 失败: %w", err)
	}
	if err := os.WriteFile(configFilePath(), out, 0600); err != nil {
		return fmt.Errorf("写入 config.yaml 失败: %w", err)
	}
	return nil
}

func (fc *fileConfig) resolvedMainDSN() string {
	if s := strings.TrimSpace(fc.MainMySQLDSN); s != "" {
		return s
	}
	return strings.TrimSpace(fc.MySQLDSN)
}

func (fc *fileConfig) resolvedPluginDSN() string {
	if s := strings.TrimSpace(fc.PluginMySQLDSN); s != "" {
		return s
	}
	return strings.TrimSpace(fc.MySQLDSN)
}

func applyFileConfigToRuntime(fc *fileConfig) {
	if fc == nil {
		return
	}
	if fc.Redis.Addr != "" {
		redisConfig.Addr = fc.Redis.Addr
	} else {
		redisConfig.Addr = "127.0.0.1:6379"
	}
	redisConfig.Pass = fc.Redis.Pass
	redisConfig.DB = fc.Redis.DB

	if s := fc.resolvedMainDSN(); s != "" {
		mysqlDSN = s
	}
	if s := normalizeMainTablePrefix(fc.TablePrefix); s != "" {
		tablePrefix = s
	}
	if s := strings.TrimSpace(fc.PluginTablePrefix); s != "" {
		setPluginTablePrefix(s)
	}

	initHTTPSecurityFromConfig(fc)
	initAdminFromConfig(fc)
}

func isAppInstalledOnDisk() bool {
	fc, err := loadConfigFile()
	if err != nil {
		return false
	}
	return fc.Installed
}


func saveInstallConfig(mainDSN, pluginDSN, mainPrefix, redisAddr, redisPass string, redisDB int) error {
	fc, err := loadConfigFile()
	if err != nil {
		return err
	}
	fc.MainMySQLDSN = strings.TrimSpace(mainDSN)
	fc.PluginMySQLDSN = strings.TrimSpace(pluginDSN)
	fc.MySQLDSN = ""
	fc.TablePrefix = normalizeMainTablePrefix(mainPrefix)
	fc.PluginTablePrefix = pluginTablePrefix
	fc.Installed = true
	fc.Auth = AuthConfig{}
	if strings.TrimSpace(redisAddr) != "" {
		fc.Redis.Addr = strings.TrimSpace(redisAddr)
	}
	fc.Redis.Pass = redisPass
	fc.Redis.DB = redisDB
	return saveConfigFile(fc)
}
