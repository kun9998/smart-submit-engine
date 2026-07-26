@echo off
chcp 65001 >nul
cd /d "%~dp0web"

echo ========================================
echo   构建管理端前端（发布用）
echo   - 产物: web\dist\（本地 go run 可直接用）
echo   - 发布包: release\web\（复制到与 tj.exe 同级的 web\）
echo   - API 全部走相对路径 /api，不绑定 8090 端口
echo ========================================
echo.

if not exist "node_modules\" (
  call npm install
  if errorlevel 1 exit /b 1
)

call npm run build:release
if errorlevel 1 exit /b 1

echo.
echo 完成。请重启 Go 服务，并使用 config.yaml 中配置的 admin.addr 端口访问。
pause
