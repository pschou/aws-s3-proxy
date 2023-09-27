package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unsafe"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Checksum struct {
	// This header can be used as a data integrity check to verify that the data
	// received is the same data that was originally sent. This header specifies the
	// base64-encoded, 32-bit CRC32 checksum of the object. For more information, see
	// Checking object integrity (https://docs.aws.amazon.com/AmazonS3/latest/userguide/checking-object-integrity.html)
	// in the Amazon S3 User Guide.
	ChecksumCRC32 *string `json:",omitempty"`

	// The base64-encoded, 32-bit CRC32C checksum of the object. This will only be
	// present if it was uploaded with the object. With multipart uploads, this may not
	// be a checksum value of the object. For more information about how checksums are
	// calculated with multipart uploads, see Checking object integrity (https://docs.aws.amazon.com/AmazonS3/latest/userguide/checking-object-integrity.html#large-object-checksums)
	// in the Amazon S3 User Guide.
	ChecksumCRC32C *string `json:",omitempty"`

	// The base64-encoded, 160-bit SHA-1 digest of the object. This will only be
	// present if it was uploaded with the object. With multipart uploads, this may not
	// be a checksum value of the object. For more information about how checksums are
	// calculated with multipart uploads, see Checking object integrity (https://docs.aws.amazon.com/AmazonS3/latest/userguide/checking-object-integrity.html#large-object-checksums)
	// in the Amazon S3 User Guide.
	ChecksumSHA1 *string `json:",omitempty"`

	// The base64-encoded, 256-bit SHA-256 digest of the object. This will only be
	// present if it was uploaded with the object. With multipart uploads, this may not
	// be a checksum value of the object. For more information about how checksums are
	// calculated with multipart uploads, see Checking object integrity (https://docs.aws.amazon.com/AmazonS3/latest/userguide/checking-object-integrity.html#large-object-checksums)
	// in the Amazon S3 User Guide.
	ChecksumSHA256 *string `json:",omitempty"`
}

func s2b(str string) []byte {
	if str == "" {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(str), len(str))
}

func b2s(bs []byte) string {
	if len(bs) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(bs), len(bs))
}

func slashed(s string) bool {
	if len(s) == 0 {
		return true
	}
	return s[len(s)-1] == '/'
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func marshalChecksum(obj interface{}) string {
	var cs Checksum
	switch t := obj.(type) {
	case *Checksum:
		cs = *t
	case *s3.GetObjectOutput:
		cs.ChecksumCRC32 = t.ChecksumCRC32
		cs.ChecksumCRC32C = t.ChecksumCRC32C
		cs.ChecksumSHA1 = t.ChecksumSHA1
		cs.ChecksumSHA256 = t.ChecksumSHA256
	case *s3.GetObjectAttributesOutput:
		if t.Checksum != nil {
			cs.ChecksumCRC32 = t.Checksum.ChecksumCRC32
			cs.ChecksumCRC32C = t.Checksum.ChecksumCRC32C
			cs.ChecksumSHA1 = t.Checksum.ChecksumSHA1
			cs.ChecksumSHA256 = t.Checksum.ChecksumSHA256
		}
	default:
		return ""
	}
	dat, _ := json.Marshal(cs)
	return b2s(dat)
}

func encodeChecksum(obj interface{}) string {
	var etag string
	var cs Checksum
	switch t := obj.(type) {
	//case *Checksum:
	//	cs = *t
	case *s3.HeadObjectOutput:
		cs.ChecksumCRC32 = t.ChecksumCRC32
		cs.ChecksumCRC32C = t.ChecksumCRC32C
		cs.ChecksumSHA1 = t.ChecksumSHA1
		cs.ChecksumSHA256 = t.ChecksumSHA256
		etag = *t.ETag
	case *s3.GetObjectOutput:
		cs.ChecksumCRC32 = t.ChecksumCRC32
		cs.ChecksumCRC32C = t.ChecksumCRC32C
		cs.ChecksumSHA1 = t.ChecksumSHA1
		cs.ChecksumSHA256 = t.ChecksumSHA256
		etag = *t.ETag
	case *s3.GetObjectAttributesOutput:
		if t.Checksum != nil {
			cs.ChecksumCRC32 = t.Checksum.ChecksumCRC32
			cs.ChecksumCRC32C = t.Checksum.ChecksumCRC32C
			cs.ChecksumSHA1 = t.Checksum.ChecksumSHA1
			cs.ChecksumSHA256 = t.Checksum.ChecksumSHA256
		}
		etag = *t.ETag
	default:
		return "failed to match"
	}
	if cs.ChecksumSHA256 != nil {
		sDec, _ := base64.StdEncoding.DecodeString(*cs.ChecksumSHA256)
		return fmt.Sprintf("{SHA256}%02x", sDec)
	} else if cs.ChecksumSHA1 != nil {
		sDec, _ := base64.StdEncoding.DecodeString(*cs.ChecksumSHA1)
		return fmt.Sprintf("{SHA}%02x", sDec)
	} else if cs.ChecksumCRC32C != nil {
		sDec, _ := base64.StdEncoding.DecodeString(*cs.ChecksumCRC32C)
		return fmt.Sprintf("{CRC32C}%02x", sDec)
	} else if cs.ChecksumCRC32 != nil {
		sDec, _ := base64.StdEncoding.DecodeString(*cs.ChecksumCRC32)
		return fmt.Sprintf("{CRC32}%02x", sDec)
	} else if len(etag) > 0 {
		return "{AWS-MD}" + unquote(etag)
	}
	return "-"
}

func unmarshalChecksum(dat []byte, obj interface{}) {
	var cs Checksum
	switch t := obj.(type) {
	case *s3.PutObjectInput:
		parts := strings.SplitN(unquote(b2s(dat)), "}", 2)
		if len(parts[0]) < 10 && len(parts) > 1 && len(parts[1]) > 7 {
			if parts[1][len(parts[1])-1] != '=' {
				if hexVal, err := hex.DecodeString(parts[1]); err == nil {
					parts[1] = base64.StdEncoding.EncodeToString(hexVal)
				}
			}
			switch parts[0] {
			case "{SHA", "{SHA1":
				t.ChecksumSHA1 = &parts[1]
			case "{SHA256":
				t.ChecksumSHA256 = &parts[1]
			case "{CRC32":
				t.ChecksumCRC32 = &parts[1]
			case "{CRC32C":
				t.ChecksumCRC32C = &parts[1]
			}
		} else if err := json.Unmarshal(dat, &cs); err == nil {
			t.ChecksumCRC32 = cs.ChecksumCRC32
			t.ChecksumCRC32C = cs.ChecksumCRC32C
			t.ChecksumSHA1 = cs.ChecksumSHA1
			t.ChecksumSHA256 = cs.ChecksumSHA256
		}

		if t.ChecksumSHA256 != nil {
			t.ChecksumAlgorithm = types.ChecksumAlgorithmSha256
		} else if t.ChecksumSHA1 != nil {
			t.ChecksumAlgorithm = types.ChecksumAlgorithmSha1
		} else if t.ChecksumCRC32C != nil {
			t.ChecksumAlgorithm = types.ChecksumAlgorithmCrc32c
		} else if t.ChecksumCRC32 != nil {
			t.ChecksumAlgorithm = types.ChecksumAlgorithmCrc32
		}
	}
}
