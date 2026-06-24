@echo off
echo ===================================================
echo [*] Starting local build for VLESS Reality Panel...
echo ===================================================

:: Step 1: Build frontend
echo [*] Step 1: Compiling frontend Vue app...
cd web\frontend
call npm install
if %ERRORLEVEL% neq 0 (
    echo [!] Frontend npm install failed!
    exit /b %ERRORLEVEL%
)
call npm run build
if %ERRORLEVEL% neq 0 (
    echo [!] Frontend npm run build failed!
    exit /b %ERRORLEVEL%
)
cd ..\..

:: Step 2: Cross compile Go backend for Linux
echo [*] Step 2: Compiling Go backend for Linux amd64...
cd web\backend-go
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64
go build -ldflags="-s -w" -o vless-panel main.go
if %ERRORLEVEL% neq 0 (
    echo [!] Go backend build failed!
    exit /b %ERRORLEVEL%
)
cd ..\..

echo ===================================================
echo [+] Build successful!
echo [+] Binary: web/backend-go/vless-panel
echo [+] Frontend static: web/backend-go/dist
echo ===================================================
echo Tips:
echo 1. Run: git add -f web/backend-go/vless-panel web/backend-go/dist
echo 2. Run: git commit -m "build: local compile assets"
echo 3. Run: git push
echo 4. On VPS, pull and run docker build using Dockerfile.fast or run directly.
echo ===================================================
