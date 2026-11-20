@echo off
setlocal
chcp 65001 >nul
cd /d "%~dp0"

rem Clear env vars that may break Electron (e.g. DSH sandbox sets ELECTRON_RUN_AS_NODE=1)
set "ELECTRON_RUN_AS_NODE="

echo ============================================
echo   HTTPX Debugger - one-click launcher
echo ============================================

rem ---------- 1. Go core: rebuild if missing or sources are newer ----------
set "NEED_BUILD=0"
if not exist "core\httpx-core.exe" (
  set "NEED_BUILD=1"
) else (
  powershell -NoProfile -Command "$e = Get-Item 'core\httpx-core.exe'; $s = Get-ChildItem 'core\*.go' | Sort-Object LastWriteTime -Descending | Select-Object -First 1; if ($s -and $s.LastWriteTime -gt $e.LastWriteTime) { exit 1 } else { exit 0 }" >nul 2>nul
  if errorlevel 1 set "NEED_BUILD=1"
)

where go >nul 2>nul
set "HAS_GO=%errorlevel%"

if "%NEED_BUILD%"=="1" (
  if not "%HAS_GO%"=="0" (
    echo [ERROR] core needs a rebuild but "go" was not found in PATH.
    echo         Install Go 1.24+ and retry.
    pause
    exit /b 1
  )
  echo [1/3] Building core\httpx-core.exe ...
  pushd core
  call go build -o httpx-core.exe .
  if errorlevel 1 (
    echo [ERROR] go build failed.
    popd
    pause
    exit /b 1
  )
  popd
  echo       Build done.
) else (
  echo [1/3] core\httpx-core.exe is ready.
)

rem ---------- 2. UI dependencies ----------
if not exist "ui\node_modules" (
  echo [2/3] Installing UI dependencies, first run may take a while...
  pushd ui
  call npm install
  if errorlevel 1 (
    echo [ERROR] npm install failed.
    popd
    pause
    exit /b 1
  )
  popd
) else (
  echo [2/3] UI dependencies are ready.
)

rem Electron binary is sometimes skipped by postinstall; download it if missing
if not exist "ui\node_modules\electron\dist\electron.exe" (
  echo [2/3] Downloading electron binary ...
  pushd ui
  call node node_modules\electron\install.js
  popd
)

rem ---------- 3. Launch ----------
if "%HTTPX_DRY%"=="1" (
  echo [3/3] DRY mode: checks passed, UI not launched.
  exit /b 0
)

echo [3/3] Starting HTTPX Debugger (dev mode, press Ctrl+C to quit)...
pushd ui
call npm run dev
popd

exit /b 0
