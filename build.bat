set GOROOT=E:\dev\sdk\go\gopath\pkg\mod\golang.org\toolchain@v0.0.1-go1.27.0.windows-amd64
set GOPATH=D:\dev\dev\sdk\gopath
set CGO_ENABLED=0
@REM set GOARCH=amd64
@REM set GOOS=windows
set GOARCH=amd64
set GOOS=linux
@REM set GOARCH=amd64
@REM set GOOS=darwin
go build -trimpath  -ldflags="-s -w -z max-page-size=16384" -o build/chrelyonly-email-server-go main.go