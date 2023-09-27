package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/araddon/dateparse"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/valyala/fasthttp"
)

func handler(ctx *fasthttp.RequestCtx) {
	if debug {
		log.Println("Got request:", ctx)
	}

	isPrivileged := !(len(uploadHeader) == 0 || len(ctx.Request.Header.Peek(uploadHeader)) == 0)

	uri := strings.TrimPrefix(b2s(ctx.URI().Path()), "/")
	method := b2s(ctx.Method())
	var err error

	switch {
	case isPrivileged && method == "PUT":
		if isPrivileged {
			ctx.Response.Header.Set("Allow", "GET, PUT, POST, HEAD")
		} else {
			ctx.Response.Header.Set("Allow", "GET, HEAD")
		}

		switch strings.ToLower(b2s(ctx.Request.Header.Peek("Action"))) {
		case "delete":
			_, err = s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
				Bucket: &bucketName,
				Key:    &uri,
			})
			if err == nil {
				ctx.SetStatusCode(fasthttp.StatusGone)
			} else {
				ctx.Error(err.Error(), fasthttp.StatusLocked)
			}

		case "copy":
			dst := strings.TrimPrefix(b2s(ctx.Request.Header.Peek("Destination")), "/")
			_, err = s3Client.CopyObject(context.TODO(), &s3.CopyObjectInput{
				Bucket:     &bucketName,
				CopySource: &uri,
				Key:        &dst,
			})
			if err == nil {
				ctx.SetStatusCode(fasthttp.StatusCreated)
			} else {
				ctx.Error(err.Error(), fasthttp.StatusLocked)
			}

		case "move":
			dst := strings.TrimPrefix(b2s(ctx.Request.Header.Peek("Destination")), "/")
			_, err = s3Client.CopyObject(context.TODO(), &s3.CopyObjectInput{
				Bucket:     &bucketName,
				CopySource: &uri,
				Key:        &dst,
			})
			if err != nil {
				ctx.Error(err.Error(), fasthttp.StatusLocked)
				return
			}

			_, err = s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
				Bucket: &bucketName,
				Key:    &uri,
			})
			if err == nil {
				ctx.SetStatusCode(fasthttp.StatusGone)
			} else {
				ctx.Error(err.Error(), fasthttp.StatusLocked)
			}

		case "tea":
			ctx.SetStatusCode(fasthttp.StatusTeapot)
			ctx.Response.Header.Set("Version", Version)

		default:
			ctx.SetStatusCode(fasthttp.StatusNotImplemented)
		}
		return

	case isPrivileged && method == "POST":
		contentLength, _ := strconv.Atoi(b2s(ctx.Request.Header.Peek("Content-Length")))
		ContentType := b2s(ctx.Request.Header.Peek("Content-Type"))
		switch ContentType {
		case "application/x-www-form-urlencoded", "":
			// Set some sane defaults in case the file has been uploaded with the wrong type
			ContentType = getMime(uri)
		}

		var body io.Reader
		if contentLength > 0 {
			body = ctx.RequestBodyStream()
		} else {
			body = bytes.NewReader([]byte{})
		}

		inputObj := &s3.PutObjectInput{
			Bucket:        &bucketName,
			ContentLength: int64(contentLength),
			ContentType:   &ContentType,
			Key:           &uri,
			Body:          body,
			Metadata:      make(map[string]string),
		}

		if d := ctx.Request.Header.Peek("Content-Date"); len(d) != 0 {
			if t, err := dateparse.ParseAny(b2s(d)); err == nil {
				inputObj.Metadata["date"] = t.Format(time.DateTime)
			}
		}

		//var hasChecksum bool
		if cs := ctx.Request.Header.Peek("Checksum"); len(cs) != 0 {
			//hasChecksum = true
			unmarshalChecksum(cs, inputObj)
		}

		// If no checksum algorithm is specified, default to SHA256
		if len(inputObj.ChecksumAlgorithm) == 0 {
			inputObj.ChecksumAlgorithm = types.ChecksumAlgorithmSha256
		}

		var result *s3.PutObjectOutput
		result, err = s3Client.PutObject(context.TODO(), inputObj)

		if err == nil {
			/*if hasChecksum {
				var checksumMatch bool
				if inputObj.ChecksumSHA256 != nil && result.ChecksumSHA256 != nil {
					checksumMatch = *inputObj.ChecksumSHA256 == *result.ChecksumSHA256
				} else if inputObj.ChecksumSHA1 != nil && result.ChecksumSHA1 != nil {
					checksumMatch = *inputObj.ChecksumSHA1 == *result.ChecksumSHA1
				} else if inputObj.ChecksumCRC32 != nil && result.ChecksumCRC32 != nil {
					checksumMatch = *inputObj.ChecksumCRC32 == *result.ChecksumCRC32
				} else if inputObj.ChecksumCRC32C != nil && result.ChecksumCRC32C != nil {
					checksumMatch = *inputObj.ChecksumCRC32C == *result.ChecksumCRC32C
				}
				if !checksumMatch {
					ctx.SetStatusCode(fasthttp.StatusExpectationFailed)
					return
				}
			}*/
			ctx.SetStatusCode(fasthttp.StatusCreated)
		} else {
			ctx.Error(err.Error(), fasthttp.StatusExpectationFailed)
		}
		if debug {
			log.Printf("Upload result: %#v  err: %v\n", result, err)
			//	log.Printf("Middleware: %v \n", result.ResultMetadata)
		}
		return

	case method == "HEAD":
		var obj *s3.HeadObjectOutput
		obj, err = s3Client.HeadObject(context.TODO(), &s3.HeadObjectInput{
			Bucket:       &bucketName,
			Key:          &uri,
			ChecksumMode: types.ChecksumModeEnabled,
			//		ObjectAttributes: []types.ObjectAttributes{types.ObjectAttributesEtag,
			//			types.ObjectAttributesChecksum, types.ObjectAttributesStorageClass,
			//			types.ObjectAttributesObjectSize},
		})
		if debug {
			log.Printf("Got object: %#v  with err %v", obj, err)
		}
		if err == nil {
			ctx.Response.Header.Set("Content-Length", fmt.Sprintf("%d", obj.ContentLength))

			if d, ok := obj.Metadata["date"]; ok {
				if t, err := time.Parse(time.DateTime, d); err == nil {
					ctx.Response.Header.Set("Last-Modified", t.Format(time.RFC1123))
					obj.LastModified = nil // Prevent Last-Modified from being sent twice
				}
			}
			if obj.LastModified != nil {
				ctx.Response.Header.Set("Last-Modified", (*obj.LastModified).UTC().Format(time.RFC1123))
			}

			// Set the Content type from the mime values
			ctx.Response.Header.Set("Content-Type", getMime(uri))

			if cs := encodeChecksum(obj); cs != "" {
				ctx.Response.Header.Set("ETag", fmt.Sprintf("%q", cs))
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
			Bucket:       &bucketName,
			Key:          &uri,
			ChecksumMode: types.ChecksumModeEnabled,
		})
		if debug {
			log.Printf("Got object: %#v  with err %v", obj, err)
		}
		if err == nil {
			// Found the file, so serve it out!
			ctx.Response.Header.SetContentLength(int(obj.ContentLength))

			if d, ok := obj.Metadata["date"]; ok {
				if t, err := time.Parse(time.DateTime, d); err == nil {
					ctx.Response.Header.Set("Last-Modified", t.Format(time.RFC1123))
					obj.LastModified = nil // Prevent Last-Modified from being sent twice
				}
			}
			if obj.LastModified != nil {
				ctx.Response.Header.Set("Last-Modified", (*obj.LastModified).UTC().Format(time.RFC1123))
			}
			// Set the Content type from the mime values
			ctx.Response.Header.Set("Content-Type", getMime(uri))

			if cs := encodeChecksum(obj); cs != "" {
				ctx.Response.Header.Set("ETag", fmt.Sprintf("%q", cs))
			}

			ctx.SetBodyStream(obj.Body, int(obj.ContentLength))
		} else if isDir(uri + "/") {
			if debug {
				log.Printf("Error finding %s so redirecting to /%s/, err: %v\n", uri, uri, err)
			}
			ctx.Redirect("/"+uri+"/", fasthttp.StatusTemporaryRedirect)
		} else {
			if debug {
				log.Printf("Error finding %s, err: %v\n", uri, err)
			}
			ctx.Error("404 file not found: "+uri, fasthttp.StatusNotFound)
		}
		//return

	default:
		ctx.Error("405 method not allowed: "+method, fasthttp.StatusMethodNotAllowed)
	}
	//if err != nil {
	//	ctx.Error(err.Error(), fasthttp.StatusInternalServerError)
	//}
}
