@echo off
echo Building Chrelyonly Mail Server...
go build -o email-server.exe main.go
if %ERRORLEVEL% equ 0 (
    echo Build successful! Run email-server.exe to start.
) else (
    echo Build failed!
)
