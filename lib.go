package main

import (
	"encoding/json"
	"unsafe"

	"dario.cat/mergo"
	types "github.com/pschou/bucket-http-proxy/types"
)

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

func getChecksum(obj interface{}) string {
	var cs types.Checksums
	if err := mergo.Merge(&cs, obj); err != nil {
		dat, _ := json.Marshal(cs)
		return b2s(dat)
	}
	return ""
}
