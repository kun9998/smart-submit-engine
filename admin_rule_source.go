package main

import (
	"context"
	"fmt"
	"strings"
)

const platformSourcePHPMetaPrefix = "platform_source_php:"

func platformSourcePHPKey(platformType string) string {
	return platformSourcePHPMetaPrefix + strings.TrimSpace(platformType)
}

func truncateUpstreamForAutoFix(body string) string {
	body = RedactSecrets(strings.TrimSpace(body))
	if body == "" {
		return ""
	}
	const max = 4000
	if len(body) > max {
		return body[:max] + "…"
	}
	return body
}

func savePlatformSourcePHP(ctx context.Context, platformType, php string) error {
	platformType = strings.TrimSpace(platformType)
	php = strings.TrimSpace(php)
	if platformType == "" {
		return nil
	}
	key := platformSourcePHPKey(platformType)
	if php == "" {
		if pluginDB == nil {
			return nil
		}
		q := fmt.Sprintf(`DELETE FROM %s WHERE meta_key=?`, pluginTable("system_meta"))
		_, err := pluginDB.ExecContext(ctx, q, key)
		return err
	}
	if len(php) > 120000 {
		return fmt.Errorf("source_php 过长")
	}
	return setSystemMeta(ctx, key, php)
}

func loadPlatformSourcePHP(ctx context.Context, platformType string) string {
	if pluginDB == nil {
		return ""
	}
	raw, err := getSystemMeta(ctx, platformSourcePHPKey(platformType))
	if err != nil || strings.TrimSpace(raw) == "" {
		return ""
	}
	return raw
}
