package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

const notificationConfigMetaKey = "notification_config"

type notifyKind string

const (
	notifySubmitFailure     notifyKind = "submit_failure"
	notifySubmitTimeout     notifyKind = "submit_timeout"
	notifyDBWriteFailure    notifyKind = "db_write_failure"
	notifyProcessingTimeout notifyKind = "processing_timeout"
)

type NotificationConfig struct {
	Enabled                 bool `json:"enabled"`
	NotifySubmitFailure     bool `json:"notify_submit_failure"`
	NotifySubmitTimeout     bool `json:"notify_submit_timeout"`
	NotifyDBWriteFailure    bool `json:"notify_db_write_failure"`
	NotifyProcessingTimeout bool `json:"notify_processing_timeout"`
}

var (
	notificationCfg   NotificationConfig
	notificationCfgMu sync.RWMutex
)

func defaultNotificationConfig() NotificationConfig {
	return NotificationConfig{
		Enabled:                 false,
		NotifySubmitFailure:     true,
		NotifySubmitTimeout:     true,
		NotifyDBWriteFailure:    true,
		NotifyProcessingTimeout: true,
	}
}

func normalizeNotificationConfig(c NotificationConfig) NotificationConfig {
	return c
}

func loadNotificationConfigFromPluginDB(ctx context.Context) {
	if pluginDB == nil {
		return
	}
	ensureSystemMetaSchema(ctx)
	raw, err := getSystemMeta(ctx, notificationConfigMetaKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		notificationCfgMu.Lock()
		notificationCfg = defaultNotificationConfig()
		notificationCfgMu.Unlock()
		return
	}
	var cfg NotificationConfig
	if json.Unmarshal([]byte(raw), &cfg) != nil {
		cfg = defaultNotificationConfig()
	}
	cfg = normalizeNotificationConfig(cfg)
	notificationCfgMu.Lock()
	notificationCfg = cfg
	notificationCfgMu.Unlock()
}

func saveNotificationConfig(ctx context.Context, cfg NotificationConfig) error {
	if pluginDB == nil {
		return fmt.Errorf("插件数据库未就绪")
	}
	cfg = normalizeNotificationConfig(cfg)
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := setSystemMeta(ctx, notificationConfigMetaKey, string(b)); err != nil {
		return err
	}
	notificationCfgMu.Lock()
	notificationCfg = cfg
	notificationCfgMu.Unlock()
	return nil
}

func getNotificationConfig() NotificationConfig {
	notificationCfgMu.RLock()
	defer notificationCfgMu.RUnlock()
	return notificationCfg
}

func enableNotificationOnShowdocBind(ctx context.Context) {
	cfg := getNotificationConfig()
	raw, _ := getSystemMeta(ctx, notificationConfigMetaKey)
	if strings.TrimSpace(raw) == "" {
		cfg = defaultNotificationConfig()
	}
	cfg.Enabled = true
	_ = saveNotificationConfig(ctx, cfg)
}

func disableNotificationOnShowdocUnbind(ctx context.Context) {
	cfg := getNotificationConfig()
	cfg.Enabled = false
	_ = saveNotificationConfig(ctx, cfg)
}

func (c NotificationConfig) allows(kind notifyKind) bool {
	if !c.Enabled {
		return false
	}
	switch kind {
	case notifySubmitFailure:
		return c.NotifySubmitFailure
	case notifySubmitTimeout:
		return c.NotifySubmitTimeout
	case notifyDBWriteFailure:
		return c.NotifyDBWriteFailure
	case notifyProcessingTimeout:
		return c.NotifyProcessingTimeout
	default:
		return true
	}
}

func sendNotification(kind notifyKind, title, content string) {
	if strings.TrimSpace(alertShowdocURL) == "" {
		return
	}
	cfg := getNotificationConfig()
	if !cfg.allows(kind) {
		return
	}
	sendShowdocAlert(title, content)
}
