package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const EmptyStringSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func handler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	w.Header().Set("Server", "Bucket-HTTP-Proxy (github.com/pschou/bucket-http-proxy)")

	if debug {
		log.Printf("Got request: %#v", r)
	}

	uri := strings.TrimPrefix(r.URL.Path, "/")

	var err error
	switch r.Method {
	case "DELETE":
		if uploadHeader == "" || r.Header.Get(uploadHeader) == "" {
			http.Error(w, "Only GET is supported", http.StatusBadRequest)
			return
		}

		_, err = s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
			Bucket: &bucketName,
			Key:    &uri,
		})

	case "PUT":
		if uploadHeader == "" || r.Header.Get(uploadHeader) == "" {
			http.Error(w, "Only GET is supported", http.StatusBadRequest)
			return
		}
		contentLength, _ := strconv.Atoi(r.Header.Get("Content-Length"))
		ContentType := r.Header.Get("Content-Type")
		switch ContentType {
		case "application/x-www-form-urlencoded", "":
			switch filepath.Ext(uri) {
			case "html", "htm":
				ContentType = "text/html;charset=UTF-8"
			case "text", "txt", "go", "conf", "repo":
				ContentType = "text/text;charset=UTF-8"
			default:
				ContentType = "application/octet-stream"
			}
		}
		/*buf := &bytes.Buffer{}
		n, _ := io.Copy(buf, r.Body)
		if debug {
			log.Println("read", n, "bytes")
		}*/
		_, err = s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket:        &bucketName,
			ContentLength: int64(contentLength),
			ContentType:   &ContentType,
			Key:           &uri,
			Body:          r.Body,
		})

	case "GET":
		if uri == "" || strings.HasSuffix(uri, "/") {
			if r.Header.Get("Accept") == "list/json" {
				jsonList(uri, w)
				return
			}
			for _, index := range directoryIndex {
				if testPath := path.Join(uri, index); isFile(testPath) {
					http.Redirect(w, r, "/"+testPath, http.StatusTemporaryRedirect)
					return
				}
			}
			dirList(uri, w)
			return
		}
		var obj *s3.GetObjectOutput
		obj, err = s3Client.GetObject(context.TODO(), &s3.GetObjectInput{
			Bucket: &bucketName,
			Key:    &uri,
		})
		if debug {
			log.Printf("Got object: %#v  with err %v", obj, err)
		}
		if err == nil {
			// Found the file, so serve it out!
			defer obj.Body.Close()
			w.Header().Set("Content-Length", fmt.Sprintf("%d", obj.ContentLength))
			if obj.LastModified != nil {
				w.Header().Set("Last-Modified", (*obj.LastModified).UTC().Format(time.RFC1123))
			}
			if obj.ContentType != nil {
				w.Header().Set("Content-Type", *obj.ContentType)
			}
			n, err := io.Copy(w, obj.Body)

			if debug {
				if err == nil {
					log.Printf("Got %d bytes\n", n)
				} else {
					log.Printf("Got %d bytes with error: %s\n", n, err)
				}
			}
		} else if isDir(uri) {
			if debug {
				log.Printf("Error finding %s so redirecting to /%s/, err: %v\n", uri, uri, err)
			}
			http.Redirect(w, r, "/"+uri+"/", http.StatusTemporaryRedirect)
			return
		} else {
			if debug {
				log.Printf("Error finding %s, err: %v\n", uri, err)
			}
			http.Error(w, "404 file not found: "+uri, http.StatusNotFound)
			return
		}

	default:
		http.Error(w, "Only GET is supported", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	/*

		if debug {
			log.Printf("Building request: %#v", bucketURL+r.URL.Path+"?max-keys=1")
		}
		req, err := http.NewRequest(r.Method, bucketURL+r.URL.Path+"?max-keys=1", nil)
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
			credentials, err := appCreds.Retrieve(context.TODO())
			if err != nil {
				log.Printf("Error refreshing credentials: %v\n", err)
				http.Error(w, "Error refreshing credentials", http.StatusInternalServerError)
				return
			}

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
		}*/
}
