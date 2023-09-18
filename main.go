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
	appCreds                      = aws.NewCredentialsCache(ec2rolecreds.New())
	credentials                   aws.Credentials
	signer                        *v4.Signer
	region, listenAddr, bucketURL string
	refreshInterval               time.Duration
	debug                         bool
)

func main() {
	var err error

	fmt.Println("Environment variables:")
	region = Env("AWS_REGION", "east-1")
	if refreshInterval, err = time.ParseDuration(Env("AWS_REFRESH", "20m")); err != nil {
		log.Fatal("  Invalid refresh duration")
	}
	listenAddr = Env("LISTEN", ":8080")
	bucketURL = Env("AWS_BUCKET_URL", "http://mybucket")
	debug = Env("DEBUG", "false") != "false"

	// New returns an object of a type that satisfies the aws.CredentialProvider interface
	credentials, err = appCreds.Retrieve(context.TODO())
	if err != nil {
		log.Fatalf("failed to get credentials, %v", err)
		// handle error
	}
	signer = v4.NewSigner()

	go func() {
		// Refresh credentials every 20 minutes to ensure low latency on requests
		for {
			time.Sleep(20 * time.Minute)
			if refresh, err := appCreds.Retrieve(context.TODO()); err == nil {
				credentials = refresh
			}
		}
	}()

	log.Printf("Listening for HTTP connections on %s", listenAddr)
	http.HandleFunc("/", handler)
	err = http.ListenAndServe(listenAddr, nil)
	log.Printf("Error: %s", err)
}

func Env(env, def string) string {
	if e := os.Getenv(env); len(e) > 0 {
		fmt.Printf("  %s = %q\n", env, e)
		return e
	}
	fmt.Printf("  %s = %q (default)\n", env, def)
	return def
}
