PROG_NAME := "bucket-http-proxy"
VERSION = 0.1.$(shell date +%Y%m%d.%H%M)
FLAGS := "-s -w -X main.version=${VERSION}"


build:
	CGO_ENABLED=0 go build -ldflags=${FLAGS} -o ${PROG_NAME} *.go

full: build
	upx --ultra-brute ${PROG_NAME}
