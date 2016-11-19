set PKGNAME=github.com/oneumyvakin/slacker

go fmt %PKGNAME%
goimports.exe -w .

set GOOS=linux
set GOARCH=amd64
go build %PKGNAME%

set GOOS=windows
set GOARCH=amd64
go build %PKGNAME%