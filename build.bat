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
echo 1. These outputs are local build artifacts and are ignored by Git.
echo 2. Upload/copy them to the VPS together with Dockerfile.fast when using fast packaging.
echo 3. On VPS, run docker build -f Dockerfile.fast -t vless-reality-panel:latest .
echo ===================================================
