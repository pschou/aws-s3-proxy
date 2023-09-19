# S3-HTTP-Proxy

The S3-Proxy is a light weight HTTP request signer for enabling the ability to
serve out S3 buckets securely.  The requests are stripped of all headers and
signed when sent to the backend bucket for serving resources.

To run the server on an EC2 instance, call the program like this:

```
./s3-proxy
```

Setting some default parameter may look like this:
```
$ AWS_REGION=us-east-1 AWS_BUCKET_URL=https://s3-http-proxy-test.s3.amazonaws.com ./s3-proxy
Environment variables:
  AWS_BUCKET_URL="https://s3-http-proxy-test.s3.amazonaws.com"
  AWS_REGION="us-east-1"
  LISTEN=":8080" (default)
  DEBUG="false" (default)
  REFRESH="20m" (default)
Listening for HTTP connections on :8080
```

After this is up and running, one may call a http command like this:
```
curl localhost:8080
```
