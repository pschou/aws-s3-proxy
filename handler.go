package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
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
		ctx.Response.Header.Set("Cache-Control", "no-cache")

		// Parse out the Action header and parse out the first word.
		action := strings.SplitN(b2s(ctx.Request.Header.Peek("Action")), " ", 2)
		switch strings.ToLower(action[0]) {
		case "delete":
			resp, err := s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
				Bucket: &bucketName,
				Key:    &uri,
			})
			if debug {
				log.Printf("Delete %q %#v", uri, resp)
			}
			if err == nil {
				ctx.SetStatusCode(fasthttp.StatusGone)
			} else {
				ctx.Error(err.Error(), fasthttp.StatusLocked)
			}

		case "copy":
			if len(action) == 1 || len(action[1]) < 2 {
				ctx.Error(err.Error(), fasthttp.StatusExpectationFailed)
				return
			}
			src := action[1]
			if src[0] == '/' {
				src = bucketName + src
			}
			src = url.QueryEscape(src)
			if debug {
				log.Println("copy", src, "->", uri)
			}
			_, err = s3Client.CopyObject(context.TODO(), &s3.CopyObjectInput{
				Bucket:     &bucketName,
				CopySource: &src,
				Key:        &uri,
			})
			if err == nil {
				ctx.SetStatusCode(fasthttp.StatusCreated)
			} else {
				ctx.Error(err.Error(), fasthttp.StatusLocked)
			}

		case "move":
			if len(action) == 1 || len(action[1]) < 2 || action[1][0] != '/' {
				ctx.Error(err.Error(), fasthttp.StatusExpectationFailed)
				return
			}
			src := url.QueryEscape(bucketName + action[1])
			if debug {
				log.Println("move", src, "->", uri)
			}

			_, err = s3Client.CopyObject(context.TODO(), &s3.CopyObjectInput{
				Bucket:     &bucketName,
				CopySource: &src,
				Key:        &uri,
			})
			if err != nil {
				ctx.Error(err.Error(), fasthttp.StatusLocked)
				return
			}

			src = action[1][1:]
			_, err = s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
				Bucket: &bucketName,
				Key:    &src,
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

		// If a checksum header is provided, unmarshall it
		if cs := ctx.Request.Header.Peek("Checksum"); len(cs) != 0 {
			unmarshalChecksum(cs, inputObj)
			if len(inputObj.ChecksumAlgorithm) == 0 {
				if debug {
					log.Printf("Invalid checksum formatted string: %q", b2s(cs))
				}
				ctx.Error(fmt.Sprintf("Invalid checksum formatted string: %q", b2s(cs)), fasthttp.StatusExpectationFailed)
				return
			}
		}

		// If no checksum algorithm is specified, default to SHA256
		if len(inputObj.ChecksumAlgorithm) == 0 {
			inputObj.ChecksumAlgorithm = types.ChecksumAlgorithmSha256
		}

		var result *s3.PutObjectOutput
		result, err = s3Client.PutObject(context.TODO(), inputObj)

		if err == nil {
			ctx.SetStatusCode(fasthttp.StatusCreated)
		} else {
			ctx.Error(err.Error(), fasthttp.StatusExpectationFailed)
		}
		if debug {
			log.Printf("Upload result: %#v  err: %v\n", result, err)
		}
		return

	case method == "HEAD":
		if time.Now().Sub(bucketDirUpdate) > bucketTimeout {
			buildDirList()
		}

		obj, exist := bucketDir.objects[uri]
		if !exist {
			if _, exist := bucketDir.objects[uri+"/"]; exist {
				if debug {
					log.Printf("Error finding %s so redirecting to /%s/, err: %v\n", uri, uri, err)
				}
				ctx.Redirect("/"+uri+"/", fasthttp.StatusTemporaryRedirect)
			} else {
				ctx.SetStatusCode(fasthttp.StatusNotFound)
			}
			return
		}

		if obj.Time != nil {
			ctx.Response.Header.Set("Last-Modified", obj.Time.Format(time.RFC1123))
		}
		if !obj.isDir {
			if len(obj.Checksum) == 0 {
				dir, _ := path.Split(uri)
				//log.Println("get checksum", uri)
				obj.getHead(dir)
			}
			ctx.Response.Header.Set("Content-Length", fmt.Sprintf("%d", obj.Size))
			ctx.Response.Header.Set("Content-Type", getMime(uri))
			ctx.Response.Header.Set("ETag", fmt.Sprintf("%q", obj.Checksum))
		}
		return

	case method == "GET":
		// If a directory listing is asked for, handle this with one of our directory functions
		if len(uri) == 0 || uri[len(uri)-1] == '/' {
			if time.Now().Sub(bucketDirUpdate) > bucketTimeout {
				buildDirList()
			}

			// When a JSON list is requested
			if accept := strings.SplitN(b2s(ctx.Request.Header.Peek("Accept")), ",", 2); accept[0] == "list/json" {
				jsonList(uri, ctx,
					len(accept) == 2 && strings.HasPrefix(accept[1], "recursive"), // Should this be a recursive listing
				)
				return
			}

			// When a directory index is provided and is found
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
