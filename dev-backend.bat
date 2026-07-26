@echo off
chcp 65001 >nul
cd /d "%~dp0"

echo ========================================
echo   tj backend - local dev
echo   - go run directly, no build step
echo   - Admin API port: see config.yaml admin.addr (default :8090)
echo   - Or dev-frontend.bat -^> http://localhost:5173/install
echo   - After install: set auth.authcode in config.yaml
echo ========================================
echo.

go run .

if errorlevel 1 (
  echo.
  echo Startup failed. Check config.yaml / Redis / MySQL / auth.authcode
  pause
)
