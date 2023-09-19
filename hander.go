package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"time"
)

const EmptyStringSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func handler(w http.ResponseWriter, r *http.Request) {
	r.Body.Close()
	w.Header().Set("Server", "S3-HTTP-Proxy (github.com/pschou/s3-http-proxy)")

	if credentials.Expired() {
		http.Error(w, "Error with credentials", http.StatusInternalServerError)
		return
	}

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
	req.Header.Set("X-Amz-Content-SHA256", EmptyStringSHA256)
	req.Header.Set("User-Agent", "S3-HTTP-Proxy (github.com/pschou/s3-http-proxy)")
	for _, h := range []string{"Range", "If-Range", "If-Unmodified-Since", "If-Modified-Since",
		"If-None-Match", "If-Match"} {
		if hdr := r.Header.Get(h); len(hdr) > 0 {
			req.Header.Set(h, hdr)
		}
	}
	/*
		credentials, err := appCreds.Retrieve(context.TODO())
		if err != nil {
			log.Printf("Error refreshing credentials: %v\n", err)
			http.Error(w, "Error refreshing credentials", http.StatusInternalServerError)
			return
		}
	*/

	signer.SignHTTP(context.TODO(), credentials, req, EmptyStringSHA256,
		"s3", region, requestTime)

	if debug {
		log.Printf("Request built: %#v", req)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	for _, h := range []string{"Content-Type", "Content-Length", "Content-Encoding",
		"Last-Modified", "Date", "ETag", "Accept-Ranges", "Range", "Content-Range"} {
		if hdr := resp.Header.Get(h); len(hdr) > 0 {
			w.Header().Set(h, hdr)
		}
	}
	n, err := io.Copy(w, resp.Body)

	if debug {
		if err == nil {
			log.Printf("Got %d bytes\n", n)
		} else {
			log.Printf("Got %d bytes with error: %s\n", n, err)
		}
	}
}
