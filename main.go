package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
)

var (
	credentials                   aws.Credentials
	signer                        *v4.Signer
	region, listenAddr, bucketURL string
	debug                         bool
	version                       string
)

func main() {
	fmt.Println("S3-HTTP-Proxy", version)
	fmt.Println("Environment variables:")
	bucketURL = Env("AWS_BUCKET_URL", "http://mybucket.changeme")
	region = Env("AWS_REGION", "changeme-east-1")
	listenAddr = Env("LISTEN", ":8080")
	debug = Env("DEBUG", "false") != "false"
	refreshTime, err := time.ParseDuration(Env("REFRESH", "20m"))
	if err != nil {
		fmt.Println(err)
		return
	}

	// New returns an object of a type that satisfies the aws.CredentialProvider interface
	var appCreds = aws.NewCredentialsCache(ec2rolecreds.New())
	credentials, err = appCreds.Retrieve(context.TODO())
	if err != nil {
		log.Printf("failed to get credentials, %v", err)
	}

	go func() {
		// Refresh credentials every 20 minutes to ensure low latency on requests
		// and recovery should the server not have a policy assigned to it yet.
		for {
			if debug {
				log.Printf("creds %#v\n", credentials)
			}
			time.Sleep(refreshTime)
			appCreds.Invalidate()
			if refresh, err := appCreds.Retrieve(context.TODO()); err == nil {
				credentials = refresh
			}
		}
	}()

	signer = v4.NewSigner()

	log.Printf("Listening for HTTP connections on %s", listenAddr)
	http.HandleFunc("/", handler)
	err = http.ListenAndServe(listenAddr, nil)
	log.Printf("Error: %s", err)
}

func Env(env, def string) string {
	if e := os.Getenv(env); len(e) > 0 {
		fmt.Printf("  %s=%q\n", env, e)
		return e
	}
	fmt.Printf("  %s=%q (default)\n", env, def)
	return def
}
