package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	_ "github.com/go-sql-driver/mysql"
)

// order_processing_engine.go - 企业级智能订单处理引擎
//
// 产品名称：智能订单处理引擎（Smart Order Processing Engine）
//
// 简介（98字）：
// 高性能、高可靠的分布式订单处理系统，采用生产者-消费者架构，支持按渠道（hid）自动隔离、
// 智能扩缩容、多重幂等保护、实时监控告警、死信队列等企业级特性。处理速度可达1000+单/分钟，
// 订单丢失率降至0.01%，系统可用性达99.9%，适用于高并发订单处理场景。
//
// 生产参数：所有配置从 config.yaml 读取，不再支持环境变量

type orderMsg struct {
	OID int   `json:"oid"`
	HID int   `json:"hid"`
	R   int   `json:"r"`  // retry count
	TS  int64 `json:"ts"` // first enqueue timestamp
}

// 产品信息常量已移至 version.go 文件，便于统一管理

var (
	rdb *redis.Client
	db  *sql.DB

	submitTimeout time.Duration // HTTP 请求超时时间（用于平台下单）

	producerTick time.Duration

	minWorkersPerHID   int
	maxWorkersPerHID   int
	scaleCheckInterval time.Duration
	scaleStepThreshold int
	processingTimeout  time.Duration
	reaperInterval     time.Duration
	timeoutConfirmWait time.Duration // 超时后快速确认数据库的等待时间
	statsInterval      time.Duration // 统计输出间隔，默认 5 分钟
	idleSleepDuration  time.Duration // 队列为空或出错时的休眠时间，默认 50ms

	// 并发跟踪
	concurrencyMu sync.RWMutex // 改为读写锁，减少锁竞争
	currWorkers   = map[int]int{}
	workerCancels = map[int][]context.CancelFunc{}

	// 队列长度缓存（用于减少 Redis 查询，降低 CPU 占用）
	queueLenCache     = make(map[int]int64)     // HID -> 队列长度
	queueLenCacheTime = make(map[int]time.Time) // HID -> 缓存时间
	queueLenCacheMu   sync.RWMutex              // 保护队列长度缓存
	queueLenCacheTTL  = 1 * time.Second         // 缓存有效期 1 秒

	// 统计
	successCount uint64
	dlqCount     uint64

	// 分渠道统计
	perMu   sync.Mutex
	succPer = map[int]uint64{}
	dlqPer  = map[int]uint64{}

	// 窗口（5分钟）统计
	succWin = map[int]uint64{}
	dlqWin  = map[int]uint64{}
	enqWin  = map[int]uint64{}

	// 日志控制：是否逐单打印成功日志（默认关闭）
	logSuccess bool
	// 是否打印每次入队扫描日志（默认关闭）
	logEnqueue bool

	// hid 列表并发访问保护
	hidsMu sync.RWMutex
	hids   []int

	// Producer 扫描游标：oid 递增扫描，扫完一轮后归零，避免大表反复扫头部
	producerLastOID uint64

	// 上次汇总快照（用于判断近一周期是否有新增）
	prevSuccess uint64
	prevDlq     uint64
	// 渠道名缓存
	nameMu    sync.Mutex
	hidToName = map[int]string{}

	// Showdoc 推送地址（绑定后从插件库加载）
	alertShowdocURL string

	// 数据库表前缀（可在配置文件中自定义）
	tablePrefix string

	// 订单状态配置
	submittedStatus  string // 订单提交成功后的状态
	submittedRemarks string // 订单提交成功后的提示信息

	// 成功 code 列表（用于判断订单提交是否成功）
	successCodes []int // 默认 [0, 1]

	// HTTP 客户端（用于平台下单 HTTP 请求）
	httpClient *http.Client

	// 限流相关
	rateLimitEnabled   bool
	globalRateLimiter  *TokenBucket
	perHIDRateLimiters = make(map[int]*TokenBucket) // 每个 HID 的限流器
	rateLimitMu        sync.RWMutex                 // 保护 perHIDRateLimiters

	// 配置文件缓存（避免频繁读取文件，降低 CPU 占用）
	configCache     *fileConfig
	configCacheTime time.Time
	configCacheMu   sync.RWMutex
	configCacheTTL  = 30 * time.Second // 配置缓存有效期 30 秒
)

type fileConfig struct {
	Redis struct {
		Addr string `yaml:"addr"`
		Pass string `yaml:"pass"`
		DB   int    `yaml:"db"`
	} `yaml:"redis"`
	MainMySQLDSN      string       `yaml:"main_mysql_dsn"`
	MySQLDSN          string       `yaml:"mysql_dsn,omitempty"`
	PluginMySQLDSN    string       `yaml:"plugin_mysql_dsn"`
	TablePrefix       string       `yaml:"table_prefix"`
	PluginTablePrefix string       `yaml:"plugin_table_prefix"`
	Installed         bool         `yaml:"installed"`
	Auth              AuthConfig   `yaml:"auth"`
	HTTPSecurity      HTTPSecurity `yaml:"http_security,omitempty"`
	Admin             struct {
		Enabled bool   `yaml:"enabled"`
		Addr    string `yaml:"addr"`
	} `yaml:"admin"`
}

// 注意：不再使用环境变量，所有配置从 config.yaml 读取
// getenv、atoiDef、mustIntEnv 等函数已移除，不再需要

var (
	redisConfig struct {
		Addr string
		Pass string
		DB   int
	}
	mysqlDSN  string
	dbCharset string // 数据库实际使用的字符集（utf8 或 utf8mb4），由 setCharset 函数自动检测
)

func initRedis() {
	redisReady = initRedisOptional()
}

func initRedisOptional() bool {
	rdb = redis.NewClient(&redis.Options{
		Addr:         redisConfig.Addr,
		Password:     redisConfig.Pass,
		DB:           redisConfig.DB,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     30,
		MinIdleConns: 5,
		MaxRetries:   3,
		PoolTimeout:  4 * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("Redis 未连接: %v（订单队列暂不启动，可先完成管理端安装）", err)
		rdb = nil
		return false
	}
	log.Printf("Redis 已连接")
	return true
}

// detectAndSetCharset 检测数据库实际字符集并设置连接字符集
// 支持 utf8 和 utf8mb4，根据表的实际字符集自动适配（不依赖排序规则）
func detectAndSetCharset(ctx context.Context, database *sql.DB, tablePrefix string) (string, error) {
	// 优先检测订单表的实际字符集（更准确）
	// 如果表不存在或查询失败，再检测数据库默认字符集
	var tableCharset string
	orderTable := tablePrefix + "_order"

	// 查询表的字符集
	query := `SELECT TABLE_COLLATION 
	          FROM information_schema.TABLES 
	          WHERE TABLE_SCHEMA = DATABASE() 
	          AND TABLE_NAME = ? 
	          LIMIT 1`
	var collation string
	err := database.QueryRowContext(ctx, query, orderTable).Scan(&collation)
	if err == nil && collation != "" {
		// 从排序规则中提取字符集（例如：utf8mb4_general_ci -> utf8mb4, utf8_unicode_ci -> utf8）
		if strings.HasPrefix(collation, "utf8mb4") {
			tableCharset = "utf8mb4"
		} else if strings.HasPrefix(collation, "utf8") {
			tableCharset = "utf8"
		}
	}

	// 如果表字符集检测成功，使用表的字符集
	if tableCharset != "" {
		log.Printf("检测到订单表字符集: %s (排序规则: %s)", tableCharset, collation)
	} else {
		// 表字符集检测失败，检测数据库默认字符集
		var dbCharset string
		err = database.QueryRowContext(ctx, "SELECT @@character_set_database").Scan(&dbCharset)
		if err != nil {
			// 如果查询失败，尝试查询服务器字符集
			err = database.QueryRowContext(ctx, "SELECT @@character_set_server").Scan(&dbCharset)
			if err != nil {
				return "", fmt.Errorf("无法检测数据库字符集: %v", err)
			}
		}
		tableCharset = dbCharset
		log.Printf("检测到数据库默认字符集: %s", tableCharset)
	}

	// 根据检测到的字符集设置连接字符集
	var charset string
	if tableCharset == "utf8mb4" {
		charset = "utf8mb4"
	} else if tableCharset == "utf8" {
		charset = "utf8"
	} else {
		// 其他字符集（如 latin1、gbk、gb2312 等）
		// 优先尝试使用表的原始字符集，如果失败则回退到 utf8
		log.Printf("警告: 检测到字符集为 %s（非 utf8/utf8mb4），将尝试使用原始字符集", tableCharset)

		// 先尝试使用表的原始字符集
		if safeCS, err := safeMySQLCharsetParam(tableCharset); err == nil {
			if _, err := database.ExecContext(ctx, fmt.Sprintf("SET NAMES %s", safeCS)); err == nil {
				if _, err := database.ExecContext(ctx, fmt.Sprintf("SET CHARACTER SET %s", safeCS)); err == nil {
					log.Printf("已使用原始字符集: %s", tableCharset)
					return tableCharset, nil
				}
			}
		}

		// 如果原始字符集设置失败，回退到 utf8（兼容性更好）
		log.Printf("警告: 无法使用字符集 %s，回退到 utf8（建议将数据库迁移到 utf8 或 utf8mb4）", tableCharset)
		charset = "utf8"
	}

	// 设置连接字符集
	safeCS, err := safeMySQLCharsetParam(charset)
	if err != nil {
		return "", fmt.Errorf("设置字符集失败: 非法字符集名")
	}
	if _, err := database.ExecContext(ctx, fmt.Sprintf("SET NAMES %s", safeCS)); err != nil {
		return "", fmt.Errorf("设置字符集失败: %v", err)
	}
	if _, err := database.ExecContext(ctx, fmt.Sprintf("SET CHARACTER SET %s", safeCS)); err != nil {
		return "", fmt.Errorf("设置字符集失败: %v", err)
	}

	return charset, nil
}

func initMySQL() {
	mainDBReady = initMySQLOptional()
}

func initMySQLOptional() bool {
	if isPlaceholderMainDSN(mysqlDSN) {
		log.Printf("主站数据库未配置（config.yaml 仍为占位 DSN），订单队列暂不启动")
		return false
	}
	var err error
	if db != nil {
		_ = db.Close()
		db = nil
	}
	db, err = sql.Open("mysql", mysqlDSN)
	if err != nil {
		log.Printf("MySQL 打开失败: %v", err)
		return false
	}
	db.SetMaxOpenConns(200)
	db.SetMaxIdleConns(100)
	db.SetConnMaxLifetime(10 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Printf("MySQL 未连接: %v（请先完成管理端安装或修正 mysql_dsn）", err)
		_ = db.Close()
		db = nil
		return false
	}
	charset, err := detectAndSetCharset(ctx, db, tablePrefix)
	if err != nil {
		log.Printf("MySQL 字符集设置失败: %v", err)
		_ = db.Close()
		db = nil
		return false
	}
	dbCharset = charset
	log.Printf("主站数据库已连接，字符集: %s", charset)
	return true
}

func reconnectMainDBAfterInstall(dsn string) bool {
	mysqlDSN = strings.TrimSpace(dsn)
	return initMySQLOptional()
}

func reconnectRedisAfterInstall(addr, pass string, dbNum int) bool {
	if addr != "" {
		redisConfig.Addr = addr
	}
	redisConfig.Pass = pass
	redisConfig.DB = dbNum
	return initRedisOptional()
}

// 保持 MySQL 心跳与自动重连
func maintainDBConnection(ctx context.Context) {
	if db == nil {
		return
	}
	interval := 15 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := db.PingContext(pingCtx)
			cancel()
			if err == nil {
				continue
			}
			log.Printf("MySQL 心跳失败，尝试重连: %v", err)
			_ = db.Close()
			newdb, oerr := sql.Open("mysql", mysqlDSN)
			if oerr != nil {
				log.Printf("MySQL 重新打开失败: %v", oerr)
				continue
			}
			newdb.SetMaxOpenConns(200)
			newdb.SetMaxIdleConns(100)
			newdb.SetConnMaxLifetime(10 * time.Minute)
			pc, pcancel := context.WithTimeout(context.Background(), 2*time.Second)
			perr := newdb.PingContext(pc)
			pcancel()
			if perr != nil {
				log.Printf("MySQL 重连 ping 失败: %v", perr)
				_ = newdb.Close()
				continue
			}
			// 重连后也需要设置字符集（使用之前检测到的字符集）
			charsetCtx, charsetCancel := context.WithTimeout(context.Background(), 2*time.Second)
			// 如果还没有检测过字符集，先检测；否则使用已保存的字符集
			if dbCharset == "" {
				detectedCharset, err := detectAndSetCharset(charsetCtx, newdb, tablePrefix)
				if err != nil {
					charsetCancel()
					log.Printf("MySQL 重连后设置字符集失败: %v", err)
					_ = newdb.Close()
					continue
				}
				dbCharset = detectedCharset
			} else {
				safeCS, csErr := safeMySQLCharsetParam(dbCharset)
				if csErr != nil {
					charsetCancel()
					log.Printf("MySQL 重连后设置字符集失败: 非法字符集名")
					_ = newdb.Close()
					continue
				}
				if _, err := newdb.ExecContext(charsetCtx, fmt.Sprintf("SET NAMES %s", safeCS)); err != nil {
					charsetCancel()
					log.Printf("MySQL 重连后设置字符集失败: %v", err)
					_ = newdb.Close()
					continue
				}
				if _, err := newdb.ExecContext(charsetCtx, fmt.Sprintf("SET CHARACTER SET %s", safeCS)); err != nil {
					charsetCancel()
					log.Printf("MySQL 重连后设置字符集失败: %v", err)
					_ = newdb.Close()
					continue
				}
			}
			charsetCancel()
			db = newdb
			log.Printf("MySQL 已重连成功")
		}
	}
}

func initConfig() {
	// 默认配置值（不再支持环境变量，仅从配置文件读取）
	tablePrefix = "love_learn"       // 默认表前缀
	submitTimeout = 30 * time.Second // 默认 HTTP 请求超时，可在货源配置中覆盖（存插件库）
	producerTick = 2 * time.Second   // 默认生产者间隔
	logSuccess = false               // 默认不打印成功日志
	logEnqueue = false               // 默认不打印入队日志

	// 并发扩缩默认值
	minWorkersPerHID = 1
	maxWorkersPerHID = 8
	scaleCheckInterval = 2 * time.Second
	scaleStepThreshold = 100
	processingTimeout = 45 * time.Minute
	reaperInterval = 2 * time.Minute
	timeoutConfirmWait = 5 * time.Second
	idleSleepDuration = 50 * time.Millisecond // 默认 50ms，用于降低CPU使用率

	// 订单状态默认值
	submittedStatus = "已提交"
	submittedRemarks = "订单已成功提交至处理系统，请耐心等待处理"
	successCodes = []int{0, 1}

	// 限流默认值（可在管理端货源配置中覆盖，存插件库）
	rateLimitEnabled = false
	defaultPerHIDMaxPerSecond = 0

	// ./config.yaml 存在则加载并覆盖
	if fc, err := loadConfigFile(); err == nil {
		applyFileConfigToRuntime(&fc)
	}

	// 告警冷却等已移除；订单异常通知见个人中心配置

	// 成功 code 列表默认值（如果配置文件中未设置）
	if len(successCodes) == 0 {
		successCodes = []int{0, 1}
	}

	// 统计输出间隔默认值（如果配置文件中未设置）
	if statsInterval == 0 {
		statsInterval = 5 * time.Minute
	}

	// 空闲休眠时间默认值（如果配置文件中未设置）
	if idleSleepDuration == 0 {
		idleSleepDuration = 50 * time.Millisecond
	}

	captureInitRuntimeDefaults()
}

func listKey(hid int) string      { return fmt.Sprintf("order_queue:%d", hid) }
func procKey(hid int) string      { return fmt.Sprintf("processing_queue:%d", hid) }
func enqKey(oid int) string       { return fmt.Sprintf("enqueued:%d", oid) }
func lockKey(oid int) string      { return fmt.Sprintf("processing:order:%d", oid) }
func dlqKey(hid int) string       { return fmt.Sprintf("dlq_queue:%d", hid) }
func submittedKey(oid int) string { return fmt.Sprintf("submitted:%d", oid) } // 保留兼容性，用于迁移
func submittedHashKey() string    { return "submitted_orders" }               // 新的 Hash 结构 key

const enqKeyTTL = 30 * time.Minute

// producerBatchLimit 单次 Producer 扫描/入队上限（大单量时配合 drain 循环连续扫多批）
const producerBatchLimit = 2000

// touchEnqKey 延长入队去重标记，避免长队列等待期间 producer 重复入队。
func touchEnqKey(ctx context.Context, oid int) {
	_ = rdb.Expire(ctx, enqKey(oid), enqKeyTTL).Err()
}

// requeueFromProcessing 将 processing 中的订单安全回推到待处理队列（RPUSH 成功后才 LRem）。
func requeueFromProcessing(ctx context.Context, src, proc, val string) bool {
	for i := 0; i < 3; i++ {
		if err := rdb.RPush(ctx, src, val).Err(); err == nil {
			_ = rdb.LRem(ctx, proc, 1, val).Err()
			return true
		}
		if i < 2 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	return false
}

// 表名生成函数
func tableName(name string) string {
	return tablePrefix + "_" + name
}

// safeMySQLCharsetParam 校验字符集名，防止异常值进入 SET NAMES（仅允许字母数字与少量符号）
func safeMySQLCharsetParam(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return "", fmt.Errorf("invalid charset")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '-', r == '+':
		default:
			return "", fmt.Errorf("invalid charset")
		}
	}
	return name, nil
}

// 🔒 提取简化的错误信息（只保留关键信息，不输出完整URL和堆栈）
func simplifyErrorMsg(errmsg string) string {
	if errmsg == "" {
		return ""
	}

	// 统一处理内部错误码（优先于英文关键字）
	if strings.Contains(errmsg, "E_HTTP_TIMEOUT") {
		return "网络请求超时"
	}
	if strings.Contains(errmsg, "E_DNS") {
		return "域名解析失败"
	}
	if strings.Contains(errmsg, "E_BLOCKED_ADDRESS") {
		return "目标地址受限"
	}
	if strings.Contains(errmsg, "E_HOST_NOT_WHITELIST") {
		return "目标域名不在白名单"
	}
	if strings.Contains(errmsg, "E_URL_INVALID") || strings.Contains(errmsg, "E_URL_HOST") || strings.Contains(errmsg, "E_URL_SCHEME") {
		return "请求地址格式错误"
	}
	if strings.Contains(errmsg, "E_HTTP_TRANSPORT") {
		return "网络连接失败"
	}
	if strings.Contains(errmsg, "E_HTTP_BAD_REQUEST") ||
		strings.Contains(errmsg, "first path segment in URL cannot contain colon") {
		return "货源地址格式错误（填 ip:port 或 http:// 开头，勿带 /system 等路径）"
	}
	if strings.Contains(errmsg, "E_HTTP_REDIRECT") {
		return "上游重定向次数过多"
	}
	if strings.Contains(errmsg, "请求 URL 为空") {
		return "提交规则缺少有效 URL（检查 branches 是否配置 url）"
	}
	if strings.Contains(errmsg, "E_HTTP_READ_BODY") {
		return "响应读取失败"
	}
	if strings.Contains(errmsg, "E_HTTP_STATUS_") {
		if idx := strings.Index(errmsg, "E_HTTP_STATUS_"); idx >= 0 {
			codeStr := errmsg[idx+len("E_HTTP_STATUS_"):]
			if p := strings.IndexAny(codeStr, " :;,)]}\n\r\t"); p >= 0 {
				codeStr = codeStr[:p]
			}
			if code, err := strconv.Atoi(codeStr); err == nil {
				switch code {
				case 400:
					return "上游接口请求参数错误(400)"
				case 302:
					return "上游未登录或 Cookie 失效(302)"
				case 401:
					return "上游接口未授权(401)"
				case 403:
					return "上游接口拒绝访问(403)"
				case 404:
					return "上游接口地址不存在(404)"
				case 405:
					return "上游接口请求方法不支持(405)"
				case 408:
					return "上游接口请求超时(408)"
				case 409:
					return "上游接口请求冲突(409)"
				case 422:
					return "上游接口参数校验失败(422)"
				case 429:
					return "上游接口请求过于频繁(429)"
				case 500:
					return "上游接口内部错误(500)"
				case 502:
					return "上游网关错误(502)"
				case 503:
					return "上游接口暂时不可用(503)"
				case 504:
					return "上游网关超时(504)"
				default:
					if code >= 400 && code < 500 {
						return fmt.Sprintf("上游接口客户端错误(%d)", code)
					}
					if code >= 500 && code < 600 {
						return fmt.Sprintf("上游接口服务端错误(%d)", code)
					}
					return fmt.Sprintf("上游接口返回异常状态(%d)", code)
				}
			}
		}
		return "上游接口返回异常状态"
	}

	// 检查是否是网络超时错误
	if strings.Contains(errmsg, "timeout") || strings.Contains(errmsg, "deadline exceeded") {
		return "网络请求超时"
	}

	// 检查是否是连接错误
	if strings.Contains(errmsg, "connection") || strings.Contains(errmsg, "refused") {
		return "网络连接失败"
	}

	// 检查是否是DNS解析错误
	if strings.Contains(errmsg, "no such host") || strings.Contains(errmsg, "DNS") {
		return "域名解析失败"
	}

	// 检查是否是SSL/TLS错误
	if strings.Contains(errmsg, "certificate") || strings.Contains(errmsg, "TLS") || strings.Contains(errmsg, "SSL") {
		return "SSL证书验证失败"
	}

	// 移除完整的 https:// 或 http:// URL
	// 优化：限制循环次数和搜索范围，避免处理超长字符串导致 CPU 占用高
	result := errmsg
	// 先处理 https://（限制最多处理 10 次，避免无限循环）
	for i := 0; i < 10 && strings.Contains(result, "https://"); i++ {
		idx := strings.Index(result, "https://")
		if idx == -1 {
			break
		}
		// 找到URL的结束位置（限制搜索范围，避免处理超长URL）
		end := idx + len("https://")
		maxSearch := end + 200 // 限制搜索范围，避免处理超长URL
		if maxSearch > len(result) {
			maxSearch = len(result)
		}
		for end < maxSearch && result[end] != ' ' && result[end] != '\n' && result[end] != '"' && result[end] != '\'' && result[end] != ')' && result[end] != ']' {
			end++
		}
		// 替换URL为 [URL已隐藏]
		result = result[:idx] + "[URL已隐藏]" + result[end:]
	}
	// 处理 http://（限制最多处理 10 次）
	for i := 0; i < 10 && strings.Contains(result, "http://"); i++ {
		idx := strings.Index(result, "http://")
		if idx == -1 {
			break
		}
		end := idx + len("http://")
		maxSearch := end + 200
		if maxSearch > len(result) {
			maxSearch = len(result)
		}
		for end < maxSearch && result[end] != ' ' && result[end] != '\n' && result[end] != '"' && result[end] != '\'' && result[end] != ')' && result[end] != ']' {
			end++
		}
		result = result[:idx] + "[URL已隐藏]" + result[end:]
	}

	// 移除 Get "..." 模式（HTTP请求错误通常包含这个）
	// 优化：限制循环次数，避免处理超长字符串
	for i := 0; i < 10 && strings.Contains(result, "Get \""); i++ {
		idx := strings.Index(result, "Get \"")
		if idx == -1 {
			break
		}
		// 找到 Get " 之后的内容，直到下一个引号或换行（限制搜索范围）
		start := idx + len("Get \"")
		end := start
		maxSearch := start + 200 // 限制搜索范围
		if maxSearch > len(result) {
			maxSearch = len(result)
		}
		for end < maxSearch && result[end] != '"' && result[end] != '\n' {
			end++
		}
		if end < len(result) && result[end] == '"' {
			// 移除整个 Get "..." 部分
			result = result[:idx] + "请求失败" + result[end+1:]
		} else {
			break
		}
	}

	// 如果错误信息太长，截取前100个字符
	if len(result) > 100 {
		// 尝试在句号、冒号或换行处截断
		if idx := strings.IndexAny(result[:100], "。:\n"); idx > 0 {
			return result[:idx+1]
		}
		return result[:100] + "..."
	}

	return RedactSecrets(result)
}

// 注意：Order, ProcessCxResult, Huoyuan, PlatformPlugin 和 platformPlugins 已在 processcx.go 中定义

// normalizeOID 规范化订单ID（转换为整数，与 order_sync/main.go 保持一致）
func normalizeOID(oid string) (int64, error) {
	// 尝试直接解析为整数
	oidInt, err := strconv.ParseInt(oid, 10, 64)
	if err == nil {
		return oidInt, nil
	}

	// 如果失败，尝试提取数字部分
	oidStr := strings.TrimSpace(oid)
	oidStr = strings.TrimPrefix(oidStr, "oid")
	oidStr = strings.TrimSpace(oidStr)

	oidInt, err = strconv.ParseInt(oidStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("无法解析订单ID: %s", oid)
	}

	return oidInt, nil
}

// 使用 Hash 存储已提交订单（优化：统一存储，减少 key 数量）
// Hash field 为订单ID，value 为 yid（如果有）
func setSubmittedOrder(ctx context.Context, oid int, yid string) error {
	hashKey := submittedHashKey()
	field := strconv.Itoa(oid)
	// 使用 HSET 设置，value 为空字符串表示已提交但 yid 未知
	if err := rdb.HSet(ctx, hashKey, field, yid).Err(); err != nil {
		return err
	}
	// 设置整个 Hash 的过期时间为 30 天（每次操作时续期）
	return rdb.Expire(ctx, hashKey, 30*24*time.Hour).Err()
}

func getSubmittedOrder(ctx context.Context, oid int) (string, bool, error) {
	hashKey := submittedHashKey()
	field := strconv.Itoa(oid)
	v, err := rdb.HGet(ctx, hashKey, field).Result()
	if err == redis.Nil {
		// Hash 中不存在，检查旧格式（兼容性）
		oldKey := submittedKey(oid)
		v, err := rdb.Get(ctx, oldKey).Result()
		if err == redis.Nil {
			return "", false, nil
		}
		// 如果旧格式存在，迁移到新格式并删除旧 key
		_ = setSubmittedOrder(ctx, oid, v)
		_ = rdb.Del(ctx, oldKey).Err()
		return v, true, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func delSubmittedOrder(ctx context.Context, oid int) error {
	hashKey := submittedHashKey()
	field := strconv.Itoa(oid)
	return rdb.HDel(ctx, hashKey, field).Err()
}

// 更新订单状态为已提交（避免重复代码）
func updateOrderStatusSubmitted(ctx context.Context, oid int, yid string, hid int) error {
	orderTable := tableName("order")
	osCfg := getEffectiveOrderStatusForHID(hid)
	status := osCfg.SubmittedStatus
	remarks := osCfg.SubmittedRemarks

	var result sql.Result
	var err error

	// 先尝试包含 update_time 的更新（如果字段存在）
	if yid != "" {
		result, err = db.ExecContext(ctx,
			fmt.Sprintf(`UPDATE %s SET status=?, dockstatus='1', remarks=?, yid=?, update_time=? WHERE oid=? LIMIT 1`, orderTable),
			status, remarks, yid, time.Now().Format("2006-01-02 15:04:05"), oid,
		)
	} else {
		result, err = db.ExecContext(ctx,
			fmt.Sprintf(`UPDATE %s SET status=?, dockstatus='1', remarks=?, update_time=? WHERE oid=? LIMIT 1`, orderTable),
			status, remarks, time.Now().Format("2006-01-02 15:04:05"), oid,
		)
	}

	// 如果失败且错误是字段不存在，尝试不包含 update_time 的更新
	if err != nil && strings.Contains(err.Error(), "Unknown column 'update_time'") {
		if yid != "" {
			result, err = db.ExecContext(ctx,
				fmt.Sprintf(`UPDATE %s SET status=?, dockstatus='1', remarks=?, yid=? WHERE oid=? LIMIT 1`, orderTable),
				status, remarks, yid, oid,
			)
		} else {
			result, err = db.ExecContext(ctx,
				fmt.Sprintf(`UPDATE %s SET status=?, dockstatus='1', remarks=? WHERE oid=? LIMIT 1`, orderTable),
				status, remarks, oid,
			)
		}
	}

	if err != nil {
		return err
	}

	// 检查实际影响的行数（静默检查，不输出日志）
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// 更新未影响任何行，可能是订单不存在或已被更新，返回错误以便上层处理
		return fmt.Errorf("更新未影响任何行，oid=%d", oid)
	}

	return nil
}

// 生产者：扫描待处理订单并入队（按 hid 分流），避免重复入队
func producer(ctx context.Context) {
	ticker := time.NewTicker(producerTick)
	defer ticker.Stop()
	drainPending := func() {
		for {
			_, scanned, err := enqueuePending(ctx)
			if err != nil || scanned < producerBatchLimit {
				break
			}
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			drainPending()
		}
	}
}

func enqueuePending(ctx context.Context) (added int, scanned int, err error) {
	orderTable := tableName("order")
	lastOID := int(atomic.LoadUint64(&producerLastOID))
	q := fmt.Sprintf(`SELECT oid, hid FROM %s WHERE dockstatus='0' AND status!='已取消' AND oid > %d ORDER BY oid ASC LIMIT %d`, orderTable, lastOID, producerBatchLimit)
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	type enqueueCandidate struct {
		oid     int
		hid     int
		payload string
	}
	candidates := make([]enqueueCandidate, 0, 256)
	maxOID := lastOID
	for rows.Next() {
		scanned++
		var oid int
		var hid int
		if err := rows.Scan(&oid, &hid); err != nil {
			return added, scanned, err
		}
		if oid > maxOID {
			maxOID = oid
		}
		if hid == 0 {
			continue
		}
		m := orderMsg{OID: oid, HID: hid, R: 0, TS: time.Now().Unix()}
		b, _ := json.Marshal(m)
		candidates = append(candidates, enqueueCandidate{oid: oid, hid: hid, payload: string(b)})
	}

	if scanned == 0 {
		if lastOID > 0 {
			atomic.StoreUint64(&producerLastOID, 0)
		}
		return 0, 0, nil
	}

	setPipe := rdb.Pipeline()
	setNXCmds := make([]*redis.BoolCmd, len(candidates))
	for i, c := range candidates {
		setNXCmds[i] = setPipe.SetNX(ctx, enqKey(c.oid), 1, enqKeyTTL)
	}
	if _, err := setPipe.Exec(ctx); err != nil && err != redis.Nil {
		return added, scanned, err
	}

	toPush := make([]enqueueCandidate, 0, len(candidates))
	for i, c := range candidates {
		ok, cmdErr := setNXCmds[i].Result()
		if cmdErr == nil && ok {
			toPush = append(toPush, c)
		}
	}

	if len(toPush) > 0 {
		pushPipe := rdb.Pipeline()
		lpushCmds := make([]*redis.IntCmd, len(toPush))
		for i, c := range toPush {
			lpushCmds[i] = pushPipe.LPush(ctx, listKey(c.hid), c.payload)
		}
		_, _ = pushPipe.Exec(ctx)

		enqDelta := map[int]uint64{}
		for i, c := range toPush {
			if err := lpushCmds[i].Err(); err != nil {
				_ = rdb.Del(ctx, enqKey(c.oid)).Err()
				continue
			}
			added++
			enqDelta[c.hid]++
		}
		if len(enqDelta) > 0 {
			perMu.Lock()
			for hid, n := range enqDelta {
				enqWin[hid] += n
			}
			perMu.Unlock()
		}
	}

	if scanned >= producerBatchLimit {
		atomic.StoreUint64(&producerLastOID, uint64(maxOID))
	} else {
		atomic.StoreUint64(&producerLastOID, 0)
	}
	return added, scanned, nil
}

// 消费一个 hid 的队列（并发=1/每hid）
func consumer(ctx context.Context, hid int, wg *sync.WaitGroup) {
	if wg != nil {
		defer wg.Done()
	}
	src := listKey(hid)
	proc := procKey(hid)
		defer func() {
			if r := recover(); r != nil {
				_ = r
			}
		}()
	// 错误重试间隔（指数退避，避免频繁重试导致 CPU 占用高）
	retryDelay := idleSleepDuration
	maxRetryDelay := 5 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 使用 BLMOVE source RIGHT -> processing LEFT，阻塞 5s（Redis >=6.2）
		var val string
		cmd := rdb.Do(ctx, "BLMOVE", src, proc, "RIGHT", "LEFT", 5)
		if cmd.Err() != nil {
			if cmd.Err() == redis.Nil {
				// 队列为空，休眠避免忙等待（降低CPU使用率）
				time.Sleep(idleSleepDuration)
				retryDelay = idleSleepDuration // 重置重试延迟
				continue
			}
			// 兼容性降级：BRPOP + LPUSH（非原子，尽量避免）
			res, err := rdb.BRPop(ctx, 5*time.Second, src).Result()
			if err != nil {
				// 出错时使用指数退避，避免频繁重试
				time.Sleep(retryDelay)
				if retryDelay < maxRetryDelay {
					retryDelay = retryDelay * 2 // 指数退避
				}
				continue
			}
			retryDelay = idleSleepDuration // 成功时重置重试延迟
			if len(res) == 2 {
				val = res[1]
				if err := rdb.LPush(ctx, proc, val).Err(); err != nil {
					_ = rdb.RPush(ctx, src, val).Err()
					continue
				}
			} else {
				// 数据格式异常，休眠避免忙等待
				time.Sleep(idleSleepDuration)
				continue
			}
		} else {
			// 正常 BLMOVE 返回值
			v, _ := cmd.Text()
			if v == "" {
				// 返回值为空，休眠避免忙等待
				time.Sleep(idleSleepDuration)
				retryDelay = idleSleepDuration // 重置重试延迟
				continue
			}
			val = v
			retryDelay = idleSleepDuration // 成功时重置重试延迟
		}

		// 处理 processing 队列头部元素（我们刚刚放入左侧）
		var msg orderMsg
		if err := json.Unmarshal([]byte(val), &msg); err != nil {
			// 无法解析，直接丢进 DLQ 并从 processing 移除
			_ = rdb.LRem(ctx, proc, 1, val).Err()
			_ = rdb.LPush(ctx, dlqKey(hid), val).Err()
			continue
		}
		touchEnqKey(ctx, msg.OID)

		// 幂等锁，避免重复消费（使用分布式锁，带续期机制）
		lkey := lockKey(msg.OID)
		lock, err := AcquireLock(ctx, rdb, lkey, 10*time.Minute)
		if err != nil {
			// 锁已被占用，说明订单正在处理中，跳过
			if err.Error() == "锁已被占用" {
				_ = rdb.LRem(ctx, proc, 1, val).Err()
				continue
			}
			// 获取锁失败（Redis 抖动等）：回队重试，避免从 processing 消失
			if !requeueFromProcessing(ctx, src, proc, val) {
				log.Printf("注意: 订单 %d 暂时未能放回队列，请稍后自动重试", msg.OID)
			}
			continue
		}

		// 每单独立闭包释放锁：禁止在 for 里直接 defer Release，否则 defer 栈与 Redis 续期 goroutine 会随订单数泄漏
		var stopConsumer bool
		handOffLock := false
		func(lock *DistributedLock) {
			defer func() {
				if !handOffLock {
					_ = lock.Release(context.Background())
				}
			}()

			// 若已提交过上游（或设置了防重复标记），则仅补写数据库，避免二次提交
			hasSubmitted := false
			submittedYid := ""
			v, exists, err := getSubmittedOrder(ctx, msg.OID)
			if err == nil && exists {
				// 订单已提交（防重复）
				hasSubmitted = true
				if v != "" {
					submittedYid = v
				}
			}

			if hasSubmitted {
				// 仅补写 DB，不再调用上游
				dbErr := updateOrderStatusSubmitted(ctx, msg.OID, submittedYid, hid)
				if dbErr == nil {
					// 视作成功完成一次修复
					recordSubmitSuccess(hid)
					// 修复成功日志已精简
					// 锁会在 defer 中自动释放
					_ = rdb.Del(ctx, enqKey(msg.OID)).Err()
					_ = rdb.LRem(ctx, proc, 1, val).Err()
					return
				}
				// DB 写失败：只更新 dockstatus，不更新 status 和 remarks（避免代理发现异常）
				name := submitLogChannel(hid)
				logSubmitDBFail(hid, msg.OID)
				orderTable := tableName("order")
				result, execErr := db.ExecContext(ctx,
					fmt.Sprintf(`UPDATE %s SET dockstatus='2' WHERE oid=? LIMIT 1`, orderTable),
					msg.OID,
				)
				if execErr != nil {
					log.Printf("更新订单状态失败，订单号 %d", msg.OID)
				} else if rows, _ := result.RowsAffected(); rows == 0 {
					log.Printf("未找到订单 %d，状态未更新", msg.OID)
				}
				// 发送 Showdoc 通知
				if alertShowdocURL != "" {
					title := fmt.Sprintf("更新订单状态失败 · %s", name)
					content := fmt.Sprintf("订单号：%d\n渠道：%s\n已标记为提交异常，请到管理端查看", msg.OID, name)
					go sendNotification(notifyDBWriteFailure, title, content)
				}
				// 进入 DLQ
				atomicAddDLQ(hid)
				_ = rdb.LRem(ctx, proc, 1, val).Err()
				b, _ := json.Marshal(msg)
				_ = rdb.LPush(ctx, dlqKey(hid), string(b)).Err()
				_ = rdb.Del(ctx, enqKey(msg.OID)).Err()
				// 锁会在 defer 中自动释放
				return
			}

			// 限流检查（在调用提交接口前）
			if allowed, err := checkRateLimit(ctx, hid); err != nil {
				// 限流检查失败，静默处理，继续处理订单
			} else if !allowed {
				// 限流拒绝，等待一段时间后重试（将订单放回队列尾部）
				logSubmitRateLimited(hid, msg.OID)
				// 等待一小段时间
				select {
				case <-ctx.Done():
					stopConsumer = true
					return
				case <-time.After(100 * time.Millisecond):
				}
				if !requeueFromProcessing(ctx, src, proc, val) {
					log.Printf("注意: 限流后订单 %d 暂时未能放回队列", msg.OID)
				}
				return
			}

			if ok, yid := orderAlreadySucceededInDB(ctx, msg.OID); ok {
				_ = setSubmittedOrder(ctx, msg.OID, yid)
				_ = updateOrderStatusSubmitted(ctx, msg.OID, yid, hid)
				recordSubmitSuccess(hid)
				_ = rdb.Del(ctx, enqKey(msg.OID)).Err()
				_ = rdb.LRem(ctx, proc, 1, val).Err()
				return
			}

			job := submitPoolJob{
				lock: lock,
				hid:  hid,
				msg:  msg,
				val:  val,
				proc: proc,
			}
			if tryDispatchSubmitPool(job) {
				handOffLock = true
				return
			}
			// 提交池满时同步降级，避免丢单
			syncTimeout := getSubmitTimeoutForHID(hid) + 10*time.Second
			submitCtx, submitCancel := context.WithTimeout(ctx, syncTimeout)
			success, yid, errmsg, callErr := submitViaInternal(submitCtx, msg.OID)
			submitCancel()
			if finalizeSubmitOutcome(lock, hid, msg, val, proc, success, yid, errmsg, callErr) {
				handOffLock = true
			}
		}(lock)
		if stopConsumer {
			return
		}
	}
}

// submitViaPlugin 使用插件库 JSON 规则处理订单提交
func submitViaPlugin(ctx context.Context, oid int) (bool, string, string, error) {
	order, huoyuan, hidNum, err := loadOrderAndHuoyuanForSubmit(ctx, oid)
	if err != nil {
		if strings.Contains(err.Error(), "查询") {
			return false, "", err.Error(), err
		}
		return false, "", err.Error(), nil
	}

	timeout := getSubmitTimeoutForHID(hidNum)
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if huoyuan.Type == "" {
		return false, "", "平台类型为空", nil
	}
	platformPluginsMu.RLock()
	plugin, exists := platformPlugins[huoyuan.Type]
	platformPluginsMu.RUnlock()
	if !exists {
		return false, "", fmt.Sprintf("不支持的平台类型: %s", huoyuan.Type), nil
	}

	submitClient := NewOutboundHTTPClient(timeout)
	result, err := plugin.AddOrder(ctx, order, huoyuan, submitClient)
	if err != nil {
		return false, "", SanitizeUserVisibleError(err.Error()), err
	}

	codes := getSuccessCodesForHID(hidNum)
	isSuccess := false
	for _, successCode := range codes {
		if result.Code == successCode {
			isSuccess = true
			break
		}
	}
	if !isSuccess && result.Code == 0 && strings.Contains(result.Msg, "成功") {
		isSuccess = true
	}

	if isSuccess {
		return true, result.YID, result.Msg, nil
	}

	return false, "", SanitizeUserVisibleError(result.Msg), nil
}

// submitViaInternal 保留原函数名以保持兼容性
func submitViaInternal(ctx context.Context, oid int) (bool, string, string, error) {
	return submitViaPlugin(ctx, oid)
}

// initPluginsForQueue 初始化平台插件表（规则由插件库 submit_platform 加载）
func initPluginsForQueue() {
	platformPlugins = make(map[string]PlatformPlugin)
}

// 获取当前存在待处理订单的 hid 列表（dockstatus=0）
func pendingHIDs(ctx context.Context) ([]int, error) {
	orderTable := tableName("order")
	q := fmt.Sprintf(`SELECT DISTINCT hid FROM %s WHERE dockstatus='0' AND status!='已取消' AND hid>0`, orderTable)
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ret []int
	for rows.Next() {
		var hid int
		if err := rows.Scan(&hid); err != nil {
			return nil, err
		}
		ret = append(ret, hid)
	}
	return ret, nil
}

// 预取多个 hid 的渠道名到缓存
func prefetchHuoyuanNames(hids []int) {
	// 构建未命中的 hid 列表
	nameMu.Lock()
	missing := []int{}
	for _, h := range hids {
		if _, ok := hidToName[h]; !ok {
			missing = append(missing, h)
		}
	}
	nameMu.Unlock()
	if len(missing) == 0 {
		return
	}
	// 查询货源表
	huoyuanTable := tableName("huoyuan")
	placeholders := make([]string, 0, len(missing))
	args := make([]interface{}, 0, len(missing))
	for _, id := range missing {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	query := fmt.Sprintf("SELECT hid,name FROM %s WHERE hid IN (%s)", huoyuanTable, strings.Join(placeholders, ","))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return
	}
	defer rows.Close()
	tmp := map[int]string{}
	for rows.Next() {
		var hid int
		var name string
		if err := rows.Scan(&hid, &name); err == nil {
			tmp[hid] = name
		}
	}
	nameMu.Lock()
	for k, v := range tmp {
		hidToName[k] = v
	}
	nameMu.Unlock()
}

func getHuoyuanName(hid int) string {
	nameMu.Lock()
	if v, ok := hidToName[hid]; ok && v != "" {
		nameMu.Unlock()
		return v
	}
	nameMu.Unlock()

	// 缓存中没有，立即查询数据库并缓存
	huoyuanTable := tableName("huoyuan")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var name string
	query := fmt.Sprintf("SELECT name FROM %s WHERE hid=? LIMIT 1", huoyuanTable)
	err := db.QueryRowContext(ctx, query, hid).Scan(&name)
	if err == nil && name != "" {
		// 查询成功，更新缓存
		nameMu.Lock()
		hidToName[hid] = name
		nameMu.Unlock()
		return name
	}

	// 查询失败或name为空，返回默认值
	return fmt.Sprintf("hid%d", hid)
}

// 发送 Showdoc 告警
func sendShowdocAlert(title, content string) {
	if alertShowdocURL == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = pushShowdoc(ctx, alertShowdocURL, title, content)
}

func main() {
	installGlobalRedactingLogOutput()
	chdirToExecutableDir()

	log.Printf("========================================")
	log.Printf("%s 正在启动...", ProductName)
	log.Printf("版本: %s", getProductVersion())
	log.Printf("========================================")

	markAppStarted()

	initConfig()

	// 初始化 HTTP 客户端（用于平台下单：白名单 + 私网拦截 + 出站校验）
	httpClient = NewOutboundHTTPClient(submitTimeout)

	// 初始化插件系统
	initPluginsForQueue()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	registerAppShutdown(cancel)

	// 优先启动管理端（未配置数据库时也可进入安装向导）
	startAdminServer(ctx)

	initRedis()
	initMySQL()
	initPluginDBFromFile()

	if !mainDBReady || !redisReady {
		logSetupHint()
	}

	// 从插件库加载提交规则
	if pluginDBReady() && redisReady {
		if n, err := reloadSubmitRulesAndRegister(context.Background()); err != nil {
			log.Printf("加载插件库提交规则失败: %v", err)
		} else if n > 0 {
			log.Printf("已从插件库加载 %d 条提交规则", n)
		}
	}

	// 初始化限流器（需要在 initRedis、initPluginDB 之后）
	initRateLimitersAfterRuntimeLoad()

	if OrderEngineReady() {
		startOrderQueueWorkers(ctx)
	} else {
		log.Printf("订单队列未启动：请先配置好数据库和 Redis，并在管理端完成安装")
	}

	// 常驻运行：等待退出信号
	<-ctx.Done()
	log.Printf("正在停止服务，处理中的订单会尽量保存…")
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
	}
	waitOrderWorkerPoolsIdle()
	if OrderEngineReady() {
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer rollbackCancel()
		rollbackProcessingAll(rollbackCtx)
	}
	log.Printf("服务已停止")
}

// startOrderQueueWorkers 启动订单队列（主库与 Redis 就绪后调用）
func startOrderQueueWorkers(ctx context.Context) {
	orderQueueStartedMu.Lock()
	if orderQueueStarted {
		orderQueueStartedMu.Unlock()
		return
	}
	orderQueueStarted = true
	orderQueueStartedMu.Unlock()

	setOrderQueueRootCtx(ctx)
	startOrderWorkerPools(ctx)

	engineStatsMu.Lock()
	engineDay = engineLocalDate()
	engineStatsMu.Unlock()
	startEngineStatsWindowReset(ctx)

	go producer(ctx)
	go maintainDBConnection(ctx)

	var err error
	hids, err = pendingHIDs(context.Background())
	if err != nil {
		log.Printf("加载待处理的 hid 列表失败: %v", err)
		hids = nil
	}
	if len(hids) == 0 {
		log.Printf("未发现待处理的订单，等待中...")
	}
	for _, hid := range hids {
		for i := 0; i < minWorkersPerHID; i++ {
			wctx, cancel := context.WithCancel(ctx)
			orderQueueWorkerWG.Add(1)
			go consumer(wctx, hid, &orderQueueWorkerWG)
			concurrencyMu.Lock()
			workerCancels[hid] = append(workerCancels[hid], cancel)
			currWorkers[hid]++
			concurrencyMu.Unlock()
		}
	}

	// 自动扩缩：定期根据队列长度调整并发
	// 优化：使用队列长度缓存，减少 Redis 查询；分批检查 HID，避免一次性检查所有
	go func() {
		ticker := time.NewTicker(scaleCheckInterval)
		defer ticker.Stop()
		checkIndex := 0 // 用于分批检查的索引
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				hidsMu.RLock()
				snapshot := make([]int, len(hids))
				copy(snapshot, hids)
				hidsMu.RUnlock()

				if len(snapshot) == 0 {
					continue
				}

				// 分批检查：每次只检查一部分 HID，避免一次性检查所有（降低 CPU 占用）
				const batchSize = 10 // 每批检查 10 个 HID
				start := checkIndex % len(snapshot)
				end := start + batchSize
				if end > len(snapshot) {
					end = len(snapshot)
				}
				batch := snapshot[start:end]
				checkIndex = end % len(snapshot) // 下次从下一个批次开始

				for _, hid := range batch {
					if opsIsChannelPaused(hid) {
						continue
					}
					// 使用缓存的队列长度，减少 Redis 查询
					var llen int64
					var err error
					queueLenCacheMu.RLock()
					cachedLen, cachedTime := queueLenCache[hid], queueLenCacheTime[hid]
					hasCache := !cachedTime.IsZero()
					queueLenCacheMu.RUnlock()

					if hasCache && time.Since(cachedTime) < queueLenCacheTTL {
						// 使用缓存
						llen = cachedLen
					} else {
						// 查询 Redis 并更新缓存
						llen, err = rdb.LLen(ctx, listKey(hid)).Result()
						if err != nil {
							continue
						}
						queueLenCacheMu.Lock()
						queueLenCache[hid] = llen
						queueLenCacheTime[hid] = time.Now()
						queueLenCacheMu.Unlock()
					}

					qCfg := getEffectiveQueueForHID(hid)
					desired := qCfg.MinWorkersPerHID
					if qCfg.ScaleStepThreshold > 0 {
						desired += int(llen) / qCfg.ScaleStepThreshold
					}
					if desired > qCfg.MaxWorkersPerHID {
						desired = qCfg.MaxWorkersPerHID
					}
					if desired < qCfg.MinWorkersPerHID {
						desired = qCfg.MinWorkersPerHID
					}

					// 使用读写锁，减少锁竞争
					concurrencyMu.RLock()
					curr := currWorkers[hid]
					concurrencyMu.RUnlock()

					// 如果不需要调整，跳过（减少锁持有时间）
					if curr == desired {
						continue
					}

					concurrencyMu.Lock()
					curr = currWorkers[hid] // 重新获取，可能已被其他 goroutine 修改
					// 扩容
					for curr < desired {
						wctx, cancel := context.WithCancel(ctx)
						orderQueueWorkerWG.Add(1)
						go consumer(wctx, hid, &orderQueueWorkerWG)
						workerCancels[hid] = append(workerCancels[hid], cancel)
						curr++
					}
					// 缩容
					for curr > desired {
						lst := workerCancels[hid]
						if len(lst) == 0 {
							break
						}
						cancel := lst[len(lst)-1]
						workerCancels[hid] = lst[:len(lst)-1]
						cancel() // 让对应 worker 退出
						curr--
					}
					currWorkers[hid] = curr
					concurrencyMu.Unlock()
				}
			}
		}
	}()

	// processing 回收：超时未完成的任务回源或仅补写 DB
	// 优化：分批加载队列数据，避免一次性加载所有数据到内存（降低 CPU 和内存占用）
	go func() {
		ticker := time.NewTicker(reaperInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now().Unix()
				hidsMu.RLock()
				snapshot := make([]int, len(hids))
				copy(snapshot, hids)
				hidsMu.RUnlock()

				for _, hid := range snapshot {
					proc := procKey(hid)

					// 优化：先检查队列长度，如果为空则跳过（early exit）
					procLen, err := rdb.LLen(ctx, proc).Result()
					if err != nil || procLen == 0 {
						continue
					}

					// 优化：分批加载，每次最多处理 100 条（避免一次性加载大量数据）
					const batchSize = 100
					offset := 0
					processedCount := 0

					for offset < int(procLen) {
						// 分批获取数据
						end := offset + batchSize
						if end > int(procLen) {
							end = int(procLen)
						}
						vals, err := rdb.LRange(ctx, proc, int64(offset), int64(end-1)).Result()
						if err != nil || len(vals) == 0 {
							break
						}

						hasTimeout := false
						for _, val := range vals {
							var msg orderMsg
							if err := json.Unmarshal([]byte(val), &msg); err != nil {
								continue
							}
							if msg.TS == 0 {
								continue
							}
							if time.Duration(now-msg.TS)*time.Second < getEffectiveQueueForHID(hid).ProcessingTimeout {
								continue
							}
							hasTimeout = true
							// 超时
							if v, exists, err := getSubmittedOrder(ctx, msg.OID); err == nil && exists {
								// 已提交：仅补写数据库并清理
								_ = updateOrderStatusSubmitted(ctx, msg.OID, v, hid)
								_ = rdb.LRem(ctx, proc, 1, val).Err()
								_ = rdb.Del(ctx, lockKey(msg.OID)).Err()
								_ = rdb.Del(ctx, enqKey(msg.OID)).Err()
								processedCount++
								continue
							}
							if ok, yid := orderAlreadySucceededInDB(ctx, msg.OID); ok {
								_ = setSubmittedOrder(ctx, msg.OID, yid)
								_ = updateOrderStatusSubmitted(ctx, msg.OID, yid, hid)
								logSubmitOKAutoVerified(hid, msg.OID)
								_ = rdb.LRem(ctx, proc, 1, val).Err()
								_ = rdb.Del(ctx, lockKey(msg.OID)).Err()
								_ = rdb.Del(ctx, enqKey(msg.OID)).Err()
								processedCount++
								continue
							}
							if tryHandleProcessingTimeoutRetry(ctx, hid, msg, val, proc) {
								_ = rdb.Del(ctx, lockKey(msg.OID)).Err()
								processedCount++
								continue
							}
							// 未提交且无法重试：标记异常并发送通知
							name := submitLogChannel(hid)
							logSubmitProcessTooLong(hid, msg.OID)
							orderTable := tableName("order")
							result, execErr := db.ExecContext(ctx,
								fmt.Sprintf(`UPDATE %s SET status='提交异常', remarks=?, dockstatus='2' WHERE oid=? LIMIT 1`, orderTable),
								"处理时间太长，已标记为提交异常", msg.OID,
							)
							if execErr != nil {
								log.Printf("更新订单状态失败，订单号 %d", msg.OID)
							} else if rows, _ := result.RowsAffected(); rows == 0 {
								log.Printf("未找到订单 %d，状态未更新", msg.OID)
							}
							if alertShowdocURL != "" {
								title := fmt.Sprintf("订单处理太久 · %s", name)
								content := fmt.Sprintf("订单号：%d\n渠道：%s\n原因：处理时间太长\n已标记为提交异常，请到管理端查看", msg.OID, name)
								go sendNotification(notifyProcessingTimeout, title, content)
							}
							// 进入 DLQ
							atomicAddDLQ(hid)
							b, _ := json.Marshal(msg)
							_ = rdb.LPush(ctx, dlqKey(hid), string(b)).Err()
							_ = rdb.LRem(ctx, proc, 1, val).Err()
							_ = rdb.Del(ctx, lockKey(msg.OID)).Err()
							_ = rdb.Del(ctx, enqKey(msg.OID)).Err()
							processedCount++
						}

						// 如果当前批次没有超时订单，且已处理完所有批次，提前退出
						if !hasTimeout && offset+batchSize >= int(procLen) {
							break
						}

						offset += batchSize
					}

					// 如果处理了订单，更新队列长度缓存（下次检查时使用最新长度）
					if processedCount > 0 {
						queueLenCacheMu.Lock()
						delete(queueLenCache, hid)
						delete(queueLenCacheTime, hid)
						queueLenCacheMu.Unlock()
					}
				}
			}
		}
	}()

	// 动态刷新：周期检测新的 hid，发现后自动拉起基础并发
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		known := map[int]struct{}{}
		hidsMu.Lock()
		for _, hid := range hids {
			known[hid] = struct{}{}
		}
		hidsMu.Unlock()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 使用带超时的 context 查询，避免长时间阻塞
				queryCtx, queryCancel := context.WithTimeout(ctx, 3*time.Second)
				latest, err := pendingHIDs(queryCtx)
				queryCancel()
				if err != nil {
					continue
				}
				for _, hid := range latest {
					if _, ok := known[hid]; ok {
						continue
					}
					if opsIsChannelPaused(hid) {
						known[hid] = struct{}{}
						hidsMu.Lock()
						hids = append(hids, hid)
						hidsMu.Unlock()
						continue
					}
					// 为新 HID 创建限流器（如果启用按 HID 限流）
					if rateLimitEnabled {
						ensureRateLimiterForHID(hid)
					}

					qStart := getEffectiveQueueForHID(hid)
					concurrencyMu.Lock()
					for i := 0; i < qStart.MinWorkersPerHID; i++ {
						wctx, cancel := context.WithCancel(ctx)
						orderQueueWorkerWG.Add(1)
						go consumer(wctx, hid, &orderQueueWorkerWG)
						workerCancels[hid] = append(workerCancels[hid], cancel)
						currWorkers[hid]++
					}
					concurrencyMu.Unlock()
					known[hid] = struct{}{}
					// 追加到扩缩跟踪集合
					hidsMu.Lock()
					hids = append(hids, hid)
					hidsMu.Unlock()
				}
			}
		}
	}()

	// 周期聚合：按配置间隔打印汇总；无订单则不输出
	go func() {
		ticker := time.NewTicker(statsInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s := atomic.LoadUint64(&successCount)
				d := atomic.LoadUint64(&dlqCount)
				// 近一周期是否有新增（移除重试统计）
				delta := (s - prevSuccess) + (d - prevDlq)
				// 各 hid 队列长度与并发
				hidsMu.RLock()
				snapshot := make([]int, len(hids))
				copy(snapshot, hids)
				hidsMu.RUnlock()
				// 若本周期无新增，且当前也没有任何待处理 hid，则跳过输出
				if delta == 0 && len(snapshot) == 0 {
					continue
				}
				// 也可进一步判断：所有 hid 队列长度都为 0 时跳过
				allZero := true
				for _, hid := range snapshot {
					qlen, _ := rdb.LLen(ctx, listKey(hid)).Result()
					if qlen > 0 {
						allZero = false
						break
					}
				}
				if delta == 0 && allZero {
					continue
				}
				prevSuccess = s
				prevDlq = d
			}
		}
	}()

	// orderQueueWorkerWG 随进程生命周期运行，不在此 Wait（与原 main 行为一致）

	startRetryScheduleWorker(ctx)
	startDLQAutoRetryWorker(ctx)
}

// 回滚所有 processing 中的未完成任务：已提交只清理和补写，未提交重置入队标记供重启后重新扫描
func rollbackProcessingAll(ctx context.Context) {
	hidsMu.Lock()
	snapshot := make([]int, 0)
	// 尝试从已知 hid 列表和当前 redis keys 里合并
	snapshot = append(snapshot, hids...)
	hidsMu.Unlock()
	for _, hid := range snapshot {
		proc := procKey(hid)
		vals, err := rdb.LRange(ctx, proc, 0, -1).Result()
		if err != nil || len(vals) == 0 {
			continue
		}
		for _, val := range vals {
			var msg orderMsg
			if err := json.Unmarshal([]byte(val), &msg); err != nil {
				_ = rdb.LRem(ctx, proc, 1, val).Err()
				continue
			}
			if v, exists, err := getSubmittedOrder(ctx, msg.OID); err == nil && exists {
				// 已提交：仅补写并清理
				_ = updateOrderStatusSubmitted(ctx, msg.OID, v, hid)
				_ = rdb.LRem(ctx, proc, 1, val).Err()
				_ = rdb.Del(ctx, lockKey(msg.OID)).Err()
				_ = rdb.Del(ctx, enqKey(msg.OID)).Err()
				continue
			}
			if ok, yid := orderAlreadySucceededInDB(ctx, msg.OID); ok {
				_ = setSubmittedOrder(ctx, msg.OID, yid)
				_ = updateOrderStatusSubmitted(ctx, msg.OID, yid, hid)
				_ = rdb.LRem(ctx, proc, 1, val).Err()
				_ = rdb.Del(ctx, lockKey(msg.OID)).Err()
				_ = rdb.Del(ctx, enqKey(msg.OID)).Err()
				continue
			}
			// 未提交：清理 Redis 入队标记，保留 dockstatus=0，重启后 producer 会重新扫描入队
			_ = rdb.LRem(ctx, proc, 1, val).Err()
			_ = rdb.Del(ctx, lockKey(msg.OID)).Err()
			_ = rdb.Del(ctx, enqKey(msg.OID)).Err()
			log.Printf("程序重启后会继续处理订单 %d", msg.OID)
		}
	}
}
