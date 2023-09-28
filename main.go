package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/valyala/fasthttp"
)

var (
	//credentials                    aws.Credentials
	signer                                           *v4.Signer
	bucketName                                       string
	uploadHeader                                     string
	debug                                            bool
	Version                                          string
	s3Client                                         *s3.Client
	directoryIndex, directoryHeader, directoryFooter []string
)

func main() {
	// Bucket configuration
	fmt.Println("Bucket-HTTP-Proxy", Version, "(github.com/pschou/bucket-http-proxy)")
	fmt.Println("Environment variables:")
	bucketName = Env("BUCKET_NAME", "my-bucket")
	//region := Env("BUCKET_REGION", "my-region")

	// Service configuration
	listenAddr := Env("LISTEN", ":8080")
	refreshTime, err := time.ParseDuration(Env("REFRESH", "20m"))
	if err != nil {
		fmt.Println(err)
		return
	}
	directoryIndex = strings.Fields(Env("DIRECTORY_INDEX", ""))
	directoryHeader = strings.Fields(Env("DIRECTORY_HEADER", ""))
	directoryFooter = strings.Fields(Env("DIRECTORY_FOOTER", ""))
	uploadHeader = Env("MODIFY_ALLOW_HEADER", "")

	// Turn on or off debugging
	debug = Env("DEBUG", "false") != "false"

	getConfig := func() error {
		sdkConfig, err := config.LoadDefaultConfig(context.TODO())
		if err != nil {
			log.Println("Could not load default config,", err)
			return err
		}

		imdsClient := imds.NewFromConfig(sdkConfig)
		gro, err := imdsClient.GetRegion(context.TODO(), &imds.GetRegionInput{})
		if err != nil {
			log.Println("Could not get region property,", err)
			return err
		}

		sdkConfig.Region = gro.Region
		s3Client = s3.NewFromConfig(sdkConfig)
		//fmt.Printf("config: %#v\n\n", sdkConfig)

		return nil
	}

	fmt.Println("Testing call to AWS...")
	if err := getConfig(); err != nil {
		os.Exit(1)
	}
	fmt.Println("Success!")

	/*
		result, err := s3Client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
		if err != nil {
			log.Fatalf("Failed to list buckets: %v\n", err)
		}

		if debug {
			fmt.Printf("result: %#v\n", result.ResultMetadata)
		}
		for _, bucket := range result.Buckets {
			fmt.Println("bucket: ", *bucket.Name)
		}*/

	go func() {
		// Refresh credentials every 20 minutes to ensure low latency on requests
		// and recovery should the server not have a policy assigned to it yet.
		for {
			if debug {
				log.Printf("creds %#v\n", s3Client)
			}
			time.Sleep(refreshTime)
			getConfig()
		}
	}()

	// Create custom server.
	s := &fasthttp.Server{
		Handler: handler,

		// Every response will contain 'Server: My super server' header.
		Name: "Bucket-HTTP-Proxy (github.com/pschou/bucket-http-proxy)",

		// Turn on upload streaming
		StreamRequestBody: true,
	}
	log.Printf("Listening for HTTP connections on %s", listenAddr)
	err = s.ListenAndServe(listenAddr)
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
