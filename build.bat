@echo off
echo Building Chrelyonly Email Server...
set GOROOT=E:\dev\sdk\go\gopath\pkg\mod\golang.org\toolchain@v0.0.1-go1.27.0.windows-amd64
set GOPATH=E:\dev\sdk\go\gopath
set PATH=%GOROOT%\bin;%PATH%
go build -o mailserver.exe main.go
echo Build finished successfully!
