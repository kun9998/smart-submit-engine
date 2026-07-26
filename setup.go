package main

import (
	"log"
	"os"
	"strings"
	"sync"
)

var (
	mainDBReady bool
	redisReady  bool

	orderQueueStarted   bool
	orderQueueStartedMu sync.Mutex
)

func isPlaceholderMainDSN(dsn string) bool {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return true
	}
	if strings.HasPrefix(dsn, "user:pass@") {
		return true
	}
	return dsn == "user:pass@tcp(127.0.0.1:3306)/www?parseTime=true"
}

func MainDBReady() bool      { return mainDBReady }
func RedisReady() bool       { return redisReady }
func OrderEngineReady() bool { return mainDBReady && redisReady }

func applyLocalRuntimeConfig() {
	fc, err := loadConfigFile()
	if err != nil {
		return
	}
	applyFileConfigToRuntime(&fc)
}

func adminListenLabel() string {
	addr := normalizeAdminListenAddr(adminAddr)
	if strings.HasPrefix(addr, ":") {
		return "0.0.0.0" + addr
	}
	return addr
}

func resolveAdminStaticDir() string {
	// 发布环境：./web/index.html 为构建产物（/assets/，无 /src/main.ts）
	if isBuiltWebIndex("./" + frontendStaticDir + "/index.html") {
		return frontendStaticDir
	}
	// 开发仓库：npm run build 产物在 web/dist/
	distIndex := frontendStaticDir + "/dist/index.html"
	if _, err := os.Stat("./" + distIndex); err == nil {
		return frontendStaticDir + "/dist"
	}
	return frontendStaticDir
}

func isBuiltWebIndex(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	s := string(b)
	return strings.Contains(s, "/assets/") && !strings.Contains(s, "/src/main.ts")
}

func logSetupHint() {
	if adminEnabled {
		log.Printf("请先访问管理端 /install 完成安装（HTTP 监听 %s，请使用绑定的域名访问）", adminListenLabel())
	}
}
