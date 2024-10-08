package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/pschou/go-convert/bin"
	"github.com/pschou/go-sorting/numstr"
	"github.com/remeh/sizedwaitgroup"
	"github.com/valyala/fasthttp"
)

type DirItem struct {
	Name         string
	Time         *time.Time `json:",omitempty"`
	realTime     *time.Time `json:",omitempty"`
	Size         int64
	Count        int64                    `json:",omitempty"`
	eTag         string                   `json:",omitempty"`
	StorageClass types.ObjectStorageClass `json:",omitempty"`
	Checksum     string                   `json:",omitempty"`
	isDir        bool
	list         []*DirItem
}

func (d *DirItem) getHead(base string) {
	if len(d.Checksum) > 0 || len(d.Name) == 0 || d.Name[len(d.Name)-1] == '/' {
		return
	}
	name := base + d.Name
	eTag := d.eTag
	cmpTime := d.realTime

	if debug {
		log.Println("cache check", fmt.Sprintf("%q%q", name, eTag))
	}
	hashCacheMutex.Lock()
	h, ok := hashCache[fmt.Sprintf("%q%q", name, eTag)]
	hashCacheMutex.Unlock()

	if ok && h.realTime.Equal(*cmpTime) {
		if debug {
			log.Println("cache hit")
		}
		d.Checksum = h.hash
		d.Time = &h.time
		return
	} else {
		if debug {
			log.Println("cache miss", ok, *cmpTime, h)
		}
	}
	if debug {
		log.Println("calling gethash", name, eTag, *cmpTime)
	}
	obj, err := s3Client.HeadObject(context.TODO(), &s3.HeadObjectInput{
		Bucket:       &bucketName,
		Key:          &name,
		ChecksumMode: types.ChecksumModeEnabled,
	})
	if err == nil {
		var outHash string
		if d.Size > 0 {
			outHash = encodeChecksum(obj)
		}
		if debug {
			log.Println("cache save", fmt.Sprintf("%q%q", name, unquote(*obj.ETag)), outHash)
		}

		var outTime *time.Time
		if dateVal, ok := obj.Metadata["date"]; ok {
			if t, err := time.Parse(time.DateTime, dateVal); err == nil {
				outTime = &t
			} else {
				outTime = obj.LastModified
			}
		} else {
			outTime = obj.LastModified
		}

		if link, ok := obj.Metadata["link"]; ok {
			outHash = "-> " + link
		}

		hashCacheMutex.Lock()
		hashCache[fmt.Sprintf("%q%q", name, unquote(*obj.ETag))] = hashdat{time: *outTime, hash: outHash, realTime: *obj.LastModified}
		hashCacheMutex.Unlock()
		d.Time = outTime
		d.Checksum = outHash
	}
	return
}

var (
	bucketDir       Root
	bucketDirLock   sync.Mutex
	bucketDirError  error
	bucketDirUpdate time.Time
	bucketTimeout   = 15 * time.Second

	hashCache      = make(map[string]hashdat)
	hashCacheMutex sync.Mutex
)

type hashdat struct {
	time     time.Time
	realTime time.Time
	hash     string
}

type Root struct {
	objects map[string]*DirItem
	size    int64
	count   int64
}

// Use the objects map to determine if an item is a directory (either explicit or implicit)
func isDir(test string) bool {
	obj, exists := bucketDir.objects[test]
	return exists && obj.isDir
}

// Use the objects map to determine if an item is a file
func isFile(test string) bool {
	obj, exists := bucketDir.objects[test]
	return exists && !obj.isDir
}

// Walk the object map providing the list of objects in a JSON formatted reply.
func jsonList(baseDir string, ctx *fasthttp.RequestCtx, recursive bool) {
	ctx.Write([]byte("{\"/\":\n["))
	defer ctx.Write([]byte("]}"))

	encoder := json.NewEncoder(ctx)
	dirs := []string{baseDir}
	wg := sizedwaitgroup.New(8)
	objects := bucketDir.objects

	for i := 0; i < len(dirs); i++ {
		if i > 0 {
			fmt.Fprintf(ctx, "],%q:\n[", "/"+strings.TrimPrefix(dirs[i], baseDir))
		}
		curDir, ok := objects[dirs[i]]
		if !ok {
			ctx.Error("404 path not found: "+dirs[i], fasthttp.StatusNotFound)
			return
		}

		{ // Get all the metadata for this directory
			for j, c := range curDir.list {
				if len(c.Checksum) == 0 {
					wg.Add()
					go func(j int) {
						defer wg.Done()
						curDir.list[j].getHead(dirs[i])
					}(j)
				}
			}
			wg.Wait()
		}

		var pastFirst bool
		for _, c := range curDir.list {
			if recursive && len(c.Name) > 0 && c.Name[len(c.Name)-1] == '/' {
				dirs = append(dirs, dirs[i]+c.Name)
			}
			if !pastFirst {
				pastFirst = true
			} else {
				ctx.Write([]byte(","))
			}

			err := encoder.Encode(c)
			if err != nil {
				return
			}
		}
	}

}

func dirList(dir string, ctx *fasthttp.RequestCtx, header, footer string) {
	curDir, ok := bucketDir.objects[dir]
	if !ok {
		ctx.Error("404 path not found: "+dir, fasthttp.StatusNotFound)
		return
	}

	ctx.Response.Header.Set("Content-Type", "text/html;charset=UTF-8")

	if !strings.HasSuffix(header, ".htm") && !strings.HasSuffix(header, ".htm") {
		fmt.Fprintf(ctx,
			`<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 3.2 Final//EN">
<html>
 <head>
  <title>Index of /%s</title>
	<style>
body { font-family:arial,sans-serif;line-height:normal; }
#entries { font-family: monospace, monospace; }
#entries th { cursor:pointer;color:blue;text-decoration:underline; }
  </style>
 </head>
 <body>
`, dir)
	}

	if header != "" {
		obj, err := s3Client.GetObject(context.TODO(), &s3.GetObjectInput{
			Bucket: &bucketName,
			Key:    &header,
		})
		if err == nil {
			io.Copy(ctx, obj.Body)
			obj.Body.Close()
		} else if debug {
			log.Println("Error grabbing header:", err)
		}
	} else {
		fmt.Fprintf(ctx,
			` <h1>Index of /%s</h1>
`, dir)
	}

	fmt.Fprintf(ctx, ` <table id="entries">
  <tr><th onclick="sortTable(0)">Name</th><th onclick="sortTable(1)">Last modified</th><th onclick="sortTable(2)">Size</th><th onclick="sortTable(3)">Checksum</th></tr>
  <tr><th colspan="4"><hr></th></tr>
`)
	tableHeaders := "2"
	if len(dir) > 0 {
		tableHeaders = "3"
		fmt.Fprintf(ctx, `  <tr><td><a href="..">../</a></td><td align="right"></td><td align="right">-</td><td></td><td></td></tr>
`)
	}

	{ // Get all the metadata
		wg := sizedwaitgroup.New(8)
		for i, c := range curDir.list {
			if len(c.Name) > 0 && c.Name[0] == '.' {
				continue
			}
			if len(c.Checksum) == 0 {
				wg.Add()
				go func(i int) {
					defer wg.Done()
					curDir.list[i].getHead(dir)
				}(i)
			}
		}
		wg.Wait()
	}

	for i, c := range curDir.list {
		name := c.Name
		if len(name) > 0 && name[0] == '.' {
			continue
		}

		var timeStr string
		fSize := c.Size
		fTime := c.Time
		fChecksum := c.Checksum

		if fTime != nil && !fTime.IsZero() {
			timeStr = "&nbsp; " + fTime.UTC().Format(time.DateTime)
		}
		binSize := bin.NewBytes(fSize)

		fmt.Fprintf(ctx,
			`  <tr><td num="%d"><a href=%q>%s</a></td><td align="right">%s</td><td align="right" num="%d">&nbsp; %0.4v</td><td>&nbsp; %s</td></tr>
`, i, name, name, timeStr, fSize, binSize, fChecksum)
	}

	fmt.Fprintf(ctx,
		`  <tr><td colspan="4" align="right" style='font-size: xx-small;'><hr><em>Index built with Bucket-HTTP-Proxy (<a href="https://github.com/pschou/bucket-http-proxy">github.com/pschou/bucket-http-proxy</a>)</em></th></tr>
 <!-- Note to the developer:  Please review the API and usage at the github.com/pschou/bucket-http-proxy page to see what capabilities may be available on this resource. -->
 </table>
 <script>
function sortTable(n) {
  var table, rows, switching, i, x, y, shouldSwitch, dir, switchcount = 0;
  table = document.getElementById("entries");
  switching = true;
  // Set the sorting direction to ascending:
  dir = "asc";
  /* Make a loop that will continue until
  no switching has been done: */
  while (switching) {
    // Start by saying: no switching is done:
    switching = false;
    rows = table.rows;
    /* Loop through all table rows (except the
    first, which contains table headers): */
    for (i = %s; i < (rows.length - 2); i++) {
      // Start by saying there should be no switching:
      shouldSwitch = false;
      /* Get the two elements you want to compare,
      one from current row and one from the next: */
      x = rows[i].getElementsByTagName("TD")[n];
      y = rows[i + 1].getElementsByTagName("TD")[n];
      /* Check if the two rows should switch place,
      based on the direction, asc or desc: */
      if (dir == "asc") {
        if (x.getAttribute("num")) {
          if (Number(x.getAttribute("num")) > Number(y.getAttribute("num"))) {
            // If so, mark as a switch and break the loop:
            shouldSwitch = true;
            break;
          }
        } else {
          if (x.innerHTML.toLowerCase() > y.innerHTML.toLowerCase()) {
            // If so, mark as a switch and break the loop:
            shouldSwitch = true;
            break;
          }
        }
      } else if (dir == "desc") {
        if (x.getAttribute("num")) {
          if (Number(x.getAttribute("num")) < Number(y.getAttribute("num"))) {
            // If so, mark as a switch and break the loop:
            shouldSwitch = true;
            break;
          }
        } else {
          if (x.innerHTML.toLowerCase() < y.innerHTML.toLowerCase()) {
            // If so, mark as a switch and break the loop:
            shouldSwitch = true;
            break;
          }
        }
      }
    }
    if (shouldSwitch) {
      /* If a switch has been marked, make the switch
      and mark that a switch has been done: */
      rows[i].parentNode.insertBefore(rows[i + 1], rows[i]);
      switching = true;
      // Each time a switch is done, increase this count by 1:
      switchcount ++;
    } else {
      /* If no switching has been done AND the direction is "asc",
      set the direction to "desc" and run the while loop again. */
      if (switchcount == 0 && dir == "asc") {
        dir = "desc";
        switching = true;
      }
    }
  }
}
 </script>
`, tableHeaders)

	if footer != "" {
		obj, err := s3Client.GetObject(context.TODO(), &s3.GetObjectInput{
			Bucket: &bucketName,
			Key:    &footer,
		})
		if err == nil {
			io.Copy(ctx, obj.Body)
			obj.Body.Close()
		} else if debug {
			log.Println("Error grabbing footer:", err)
		}
	}

	if !strings.HasSuffix(footer, ".htm") && !strings.HasSuffix(footer, ".htm") {
		fmt.Fprintf(ctx, ` </body>
</html>`)
	}

}

func buildDirList() {
	bucketDirLock.Lock()
	defer bucketDirLock.Unlock()
	// Short circuit again after lock
	if time.Now().Sub(bucketDirUpdate) < bucketTimeout {
		return
	}
	var listErr error

	if bucketDirUpdate.IsZero() || time.Now().Sub(bucketDirUpdate) > bucketTimeout {
		lop := s3.NewListObjectsV2Paginator(s3Client, &s3.ListObjectsV2Input{Bucket: &bucketName})
		newDir := Root{objects: make(map[string]*DirItem)}
		RootItem := &DirItem{Name: "", isDir: true}
		newDir.objects[""] = RootItem

		var next, pathObject *DirItem
		var ok bool

		for lop.HasMorePages() {
			page, err := lop.NextPage(context.TODO())
			if err != nil {
				if debug {
					log.Println("Error listing bucket:", err)
				}
				listErr = err
				break
			}
			var count int64
		contents_loop:
			for _, c := range page.Contents {
				parts := strings.Split(*c.Key, "/")
				newDir.size += c.Size
				newDir.count++

				if len(parts[len(parts)-1]) > 0 { // If the object is a file
					count = 1 // Make sure we count it in the object count under a path
				} else {
					count = 0 // Directories are omitted
				}

				// Ensure the directory structure is built
				var curPath string
				pathObject = RootItem
				for len(parts) > 1 {
					curPath = curPath + parts[0] + "/"

					if next, ok = newDir.objects[curPath]; !ok {
						next = &DirItem{Name: parts[0] + "/", Size: c.Size, Count: count, isDir: true}
						newDir.objects[curPath] = next
						pathObject.list = append(pathObject.list, next)
					} else {
						pathObject.Size += c.Size
						pathObject.Count += count
					}
					pathObject = next

					if len(parts) == 2 && len(parts[1]) == 0 {
						next.Time = c.LastModified
						next.realTime = c.LastModified
						next.StorageClass = c.StorageClass
						continue contents_loop
					}
					parts = parts[1:]
				}

				next = &DirItem{Name: parts[0], Size: c.Size, Time: c.LastModified, realTime: c.LastModified,
					eTag: unquote(*c.ETag), StorageClass: c.StorageClass}
				newDir.objects[*c.Key] = next
				pathObject.list = append(pathObject.list, next)
			}
		}

		for _, obj := range newDir.objects {
			if len(obj.list) > 1 {
				sort.Slice(obj.list, func(i, j int) bool { return numstr.LessThanFold(obj.list[i].Name, obj.list[j].Name) })
			}
		}
		bucketDirError = listErr
		bucketDir = newDir
		bucketDirUpdate = time.Now()
	}
}
