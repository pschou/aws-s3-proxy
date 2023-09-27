# Bucket-HTTP-Proxy

This bucket proxy is a lightweight HTTP proxy that can serve out S3 buckets
securely from non-public readable buckets.  All requests are sanitized
(stripped of all non-relevant headers) before the requests are sent to the
backend bucket for serving up resources.  All of this is done using the policy
set on the instance.

This utility was designed to provide ways and means to:

- A microservice which is designed to run behind a reverse proxy and provide real time view of the buckets.

- Control access to a bucket by restricting access to an EC2 instance IAM policy (Don't make your bucket public!).

- Optionally provides a website front end by presenting a defined index page (like index.html).

- If no index is found or provided, a directory listing is presented with the name, date, size, and checksum.

- Allow specific users to upload to a bucket by using a PUT command (if the policy is configured to allow this and this container is ran behind a revers proxy).

- API JSON endpoint to query for the list of files (by setting the accept header to "list/json" in the GET request for a folder).

- When configuring your reverse proxy, such as Nginx, one can set the cache size and time to prevent sending back requests to the bucket backend.

- This microservice remains small and lightweight in memory and CPU, as lightweight is the goal this tool does not provide any content caching.

- Written using golang with automatic scaling of threads to handle increase loads.  If concurrency is a concern, consider using a reverse proxy in front to limit connections.

## Variables

BUCKET_NAME - Name of the bucket, ex: "my-bucket"

LISTEN - Bind address to listen, ex: "1.2.3.4:8080"

REFRESH - Time between pulling new keys, ex: "10m"

DIRECTORY_INDEX - List of files to use as an index if found, ex: "index.html index.htm"

DIRECTORY_HEADER - File to use as a header when doing automatic directories, ex: ".HEADER.html"

DIRECTORY_FOOTER - File to use as a header when doing automatic directories, ex: ".FOOTER.html"

MODIFY_ALLOW_HEADER - Header to look for to allow PUT and DELETE methods, the header just has to be set to a non-empty string value, ex: "X-USER"

DEBUG - Turn on verbosity, ex: "true"

## Running from the command line

To run the server on an EC2 instance, call the program like this using
environment variables.  Some defaults may look like this:

```
$ BUCKET_NAME=repo-test MODIFY_ALLOW_HEADER=X-USER ./bucket-http-proxy
2023/09/27 02:15:23 112 mime types loaded from ./mime.types
Bucket-HTTP-Proxy 0.1.20230926.2213 (github.com/pschou/bucket-http-proxy)
Environment variables:
  BUCKET_NAME="repo-test"
  LISTEN=":8080" (default)
  REFRESH="20m" (default)
  DIRECTORY_INDEX="" (default)
  DIRECTORY_HEADER="" (default)
  DIRECTORY_FOOTER="" (default)
  MODIFY_ALLOW_HEADER="X-USER"
  DEBUG="false" (default)
2023/09/27 02:15:23 Listening for HTTP connections on :8080
```

## Running using a docker-compose file

See the example docker-compose file in the git repository as it has several settings needed to ensure that the container is able to access the rest endpoint for getting the needed server keys.

```
$ docker-compose up -d --build
Network test-user_default                Created
Container test-user-nginx-1              Started
Container test-user-bucket-http-proxy-1  Started   
```

## Verify operations

After this is up and running, one may call a curl command like this:

```
$ curl -s localhost:8080/
<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 3.2 Final//EN">
<html>
 <head>
  <title>Index of /</title>
 </head>
 <body>
<h1>Index of /</h1>
<table><tr><th>Name</th><th>Last modified</th><th>Size</th></tr><tr><th colspan="3"><hr></th></tr>
 <tr><td><a href="test/">test/</a></td><td align="right">&nbsp; 2023-09-18 23:36:11</td><td align="right">-</td></tr>
...
```

## Rest endpoint

To verify the REST API call to list the contents:
```
$ curl -s -H "Accept: list/json" localhost:8080/
[{"Name":"a/","Size":0}
...
]
```
