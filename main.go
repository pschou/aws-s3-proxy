package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var (
	//credentials                    aws.Credentials
	signer                                           *v4.Signer
	bucketName                                       string
	uploadHeader                                     string
	debug                                            bool
	version                                          string
	s3Client                                         *s3.Client
	directoryIndex, directoryHeader, directoryFooter []string
)

func main() {
	// Bucket configuration
	fmt.Println("Bucket-HTTP-Proxy", version, "(https://github.com/pschou/bucket-http-proxy)")
	fmt.Println("Environment variables:")
	bucketName = Env("BUCKET_NAME", "my-bucket")
	region := Env("BUCKET_REGION", "my-region")

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

	// Load CACert for upstream verification
	if caFile := Env("CACERT", ""); len(caFile) > 0 {
		// Load CA cert
		caCert, err := ioutil.ReadFile(caFile)
		if err != nil {
			log.Fatal(err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		// Setup HTTPS client
		tlsConfig := &tls.Config{
			RootCAs: caCertPool,
		}
		http.DefaultClient.Transport = &http.Transport{TLSClientConfig: tlsConfig}
	}

	// Turn on or off debugging
	debug = Env("DEBUG", "false") != "false"

	{
		sdkConfig, err := config.LoadDefaultConfig(context.TODO())
		if err != nil {
			log.Fatalf("Could not load default config, %v", err)
		}
		//fmt.Printf("config: %#v\n\n", sdkConfig)
		sdkConfig.Region = region

		s3Client = s3.NewFromConfig(sdkConfig)
		//fmt.Printf("s3client: %#v\n\n", s3Client)
	}

	//UploadFile(bucketURL, "test123.txt", []byte("blah"))

	result, err := s3Client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
	if err != nil {
		log.Fatalf("Failed to list buckets: %v\n", err)
	}

	if debug {
		fmt.Printf("result: %#v\n", result.ResultMetadata)
	}
	/*for _, bucket := range result.Buckets {
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
			sdkConfig, err := config.LoadDefaultConfig(context.TODO())
			if err != nil {
				log.Printf("Could not load default config, %v", err)
				continue
			}
			//fmt.Printf("config: %#v\n\n", sdkConfig)
			sdkConfig.Region = region

			s3Client = s3.NewFromConfig(sdkConfig)
		}
	}()

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

/*func GetRegion() string {
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		return ""
		//log.Fatalf("Unable to load SDK config, %v", err)
	}

	md_svc := ec2metadata.New(cfg)

	if !md_svc.Available() {
		return ""
		//log.Fatalf("Metadata service cannot be reached.  Are you on an EC2/ECS/Lambda machine?")
	}

	region, err := md_svc.Region()
	if err != nil {
		return ""
		//log.Fatalf("Could not determine region")
	}

	return region
}*/
