package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"time"
)

func handler(w http.ResponseWriter, r *http.Request) {
	r.Body.Close()
	w.Header().Set("Server", "S3-Proxy (github.com/pschou/aws-s3-proxy)")

	if debug {
		log.Printf("Got request: %#v", r)
	}
	switch r.Method {
	case "GET", "HEAD":
	default:
		http.Error(w, "Only GET and HEAD are supported", http.StatusBadRequest)
		return
	}

	if debug {
		log.Printf("Building request: %#v", bucketURL+r.URL.Path)
	}
	req, err := http.NewRequest(r.Method, bucketURL+r.URL.Path, nil)
	requestTime := time.Now()
	req.Header.Set("X-Amz-Date", requestTime.Format("20060102T150405Z"))
	req.Header.Set("X-Amz-Content-SHA256", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	req.Header.Set("User-Agent", "S3-Proxy (github.com/pschou/aws-s3-proxy)")
	signer.SignHTTP(context.TODO(), credentials, req, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "s3", region, requestTime)

	if debug {
		log.Printf("Request built: %#v", req)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	for _, h := range []string{"Content-Type", "Content-Length", "Content-Encoding", "Last-Modified", "Date", "ETag"} {
		if hdr := resp.Header.Get(h); len(hdr) > 0 {
			w.Header().Set("Content-Type", hdr)
		}
	}
	n, err := io.Copy(w, resp.Body)

	if debug {
		log.Printf("Got %d bytes with error: %s\n", n, err)
	}
}
