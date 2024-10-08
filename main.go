package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/valyala/fasthttp"
)

var (
	//credentials                    aws.Credentials
	bucketName                                       string
	uploadHeader                                     string
	debug                                            bool
	Version                                          string
	s3Client                                         *s3.Client
	directoryIndex, directoryHeader, directoryFooter []string
	imdsClient                                       *imds.Client
)

func getIMDS() (region, ARN, ID string) {
	/*sdkConfig, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatal("Could not load default config,", err)
	}*/

	imdsClient = imds.New(imds.Options{})
	gro, err := imdsClient.GetRegion(context.TODO(), &imds.GetRegionInput{})
	if err != nil {
		log.Fatal("Could not get region property,", err)
	}

	iam, err := imdsClient.GetIAMInfo(context.TODO(), &imds.GetIAMInfoInput{})
	if err != nil {
		log.Fatal("Could not get IAM property,", err)
	}

	return gro.Region, iam.IAMInfo.InstanceProfileArn, iam.IAMInfo.InstanceProfileID
}

func main() {
	// Bucket configuration
	fmt.Println("Bucket-HTTP-Proxy", Version, "(github.com/pschou/bucket-http-proxy)")
	fmt.Println("Environment variables:")
	bucketName = Env("BUCKET_NAME", "my-bucket", "The name of the bucket to be served")

	// Service configuration
	listenAddr := Env("LISTEN", ":8080", "The listening port to serve the contents of the bucket from")
	refreshTime, err := time.ParseDuration(Env("REFRESH", "20m", "The refresh interval for grabbing new AMI credentials"))
	if err != nil {
		fmt.Println(err)
		return
	}
	directoryIndex = strings.Fields(Env("DIRECTORY_INDEX", "", "Which file to use for a directory index, for example: \"index.html index.htm\""))
	directoryHeader = strings.Fields(Env("DIRECTORY_HEADER", "", "If an html file is specified it will be prepended to the directory listing, for example: \"header.html\""))
	directoryFooter = strings.Fields(Env("DIRECTORY_FOOTER", "", "Like header but appended to the directory listing, for example: \"footer.html\" or \"/.footer.html\" for an absolute path"))
	uploadHeader = Env("MODIFY_ALLOW_HEADER", "", "Look for this header in the request to allow bucket write permissions")
	Env("SSL_CERT_FILE", "", "Override the system CA chain default with this CA file")

	// Turn on or off debugging
	debug = Env("DEBUG", "false", "Turn on debugging output for evaluating what is happening") != "false"

	fmt.Println("EC2 Environment:")
	region, arn, id := getIMDS()
	fmt.Println("  AWS_REGION:", region)
	fmt.Println("  IMDS_ARN:", arn)
	fmt.Println("  IMDS_ID:", id)

	getConfig := func() error {
		// Get a credential provider from the configured role attached to the currently running EC2 instance
		provider := ec2rolecreds.New(func(o *ec2rolecreds.Options) {
			o.Client = imdsClient
		})

		// Construct a client, wrap the provider in a cache, and supply the region for the desired service
		s3Client = s3.New(s3.Options{
			Credentials: aws.NewCredentialsCache(provider),
			Region:      region,
		})
		//fmt.Printf("config: %#v\n\n", sdkConfig)

		return nil
	}

	fmt.Println("Testing call to AWS...")
	if err := getConfig(); err != nil {
		log.Fatal("Error getting config:", err)
	}
	buildDirList()
	if bucketDirError != nil {
		log.Fatal("Error listing bucket:", bucketDirError)
	}
	fmt.Println("Success!  Found", bucketDir.count, "objects using", bucketDir.size)

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

func Env(env, def, usage string) string {
	fmt.Println("  #", usage)
	if e := os.Getenv(env); len(e) > 0 {
		fmt.Printf("  %s=%q\n", usage, env, e)
		return e
	}
	fmt.Printf("  %s=%q (default)\n", env, def)
	return def
}
