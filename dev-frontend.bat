@echo off
chcp 65001 >nul
cd /d "%~dp0web"

echo ========================================
echo   tj frontend - Vite dev mode
echo   - no npm run build needed
echo   - Page: http://localhost:5173/install
echo   - /api proxies to backend (default 127.0.0.1:8090, set ADMIN_PORT to match config.yaml)
echo   - Start dev-backend.bat first
echo   - If styles look wrong, restart Vite
echo ========================================
echo.

if not exist "node_modules\" (
  echo First run: installing npm dependencies...
  call npm install
  if errorlevel 1 (
    echo npm install failed
    pause
    exit /b 1
  )
)

call npm run dev
