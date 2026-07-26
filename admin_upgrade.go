package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// frontendStaticDir 生产环境管理前端静态资源目录（与二进制同级）
const frontendStaticDir = "web"

type releaseInfo struct {
	HasUpdate   bool   `json:"has_update"`
	Version     string `json:"version"`
	Changelog   string `json:"changelog,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Force       bool   `json:"force"`
	PublishedAt string `json:"published_at,omitempty"`
}

type upgradeStatusDTO struct {
	Phase       string       `json:"phase"`
	Message     string       `json:"message"`
	Progress    int          `json:"progress"`
	CurrentVer  string       `json:"current_version"`
	TargetVer   string       `json:"target_version,omitempty"`
	Error       string       `json:"error,omitempty"`
	Release     *releaseInfo `json:"release,omitempty"`
	StartedAt   string       `json:"started_at,omitempty"`
	CompletedAt string       `json:"completed_at,omitempty"`
}

type upgradeState struct {
	mu          sync.Mutex
	phase       string
	message     string
	progress    int
	targetVer   string
	errMsg      string
	release     *releaseInfo
	startedAt   time.Time
	completedAt time.Time
	running     bool
}

var upgradeMgr upgradeState

func upgradePlatformID() string {
	return runtime.GOOS + "_" + runtime.GOARCH
}

func upgradeWorkDir() (string, error) {
	exe, err := resolveExecutablePath()
	if err == nil {
		if dir := filepath.Dir(exe); dir != "" {
			return dir, nil
		}
	}
	return os.Getwd()
}

func upgradeStagingDir(version string) (string, error) {
	workDir, err := upgradeWorkDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(workDir, ".tj-update", "staging", sanitizeUpgradePath(version))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func sanitizeUpgradePath(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "..", "")
	s = strings.ReplaceAll(s, string(os.PathSeparator), "_")
	s = strings.ReplaceAll(s, "/", "_")
	if s == "" {
		return "unknown"
	}
	return s
}

func (s *upgradeState) snapshot() upgradeStatusDTO {
	s.mu.Lock()
	defer s.mu.Unlock()
	dto := upgradeStatusDTO{
		Phase:      s.phase,
		Message:    s.message,
		Progress:   s.progress,
		CurrentVer: getProductVersion(),
		TargetVer:  s.targetVer,
		Error:      s.errMsg,
	}
	if s.release != nil {
		cp := *s.release
		dto.Release = &cp
	}
	if !s.startedAt.IsZero() {
		dto.StartedAt = s.startedAt.Format(time.RFC3339)
	}
	if !s.completedAt.IsZero() {
		dto.CompletedAt = s.completedAt.Format(time.RFC3339)
	}
	if dto.Phase == "" {
		dto.Phase = "idle"
	}
	return dto
}

func (s *upgradeState) set(phase, message string, progress int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phase = phase
	s.message = message
	s.progress = progress
}

func (s *upgradeState) fail(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phase = "failed"
	s.message = message
	s.errMsg = message
	s.progress = 0
	s.running = false
	s.completedAt = time.Now()
}

func (s *upgradeState) begin(target string, rel *releaseInfo) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	s.phase = "checking"
	s.message = "准备升级..."
	s.progress = 0
	s.targetVer = target
	s.errMsg = ""
	s.release = rel
	s.startedAt = time.Now()
	s.completedAt = time.Time{}
	return true
}

func (s *upgradeState) finishRestarting() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phase = "restarting"
	s.message = "正在重启服务…若长时间 502，请 SSH 查看程序目录下 tj-upgrade-restart.log，或手动 systemctl/supervisor 重启"
	s.progress = 100
}

func fetchLatestRelease(ctx context.Context) (*releaseInfo, error) {
	_ = ctx
	return nil, fmt.Errorf("在线升级已禁用（本副本已去除授权站）")
}
func downloadReleaseFile(ctx context.Context, rel *releaseInfo, destPath string, onProgress func(pct int)) error {
	// 下载凭 release/latest 签发的短时 token，不重复校验 session（避免大包下载中途会话过期）
	if rel == nil || strings.TrimSpace(rel.DownloadURL) == "" {
		return fmt.Errorf("缺少下载地址")
	}
	if err := ValidateOutboundHTTPURL(ctx, rel.DownloadURL); err != nil {
		return fmt.Errorf("下载地址被安全策略拦截")
	}

	client := NewOutboundHTTPClient(30 * time.Minute)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rel.DownloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 OrderSync-Upgrade/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	tmpPath := destPath + ".part"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	total := rel.Size
	if total <= 0 {
		total = resp.ContentLength
	}
	var written int64
	buf := make([]byte, 32*1024)
	hasher := sha256.New()
	multi := io.MultiWriter(out, hasher)

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := multi.Write(buf[:n]); wErr != nil {
				out.Close()
				os.Remove(tmpPath)
				return wErr
			}
			written += int64(n)
			if total > 0 && onProgress != nil {
				pct := int(written * 100 / total)
				if pct > 99 {
					pct = 99
				}
				onProgress(pct)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			out.Close()
			os.Remove(tmpPath)
			return readErr
		}
	}
	if err := out.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	sum := hex.EncodeToString(hasher.Sum(nil))
	expected := strings.ToLower(strings.TrimSpace(rel.SHA256))
	if expected != "" && !strings.EqualFold(sum, expected) {
		os.Remove(tmpPath)
		return fmt.Errorf("文件校验失败（SHA256 不匹配）")
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if onProgress != nil {
		onProgress(100)
	}
	return nil
}

func fileHasZipMagic(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 4)
	n, err := f.Read(buf)
	if err != nil || n < 2 {
		return false
	}
	return buf[0] == 'P' && buf[1] == 'K'
}

func isNativeExecutable(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 4)
	n, err := f.Read(buf)
	if err != nil || n < 4 {
		return false
	}
	switch runtime.GOOS {
	case "windows":
		return buf[0] == 'M' && buf[1] == 'Z'
	default:
		return buf[0] == 0x7f && buf[1] == 'E' && buf[2] == 'L' && buf[3] == 'F'
	}
}

func cleanUpgradeZipEntryName(raw string) (string, bool) {
	name := filepath.Clean(filepath.FromSlash(raw))
	if name == "." || name == "" {
		return "", false
	}
	if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
		return "", false
	}
	return name, true
}

// detectUpgradeZipRootPrefix 若 ZIP 内所有条目都在同一顶层目录下（如 智能提交引擎V3.4/...），返回该目录名以便解压时剥离。
func detectUpgradeZipRootPrefix(entries []*zip.File) string {
	root := ""
	for _, f := range entries {
		name, ok := cleanUpgradeZipEntryName(f.Name)
		if !ok {
			continue
		}
		sep := string(os.PathSeparator)
		idx := strings.Index(name, sep)
		if idx < 0 {
			return ""
		}
		top := name[:idx]
		if root == "" {
			root = top
		} else if root != top {
			return ""
		}
	}
	return root
}

func isUpgradeWebStaticDir(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	return isBuiltWebIndex(filepath.Join(dir, "index.html"))
}

func findUpgradeBinary(destDir, exeName string) string {
	candidate := filepath.Join(destDir, exeName)
	if isNativeExecutable(candidate) {
		return candidate
	}
	found := ""
	depth := -1
	_ = filepath.WalkDir(destDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		base := d.Name()
		if base != exeName && base != "tj" && base != "tj.exe" {
			return nil
		}
		if !isNativeExecutable(path) {
			return nil
		}
		rel, relErr := filepath.Rel(destDir, path)
		if relErr != nil {
			return nil
		}
		dpt := strings.Count(rel, string(os.PathSeparator))
		if found == "" || dpt < depth {
			found = path
			depth = dpt
		}
		return nil
	})
	return found
}

func findUpgradeWebStatic(destDir, nearBinary string) string {
	candidate := filepath.Join(destDir, frontendStaticDir)
	if isUpgradeWebStaticDir(candidate) {
		return candidate
	}
	if nearBinary != "" {
		sibling := filepath.Join(filepath.Dir(nearBinary), frontendStaticDir)
		if isUpgradeWebStaticDir(sibling) {
			return sibling
		}
	}
	found := ""
	depth := -1
	_ = filepath.WalkDir(destDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || d.Name() != frontendStaticDir {
			return nil
		}
		if !isUpgradeWebStaticDir(path) {
			return nil
		}
		rel, relErr := filepath.Rel(destDir, path)
		if relErr != nil {
			return nil
		}
		dpt := strings.Count(rel, string(os.PathSeparator))
		if found == "" || dpt < depth {
			found = path
			depth = dpt
		}
		return nil
	})
	return found
}

func extractUpgradeZip(zipPath, destDir string) (binaryPath string, webStaticPath string, err error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", "", err
	}
	defer r.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", "", err
	}

	exeName := "tj"
	if runtime.GOOS == "windows" {
		exeName = "tj.exe"
	}

	rootPrefix := detectUpgradeZipRootPrefix(r.File)
	sep := string(os.PathSeparator)

	for _, f := range r.File {
		name, ok := cleanUpgradeZipEntryName(f.Name)
		if !ok {
			continue
		}
		if rootPrefix != "" && strings.HasPrefix(name, rootPrefix+sep) {
			name = strings.TrimPrefix(name, rootPrefix+sep)
			if name == "" {
				continue
			}
		}
		target := filepath.Join(destDir, name)
		if !strings.HasPrefix(target, filepath.Clean(destDir)+sep) && target != filepath.Clean(destDir) {
			continue
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", "", err
		}
		rc, err := f.Open()
		if err != nil {
			return "", "", err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode().Perm())
		if err != nil {
			rc.Close()
			return "", "", err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return "", "", err
		}
		out.Close()
		rc.Close()
	}

	binaryPath = findUpgradeBinary(destDir, exeName)
	webStaticPath = findUpgradeWebStatic(destDir, binaryPath)
	if binaryPath == "" {
		return "", "", fmt.Errorf("更新包中未找到可执行文件")
	}
	if webStaticPath == "" {
		log.Printf("[升级] 更新包中未找到 web 静态目录，将仅更新程序")
	} else if rootPrefix != "" {
		log.Printf("[升级] 已识别 ZIP 顶层目录 %q 并剥离后解压", rootPrefix)
	}
	return binaryPath, webStaticPath, nil
}

func backupCurrentBinary(exePath string) (string, error) {
	backup := exePath + ".bak." + time.Now().Format("20060102150405")
	src, err := os.Open(exePath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.OpenFile(backup, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return "", err
	}
	if err := dst.Close(); err != nil {
		return "", err
	}
	return backup, nil
}

func writeUpgradeScript(scriptPath string, pid int, stagingBinary, stagingWebStatic, exePath, workDir string) error {
	logPath := filepath.Join(workDir, "tj-upgrade-restart.log")
	updateRoot := filepath.Join(workDir, ".tj-update")
	if runtime.GOOS == "windows" {
		content := fmt.Sprintf(`@echo off
setlocal EnableExtensions
set "PID=%d"
set "NEW=%s"
set "EXE=%s"
set "WEBSRC=%s"
set "WORKDIR=%s"
set "LOG=%s"
set "UPDATE_ROOT=%s"
:waitloop
tasklist /FI "PID eq %%PID%%" 2>NUL | find "%%PID%%" >NUL
if not errorlevel 1 (
  timeout /t 1 /nobreak >NUL
  goto waitloop
)
timeout /t 1 /nobreak >NUL
copy /Y "%%NEW%%" "%%EXE%%.part" >NUL
if errorlevel 1 (
  echo upgrade copy failed>>"%%LOG%%"
  exit /b 1
)
move /Y "%%EXE%%.part" "%%EXE%%" >NUL
if errorlevel 1 (
  echo upgrade move failed>>"%%LOG%%"
  exit /b 1
)
if exist "%%WEBSRC%%" (
  if exist "%%WORKDIR%%\web" rmdir /S /Q "%%WORKDIR%%\web"
  xcopy /E /I /Y "%%WEBSRC%%" "%%WORKDIR%%\web" >NUL
  if errorlevel 1 (
    echo upgrade web copy failed>>"%%LOG%%"
    exit /b 1
  )
  if exist "%%WORKDIR%%\web\dist" rmdir /S /Q "%%WORKDIR%%\web\dist"
) else (
  echo upgrade web static missing>>"%%LOG%%"
)
cd /D "%%WORKDIR%%"
start "" "%%EXE%%"
timeout /t 2 /nobreak >NUL
if exist "%%UPDATE_ROOT%%" rmdir /S /Q "%%UPDATE_ROOT%%"
if exist "%%LOG%%" del /F /Q "%%LOG%%"
for /f "skip=1 delims=" %%F in ('dir /b /o-d "%%EXE%%.bak.*" 2^>nul') do del /F /Q "%%F" 2>nul
del "%%~f0"
`, pid, stagingBinary, exePath, stagingWebStatic, workDir, logPath, updateRoot)
		return os.WriteFile(scriptPath, []byte(content), 0o755)
	}

	content := fmt.Sprintf(`#!/bin/sh
set -e
PID=%d
NEW=%q
EXE=%q
WEBSRC=%q
WORKDIR=%q
LOG=%q
UPDATE_ROOT=%q
cleanup_upgrade_artifacts() {
  rm -rf "$UPDATE_ROOT"
  rm -f "$LOG"
  for old in $(ls -t "$EXE.bak."* 2>/dev/null | tail -n +2); do
    rm -f "$old"
  done
}
while kill -0 "$PID" 2>/dev/null; do sleep 1; done
sleep 1
cp -f "$NEW" "$EXE.part"
chmod +x "$EXE.part"
mv -f "$EXE.part" "$EXE"
if [ -d "$WEBSRC" ]; then
  rm -rf "$WORKDIR/web"
  cp -a "$WEBSRC" "$WORKDIR/web"
  rm -rf "$WORKDIR/web/dist"
else
  echo "upgrade web static missing: $WEBSRC" >> "$LOG"
fi
cd "$WORKDIR"
if [ -f "$WORKDIR/.tj-systemd-unit" ] && command -v systemctl >/dev/null 2>&1; then
  UNIT=$(tr -d '\r\n' < "$WORKDIR/.tj-systemd-unit")
  if [ -n "$UNIT" ]; then
    systemctl restart "$UNIT" >> "$LOG" 2>&1 || exit 1
    cleanup_upgrade_artifacts
    rm -f "$0"
    exit 0
  fi
fi
if command -v supervisorctl >/dev/null 2>&1 && [ -f "$WORKDIR/.tj-supervisor-program" ]; then
  PROG=$(tr -d '\r\n' < "$WORKDIR/.tj-supervisor-program")
  if [ -n "$PROG" ]; then
    supervisorctl restart "$PROG" >> "$LOG" 2>&1 || exit 1
    cleanup_upgrade_artifacts
    rm -f "$0"
    exit 0
  fi
fi
nohup "$EXE" >> "$LOG" 2>&1 &
NEWPID=$!
sleep 2
if ! kill -0 "$NEWPID" 2>/dev/null; then
  echo "upgrade start failed, see $LOG" >> "$LOG"
  exit 1
fi
cleanup_upgrade_artifacts
rm -f "$0"
`, pid, stagingBinary, exePath, stagingWebStatic, workDir, logPath, updateRoot)
	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		return err
	}
	return nil
}

func launchUpgradeScript(scriptPath string) error {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd.exe", "/C", scriptPath)
		cmd.Dir = filepath.Dir(scriptPath)
		return cmd.Start()
	}
	cmd := exec.Command("/bin/sh", scriptPath)
	cmd.Dir = filepath.Dir(scriptPath)
	return cmd.Start()
}

func runUpgradeJob(rel *releaseInfo) {
	defer func() {
		upgradeMgr.mu.Lock()
		upgradeMgr.running = false
		upgradeMgr.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
	defer cancel()

	upgradeMgr.set("downloading", "正在下载更新包...", 5)
	stagingDir, err := upgradeStagingDir(rel.Version)
	if err != nil {
		upgradeMgr.fail(err.Error())
		return
	}

	downloadName := "package.zip"
	downloadPath := filepath.Join(stagingDir, downloadName)

	if err := downloadReleaseFile(ctx, rel, downloadPath, func(pct int) {
		upgradeMgr.set("downloading", fmt.Sprintf("正在下载更新包... %d%%", pct), pct)
	}); err != nil {
		upgradeMgr.fail(err.Error())
		return
	}

	upgradeMgr.set("verifying", "正在校验并准备安装...", 95)

	exePath, err := resolveExecutablePath()
	if err != nil {
		upgradeMgr.fail("无法定位当前程序路径")
		return
	}
	workDir, err := upgradeWorkDir()
	if err != nil {
		upgradeMgr.fail(err.Error())
		return
	}

	stagingBinary := downloadPath
	stagingWebStatic := ""
	if fileHasZipMagic(downloadPath) {
		extractDir := filepath.Join(stagingDir, "extracted")
		bin, webStatic, err := extractUpgradeZip(downloadPath, extractDir)
		if err != nil {
			upgradeMgr.fail(err.Error())
			return
		}
		stagingBinary = bin
		stagingWebStatic = webStatic
		log.Printf("[升级] 解压完成: binary=%s web=%s", stagingBinary, stagingWebStatic)
	} else if !isNativeExecutable(downloadPath) {
		upgradeMgr.fail("下载内容既不是 ZIP 发布包，也不是有效的可执行文件（授权站 download_url 通常返回 ZIP，请勿直接替换为 tj）")
		return
	}
	if err := os.Chmod(stagingBinary, 0o755); err != nil {
		log.Printf("[升级] 设置新版本执行权限失败（继续）: %v", err)
	}

	if _, err := backupCurrentBinary(exePath); err != nil {
		log.Printf("[升级] 备份当前程序失败（继续）: %v", err)
	}

	upgradeMgr.set("applying", "正在应用更新...", 98)
	scriptPath := filepath.Join(stagingDir, "apply-upgrade"+upgradeScriptExt())
	if err := writeUpgradeScript(scriptPath, os.Getpid(), stagingBinary, stagingWebStatic, exePath, workDir); err != nil {
		upgradeMgr.fail(err.Error())
		return
	}
	if err := launchUpgradeScript(scriptPath); err != nil {
		upgradeMgr.fail("启动升级脚本失败: " + err.Error())
		return
	}

	upgradeMgr.finishRestarting()
	log.Printf("[升级] 更新包已就绪，正在重启...")
	requestAppRestart("在线升级")
}

func upgradeScriptExt() string {
	if runtime.GOOS == "windows" {
		return ".bat"
	}
	return ".sh"
}

func adminSystemInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1,
		"msg":  "ok",
		"data": map[string]interface{}{
			"product_name":    ProductName,
			"product_version": getProductVersion(),
			"platform":        upgradePlatformID(),
		},
	})
}

func adminUpgradeStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "ok", "data": upgradeMgr.snapshot()})
}

func adminUpgradeCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	rel, err := fetchLatestRelease(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1,
		"msg":  "ok",
		"data": map[string]interface{}{
			"current_version": getProductVersion(),
			"release":         rel,
		},
	})
}

func adminUpgradeApplyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}

	upgradeMgr.mu.Lock()
	if upgradeMgr.running {
		upgradeMgr.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]interface{}{"code": -1, "msg": "已有升级任务进行中"})
		return
	}
	upgradeMgr.mu.Unlock()

	body, _ := io.ReadAll(r.Body)
	var req struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(body, &req)

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	rel, err := fetchLatestRelease(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	if !rel.HasUpdate {
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": -1, "msg": "当前已是最新版本"})
		return
	}
	if v := strings.TrimSpace(req.Version); v != "" && !strings.EqualFold(v, rel.Version) {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "目标版本与最新版本不一致"})
		return
	}
	if strings.TrimSpace(rel.DownloadURL) == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": -1, "msg": "授权站未提供下载地址"})
		return
	}

	if !upgradeMgr.begin(rel.Version, rel) {
		writeJSON(w, http.StatusConflict, map[string]interface{}{"code": -1, "msg": "已有升级任务进行中"})
		return
	}

	go runUpgradeJob(rel)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 1,
		"msg":  "升级已开始，服务即将重启",
		"data": upgradeMgr.snapshot(),
	})
}
