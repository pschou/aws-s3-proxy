PROG_NAME := "bucket-http-proxy"
VERSION = 0.1.$(shell date +%Y%m%d.%H%M)
FLAGS := "-s -w -X main.Version=${VERSION}"


build:
	CGO_ENABLED=0 go build -ldflags=${FLAGS} -o ${PROG_NAME} *.go

tiny: build
	upx ${PROG_NAME}
	#upx --ultra-brute ${PROG_NAME}
