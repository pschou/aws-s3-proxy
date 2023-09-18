# S3-Proxy

The S3-Proxy is a light weight HTTP request signer for enabling the ability to
serve out S3 buckets securely.  The requests are stripped of all headers and
signed when sent to the backend bucket for serving resources.

To run the server on an EC2 instance, call the program like this:

```
./s3-proxy
```
