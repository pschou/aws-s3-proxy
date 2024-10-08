module main

go 1.20

replace github.com/pschou/bucket-http-proxy/types => ./types

require (
	github.com/araddon/dateparse v0.0.0-20210429162001-6b43995a97de
	github.com/aws/aws-sdk-go-v2 v1.21.0
	github.com/aws/aws-sdk-go-v2/credentials v1.13.38
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.13.11
	github.com/aws/aws-sdk-go-v2/service/s3 v1.38.5
	github.com/pschou/go-convert/bin v0.0.0-20230315170244-4707bf44a557
	github.com/pschou/go-sorting/numstr v0.0.0-20230926171104-73c9f807d196
	github.com/remeh/sizedwaitgroup v1.0.0
	github.com/valyala/fasthttp v1.50.0
)

require (
	github.com/andybalholm/brotli v1.0.5 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.4.13 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.41 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.35 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.1.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.9.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.1.36 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.35 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.15.4 // indirect
	github.com/aws/smithy-go v1.14.2 // indirect
	github.com/cymertek/go-big v0.0.0-20221028234842-57aba6a92118 // indirect
	github.com/klauspost/compress v1.16.3 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
)
