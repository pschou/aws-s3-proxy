package main

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var typeFiles = []string{
	"/etc/mime.types",
	"/etc/apache2/mime.types",
	"/etc/apache/mime.types",
	"/etc/httpd/conf/mime.types",
	"./mime.types",
}
var (
	mime = make(map[string]string)
)

func getMime(uri string) (ContentType string) {
	ext := strings.ToLower(filepath.Ext(uri))
	var ok bool
	if ContentType, ok = mime[ext]; ok {
		return
	}
	return "application/octet-stream"
}

func loadMimeFile(filename string) {
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()

	var count int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) <= 1 || fields[0][0] == '#' {
			continue
		}
		mimeType := fields[0]
		for _, ext := range fields[1:] {
			if ext[0] == '#' {
				break
			}
			ext = strings.TrimSuffix(ext, ";")
			mime["."+ext] = mimeType
			count++
		}
	}
	if count > 0 {
		log.Println(count, "mime types loaded from", filename)
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
}

func init() {
	for _, filename := range typeFiles {
		loadMimeFile(filename)
	}
}
