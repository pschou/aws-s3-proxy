package main

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// UploadFile reads from a file and puts the data into an object in a bucket.
func UploadFile(objectKey string, body io.Reader) (err error) {
	_, err = s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: &bucketName,
		Key:    &objectKey,
		Body:   body,
	})
	return
}
