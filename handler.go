package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/valyala/fasthttp"
)

func handler(ctx *fasthttp.RequestCtx) {
	if debug {
		log.Println("Got request:", ctx)
	}

	isPrivileged := !(len(uploadHeader) == 0 || len(ctx.Request.Header.Peek(uploadHeader)) == 0)
	if isPrivileged {
		ctx.Response.Header.Set("Allow", "GET, PUT, DELETE, HEAD")
	} else {
		ctx.Response.Header.Set("Allow", "GET, HEAD")
	}

	uri := strings.TrimPrefix(b2s(ctx.URI().Path()), "/")
	method := b2s(ctx.Method())
	var err error

	switch {
	case isPrivileged && method == "DELETE":
		_, err = s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
			Bucket: &bucketName,
			Key:    &uri,
		})

	case isPrivileged && method == "PUT":
		contentLength, _ := strconv.Atoi(b2s(ctx.Request.Header.Peek("Content-Length")))
		ContentType := b2s(ctx.Request.Header.Peek("Content-Type"))
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

		var body io.Reader
		if contentLength > 0 {
			body = ctx.RequestBodyStream()
		} else {
			body = bytes.NewReader([]byte{})
		}

		var result *s3.PutObjectOutput
		result, err = s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket:        &bucketName,
			ContentLength: int64(contentLength),
			ContentType:   &ContentType,
			Key:           &uri,
			Body:          body,
		})

		if debug {
			log.Printf("Upload result: %#v  err: %v", result, err)
		}

	case method == "HEAD":
		var obj *s3.GetObjectAttributesOutput
		obj, err = s3Client.GetObjectAttributes(context.TODO(), &s3.GetObjectAttributesInput{
			Bucket: &bucketName,
			Key:    &uri,
			ObjectAttributes: []types.ObjectAttributes{types.ObjectAttributesEtag,
				types.ObjectAttributesChecksum, types.ObjectAttributesStorageClass,
				types.ObjectAttributesObjectSize},
		})
		if debug {
			log.Printf("Got object: %#v  with err %v", obj, err)
		}
		if err == nil {
			ctx.Response.Header.Set("Content-Length", fmt.Sprintf("%d", obj.ObjectSize))
			if obj.LastModified != nil {
				ctx.Response.Header.Set("Last-Modified", (*obj.LastModified).UTC().Format(time.RFC1123))
			}
			if obj.ETag != nil {
				ctx.Response.Header.Set("ETag", unquote(*obj.ETag))
			}
			if cs := getChecksum(obj); cs != "" {
				ctx.Response.Header.Set("Checksum", cs)
			}
			if string(obj.StorageClass) == "" {
				ctx.Response.Header.Set("Storage-Class", string(types.StorageClassStandard))
			} else {
				ctx.Response.Header.Set("Storage-Class", string(obj.StorageClass))
			}
		}

	case method == "GET":

		if len(uri) == 0 || uri[len(uri)-1] == '/' {
			if strings.SplitN(b2s(ctx.Request.Header.Peek("Accept")), ",", 2)[0] == "list/json" {
				jsonList(uri, ctx)
				return
			}
			var found bool
			for _, index := range directoryIndex {
				if testPath := path.Join(uri, index); isFile(testPath) {
					uri = testPath
					found = true
					break
				}
			}

			if !found {
				var header, footer string

				for _, test := range directoryHeader {
					if len(test) > 0 && test[0] == '/' && isFile(test[1:]) {
						header = test[1:]
						break
					}
					if testPath := path.Join(uri, test); isFile(testPath) {
						header = testPath
						break
					}
				}

				for _, test := range directoryFooter {
					if len(test) > 0 && test[0] == '/' && isFile(test[1:]) {
						footer = test[1:]
						break
					}
					if testPath := path.Join(uri, test); isFile(testPath) {
						footer = testPath
						break
					}
				}

				if debug {
					log.Println("calling dirlist", uri, ctx, header, footer)
				}
				dirList(uri, ctx, header, footer)
				return
			}
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
			ctx.Response.Header.Set("Content-Length", fmt.Sprintf("%d", obj.ContentLength))
			if obj.LastModified != nil {
				ctx.Response.Header.Set("Last-Modified", (*obj.LastModified).UTC().Format(time.RFC1123))
			}
			if obj.ContentType != nil {
				ctx.Response.Header.Set("Content-Type", *obj.ContentType)
			}
			if obj.ETag != nil {
				ctx.Response.Header.Set("ETag", unquote(*obj.ETag))
			}

			if cs := getChecksum(obj); cs != "" {
				ctx.Response.Header.Set("Checksum", cs)
			}
			if string(obj.StorageClass) == "" {
				ctx.Response.Header.Set("Storage-Class", string(types.StorageClassStandard))
			} else {
				ctx.Response.Header.Set("Storage-Class", string(obj.StorageClass))
			}

			ctx.SetBodyStream(obj.Body, int(obj.ContentLength))
		} else if isDir(uri) {
			if debug {
				log.Printf("Error finding %s so redirecting to /%s/, err: %v\n", uri, uri, err)
			}
			ctx.Redirect("/"+uri+"/", fasthttp.StatusTemporaryRedirect)
			return
		} else {
			if debug {
				log.Printf("Error finding %s, err: %v\n", uri, err)
			}
			ctx.Error("404 file not found: "+uri, fasthttp.StatusNotFound)
			return
		}

	default:
		ctx.Error("405 method not allowed: "+method, fasthttp.StatusMethodNotAllowed)
		return
	}
	if err != nil {
		ctx.Error(err.Error(), fasthttp.StatusInternalServerError)
	}
}
