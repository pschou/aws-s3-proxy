package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
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
	//Owner             *types.Owner              `json:",omitempty"`
	//RestoreStatus     *types.RestoreStatus      `json:",omitempty"`
	Checksum string `json:",omitempty"`
}

func (d *DirItem) getHash(base string) {
	if len(d.Checksum) > 0 || len(d.Name) == 0 || d.Name[len(d.Name)-1] == '/' {
		return
	}
	di := *d
	di.Checksum, di.Time = getHash(base+d.Name, di.eTag, d.realTime)
	*d = di
}

func getHash(name, eTag string, cmpTime *time.Time) (outHash string, outTime *time.Time) {
	hashCacheMutex.Lock()
	defer hashCacheMutex.Unlock()
	if debug {
		log.Println("cache check", fmt.Sprintf("%q%q", name, eTag))
	}
	if h, ok := hashCache[fmt.Sprintf("%q%q", name, eTag)]; ok && h.realTime.Equal(*cmpTime) {
		if debug {
			log.Println("cache hit")
		}
		outHash = h.hash
		outTime = &h.time
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
		//	ObjectAttributes: []types.ObjectAttributes{types.ObjectAttributesEtag, types.ObjectAttributesChecksum},
	})
	if err == nil {
		outHash = encodeChecksum(obj)
		if debug {
			log.Println("cache save", fmt.Sprintf("%q%q", name, unquote(*obj.ETag)), outHash)
		}

		if d, ok := obj.Metadata["date"]; ok {
			if t, err := time.Parse(time.DateTime, d); err == nil {
				outTime = &t
			} else {
				outTime = obj.LastModified
			}
		} else {
			outTime = obj.LastModified
		}

		hashCache[fmt.Sprintf("%q%q", name, unquote(*obj.ETag))] = hashdat{time: *outTime, hash: outHash, realTime: *obj.LastModified}
		//*cmpTime = *obj.LastModified
	}
	return
}

var (
	bucketDir       Root
	bucketDirLock   sync.Mutex
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

type Dir struct {
	list        []DirItem
	size, count int64
}

type Root struct {
	subdirs map[string]*Dir
}

func isFile(test string) bool {
	if time.Now().Sub(bucketDirUpdate) > bucketTimeout {
		buildDirList()
	}

	d, f := path.Split(test)
	if curdir, ok := bucketDir.subdirs[d]; ok {
		for _, child := range curdir.list {
			if child.Name == f {
				return true
			}
		}
	}
	return false
}

func isDir(test string) (exist bool) {
	if time.Now().Sub(bucketDirUpdate) > bucketTimeout {
		buildDirList()
	}
	_, exist = bucketDir.subdirs[test]
	return
}

func jsonList(dir string, ctx *fasthttp.RequestCtx) {
	ctx.Write([]byte("["))
	defer ctx.Write([]byte("]"))

	if time.Now().Sub(bucketDirUpdate) > bucketTimeout {
		buildDirList()
	}

	curDir, ok := bucketDir.subdirs[dir]
	if !ok {
		ctx.Error("404 path not found: "+dir, fasthttp.StatusNotFound)
		return
	}

	encoder := json.NewEncoder(ctx)

	{ // Get all the metadata
		wg := sizedwaitgroup.New(8)
		for i, c := range curDir.list {
			if len(c.Checksum) == 0 {
				wg.Add()
				go func(i int) {
					defer wg.Done()
					curDir.list[i].getHash(dir)
				}(i)
			}
		}
		wg.Wait()
	}

	for i, c := range curDir.list {
		if i > 0 {
			ctx.Write([]byte(","))
		}

		err := encoder.Encode(c)
		if err != nil {
			return
		}
	}
}

func dirList(dir string, ctx *fasthttp.RequestCtx, header, footer string) {
	if time.Now().Sub(bucketDirUpdate) > bucketTimeout {
		buildDirList()
	}

	curDir, ok := bucketDir.subdirs[dir]
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
			if len(c.Checksum) == 0 {
				wg.Add()
				go func(i int) {
					defer wg.Done()
					curDir.list[i].getHash(dir)
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
		humanSize := float32(fSize)
		humanSuffix := ""
		for _, prefix := range []string{"k", "M", "G", "T", "P"} {
			if humanSize >= 1000 {
				humanSize = humanSize / 1024
				humanSuffix = prefix
			} else {
				break
			}
		}

		var humanize string
		if len(humanSuffix) == 0 {
			humanize = fmt.Sprintf("%d", fSize)
		} else {
			humanize = fmt.Sprintf("%0.2f%s", humanSize, humanSuffix)
		}

		fmt.Fprintf(ctx,
			`  <tr><td num="%d"><a href=%q>%s</a></td><td align="right">%s</td><td align="right" num="%d">&nbsp; %s</td><td>&nbsp; %s</td></tr>
`, i, name, name, timeStr, fSize, humanize, fChecksum)
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

	if bucketDirUpdate.IsZero() || time.Now().Sub(bucketDirUpdate) > bucketTimeout {
		lop := s3.NewListObjectsV2Paginator(s3Client, &s3.ListObjectsV2Input{Bucket: &bucketName})
		newDir := Root{subdirs: make(map[string]*Dir)}
		for lop.HasMorePages() {
			page, err := lop.NextPage(context.TODO())
			if err != nil {
				break
			}
			var count int64
		contents_loop:
			for _, c := range page.Contents {
				parts := strings.Split(*c.Key, "/")

				var curDir, prevDir string
				if len(parts[len(parts)-1]) > 0 {
					count = 1
				} else {
					count = 0
				}

				for len(parts) > 1 {
					prevDir = curDir
					curDir = curDir + parts[0] + "/"

					if ncd, ok := newDir.subdirs[curDir]; ok {
						ncd.size += c.Size
						ncd.count += count
					} else {
						if len(parts) == 2 && parts[1] == "" {
							tmp := newDir.subdirs[prevDir]
							di := DirItem{Name: parts[0] + "/", Time: c.LastModified, realTime: c.LastModified, StorageClass: c.StorageClass}
							tmp.list = append(tmp.list, di)
							//newDir.subdirs[prevDir] = tmp
							newDir.subdirs[curDir] = &Dir{size: c.Size, count: count}
							continue contents_loop
						} else {
							tmp := newDir.subdirs[prevDir]
							di := DirItem{Name: parts[0] + "/"}
							tmp.list = append(tmp.list, di)
							//newDir.subdirs[prevDir] = tmp
							newDir.subdirs[curDir] = &Dir{size: c.Size, count: count}
						}
						//newDir.subdirs[curDir] = ncd
						//} else if cd.Time.IsZero() {
						//	newDir.subdirs[prevDir].Time = c.LastModified
						//	newDir.subdirs[prevDir].Owner = c.Owner
					}
					parts = parts[1:]
				}
				tmp, ok := newDir.subdirs[curDir]
				if !ok {
					tmp = &Dir{}
					newDir.subdirs[curDir] = tmp
				}
				tmp.list = append(tmp.list, DirItem{Name: parts[0], Size: c.Size, Time: c.LastModified, realTime: c.LastModified,
					eTag: unquote(*c.ETag), StorageClass: c.StorageClass})
				//newDir.subdirs[curDir] = tmp
			}
		}

		for dir, v := range newDir.subdirs {
			sort.Slice(v.list, func(i, j int) bool { return numstr.LessThanFold(v.list[i].Name, v.list[j].Name) })
			for i, c := range v.list {
				if c.Name[len(c.Name)-1] == '/' {
					d := newDir.subdirs[dir+c.Name]
					v.list[i].Size = d.size
					v.list[i].Count = d.count
				}
			}
		}
		bucketDir = newDir
		bucketDirUpdate = time.Now()
	}
}

/*
func ensureEntries(d *dir, base string, tmp map[string]*dir) {
	if d.subdirs == nil {
		return
	}
	for k, v := range d.subdirs {
		d, f := path.Split(k[len(k)-1])
		_, tmp[f+"/"] = v
	}
	for i, cd := range d.children {
		if strings.HasSuffix(cd.Name, "/") {
			if c, ok := tmp[cd.Name]; ok {
				d.children[i].Size = c.size
				delete(tmp, cd.Name)
			}
		}
	}
	for k, _ := range tmp {
		d.children = append(d.children, DirItem{Name: k})
		delete(tmp, k)
	}
	sort.Slice(d.children, func(i, j int) bool { return numstr.LessThanFold(d.children[i].Name, d.children[j].Name) })
	for _, recursive := range d.subdirs {
		ensureEntries(recursive, tmp)
	}
}*/
