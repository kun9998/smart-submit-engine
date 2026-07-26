package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const aiConfigMetaKey = "ai_rule_config"

type AIConfig struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

var (
	aiConfig   AIConfig
	aiConfigMu sync.RWMutex
)

func defaultAIConfig() AIConfig {
	return AIConfig{
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-4o-mini",
	}
}

func normalizeAIConfig(c AIConfig) AIConfig {
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if c.BaseURL == "" {
		c.BaseURL = defaultAIConfig().BaseURL
	}
	c.Model = strings.TrimSpace(c.Model)
	if c.Model == "" {
		c.Model = defaultAIConfig().Model
	}
	c.Model = normalizeDeepSeekModel(c.Model)
	c.APIKey = strings.TrimSpace(c.APIKey)
	return c
}

// normalizeDeepSeekModel 将已弃用的 deepseek-chat / deepseek-reasoner 映射到 V4。
func normalizeDeepSeekModel(model string) string {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "deepseek-chat", "deepseek-reasoner":
		return "deepseek-v4-flash"
	default:
		return strings.TrimSpace(model)
	}
}

func getAIConfig() AIConfig {
	aiConfigMu.RLock()
	defer aiConfigMu.RUnlock()
	return aiConfig
}

func loadAIConfigFromPluginDB(ctx context.Context) {
	if pluginDB == nil {
		return
	}
	raw, err := getSystemMeta(ctx, aiConfigMetaKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		if migrated := migrateAIConfigFromFile(ctx); migrated {
			return
		}
		aiConfigMu.Lock()
		aiConfig = defaultAIConfig()
		aiConfigMu.Unlock()
		return
	}
	var cfg AIConfig
	if json.Unmarshal([]byte(raw), &cfg) != nil {
		cfg = defaultAIConfig()
	}
	cfg = normalizeAIConfig(cfg)
	aiConfigMu.Lock()
	aiConfig = cfg
	aiConfigMu.Unlock()
}

func migrateAIConfigFromFile(ctx context.Context) bool {
	cfg, ok := readLegacyAIConfigFromFile()
	if !ok {
		return false
	}
	if err := saveAIConfig(ctx, cfg); err != nil {
		return false
	}
	return true
}

func readLegacyAIConfigFromFile() (AIConfig, bool) {
	b, err := os.ReadFile(configFilePath())
	if err != nil {
		return AIConfig{}, false
	}
	var legacy struct {
		AI AIConfig `yaml:"ai"`
	}
	if err := yaml.Unmarshal(b, &legacy); err != nil {
		return AIConfig{}, false
	}
	if !legacy.AI.Enabled && strings.TrimSpace(legacy.AI.APIKey) == "" {
		return AIConfig{}, false
	}
	return normalizeAIConfig(legacy.AI), true
}

func saveAIConfig(ctx context.Context, cfg AIConfig) error {
	if pluginDB == nil {
		return fmt.Errorf("插件数据库未就绪")
	}
	cfg = normalizeAIConfig(cfg)
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := setSystemMeta(ctx, aiConfigMetaKey, string(b)); err != nil {
		return err
	}
	aiConfigMu.Lock()
	aiConfig = cfg
	aiConfigMu.Unlock()
	return nil
}

func aiConfigReady() bool {
	cfg := getAIConfig()
	return cfg.Enabled &&
		strings.TrimSpace(cfg.APIKey) != "" &&
		strings.TrimSpace(cfg.BaseURL) != ""
}

// aiChatCompletionsURL 兼容 OpenAI（…/v1）与 DeepSeek（https://api.deepseek.com）等根地址。
func aiChatCompletionsURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}

func isDeepSeekBaseURL(baseURL string) bool {
	return strings.Contains(strings.ToLower(baseURL), "deepseek")
}

func isDeepSeekModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "deepseek-")
}

func useDeepSeekRuleConversionOptions(baseURL, model string) bool {
	return isDeepSeekBaseURL(baseURL) || isDeepSeekModel(model)
}

// applyDeepSeekRuleConversionOptions 规则转换用非思考模式 + JSON 输出（替代已弃用的 deepseek-chat）。
func applyDeepSeekRuleConversionOptions(payload map[string]interface{}) {
	payload["response_format"] = map[string]string{"type": "json_object"}
	payload["thinking"] = map[string]string{"type": "disabled"}
	payload["temperature"] = 0.1
}

// applyAIRuleConversionOptions 为 OpenAI 兼容 / DeepSeek 接口设置 JSON 输出参数。
func applyAIRuleConversionOptions(payload map[string]interface{}, baseURL, model string) {
	payload["max_tokens"] = 8192
	if useDeepSeekRuleConversionOptions(baseURL, model) {
		applyDeepSeekRuleConversionOptions(payload)
		return
	}
	payload["temperature"] = 0.1
	payload["response_format"] = map[string]string{"type": "json_object"}
}

func applyAISettingsUpdate(cur AIConfig, req *settingsAIDTO) (AIConfig, error) {
	if req == nil {
		return cur, fmt.Errorf("请求无效")
	}
	out := cur
	out.Enabled = req.Enabled
	if s := strings.TrimSpace(req.BaseURL); s != "" {
		out.BaseURL = strings.TrimRight(s, "/")
	}
	if s := strings.TrimSpace(req.Model); s != "" {
		out.Model = s
	}
	if s := strings.TrimSpace(req.APIKey); s != "" {
		out.APIKey = s
	}
	out = normalizeAIConfig(out)
	return out, nil
}
