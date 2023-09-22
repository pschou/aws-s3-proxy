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
	"github.com/pschou/go-sorting/numstr"
	"github.com/valyala/fasthttp"
)

var (
	bucketDir       dir
	bucketDirLock   sync.Mutex
	bucketDirUpdate time.Time
)

type dir struct {
	children []dirItem
	subdirs  map[string]*dir
	size     int64
}
type dirItem struct {
	Name              string
	Time              *time.Time `json:",omitempty"`
	Size              int64
	ChecksumAlgorithm []types.ChecksumAlgorithm `json:",omitempty"`
	ETag              string                    `json:",omitempty"`
	StorageClass      types.ObjectStorageClass  `json:",omitempty"`
	Owner             *types.Owner              `json:",omitempty"`
	RestoreStatus     *types.RestoreStatus      `json:",omitempty"`
}

func isFile(path string) bool {
	if time.Now().Sub(bucketDirUpdate) > 5*time.Second {
		buildDirList()
	}

	parts := strings.Split(path, "/")
	if path == "" {
		path = "/"
	}
	curDir := &bucketDir
	for len(parts) > 1 {
		if curDir.subdirs == nil {
			return false
		}
		if d, ok := curDir.subdirs[parts[0]]; ok {
			curDir = d
		} else {
			return false
		}
		parts = parts[1:]
	}
	for _, child := range curDir.children {
		if child.Name == parts[0] {
			return true
		}
	}
	return false
}

func isDir(path string) bool {
	if time.Now().Sub(bucketDirUpdate) > 5*time.Second {
		buildDirList()
	}

	parts := strings.Split(path, "/")
	if path == "" {
		path = "/"
	}
	curDir := &bucketDir
	for len(parts) > 0 {
		if curDir.subdirs == nil {
			return false
		}
		if d, ok := curDir.subdirs[parts[0]]; ok {
			curDir = d
		} else {
			return false
		}
		parts = parts[1:]
	}
	return true
}

func jsonList(path string, ctx *fasthttp.RequestCtx) {
	ctx.Write([]byte("["))
	defer ctx.Write([]byte("]"))

	if time.Now().Sub(bucketDirUpdate) > 5*time.Second {
		buildDirList()
	}

	parts := strings.Split(path, "/")
	if path == "" {
		path = "/"
	}
	curDir := &bucketDir
	for len(parts) > 1 {
		if curDir.subdirs == nil {
			ctx.Error("404 path not found: "+path, fasthttp.StatusNotFound)
			return
		}
		if d, ok := curDir.subdirs[parts[0]]; ok {
			curDir = d
		} else {
			ctx.Error("404 path not found: "+path, fasthttp.StatusNotFound)
			return
		}
		parts = parts[1:]
	}

	encoder := json.NewEncoder(ctx)
	for i, c := range curDir.children {
		if i > 0 {
			ctx.Write([]byte(","))
		}

		err := encoder.Encode(c)
		if err != nil {
			return
		}
	}
}

func dirList(path string, ctx *fasthttp.RequestCtx, header, footer string) {
	if time.Now().Sub(bucketDirUpdate) > 5*time.Second {
		buildDirList()
	}

	parts := strings.Split(path, "/")
	if path == "" {
		path = "/"
	}
	curDir := &bucketDir
	for len(parts) > 1 {
		if curDir.subdirs == nil {
			ctx.Error("404 path not found: "+path, fasthttp.StatusNotFound)
			return
		}
		if d, ok := curDir.subdirs[parts[0]]; ok {
			curDir = d
		} else {
			ctx.Error("404 path not found: "+path, fasthttp.StatusNotFound)
			return
		}
		parts = parts[1:]
	}
	ctx.Response.Header.Set("Content-Type", "text/html;charset=UTF-8")

	if !strings.HasSuffix(header, ".htm") && !strings.HasSuffix(header, ".htm") {
		fmt.Fprintf(ctx,
			`<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 3.2 Final//EN">
<html>
 <head>
  <title>Index of %s</title>
	<style>
body { font-family:arial,sans-serif;line-height:normal; }
#entries th { cursor:pointer;color:blue;text-decoration:underline; }
  </style>
 </head>
 <body>
`, path)
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
			` <h1>Index of %s</h1>
`, path)
	}

	fmt.Fprintf(ctx, ` <table id="entries">
  <tr><th onclick="sortTable(0)">Name</th><th onclick="sortTable(1)">Last modified</th><th onclick="sortTable(2)">Size</th><th onclick="sortTable(3)">StorageClass</th><th onclick="sortTable(4)">ETag</th></tr>
  <tr><th colspan="5"><hr></th></tr>
`)
	tableHeaders := "2"
	if path != "/" {
		tableHeaders = "3"
		fmt.Fprintf(ctx, `  <tr><td><a href="..">../</a></td><td align="right"></td><td align="right">-</td><td></td><td></td></tr>
`)
	}
file_loop:
	for i, c := range curDir.children {
		name := c.Name
		if len(name) > 0 && name[0] == '.' {
			continue
		}

		var timeStr string
		fSize := c.Size
		fTime := c.Time
		fETag := c.ETag
		fSC := c.StorageClass
		{ // Try to flatten directories which don't exist but have files which are in said bucket
			longname := name
			flatten := curDir
			for strings.HasSuffix(name, "/") && flatten.subdirs != nil {
				if childDir, ok := flatten.subdirs[name[:len(name)-1]]; ok && len(childDir.children) == 1 && fTime == nil {
					flatten = childDir
					name = childDir.children[0].Name
					if len(name) > 0 && name[0] == '.' {
						continue file_loop
					}
					longname = longname + childDir.children[0].Name
					c = childDir.children[0]
					fTime = c.Time
					fSize = c.Size
					fETag = c.ETag
					fSC = c.StorageClass
				} else {
					break
				}
			}
			name = longname
		}
		if fTime != nil && !fTime.IsZero() {
			timeStr = "&nbsp; " + fTime.UTC().Format(time.DateTime)
		}
		fmt.Fprintf(ctx,
			`  <tr><td num="%d"><a href=%q>%s</a></td><td align="right">%s</td><td align="right" num="%d">&nbsp; %d</td><td>&nbsp; %s<td>&nbsp; %s</td></tr>
`, i, name, name, timeStr, fSize, fSize, fSC, unquote(fETag))
	}

	fmt.Fprintf(ctx,
		` </table>
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
    for (i = %s; i < (rows.length - 1); i++) {
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

	if bucketDirUpdate.IsZero() || time.Now().Sub(bucketDirUpdate) > 5*time.Second {
		lop := s3.NewListObjectsV2Paginator(s3Client, &s3.ListObjectsV2Input{Bucket: &bucketName})
		newDir := dir{}
		for lop.HasMorePages() {
			page, err := lop.NextPage(context.TODO())
			if err != nil {
				break
			}
			//fmt.Printf("page: %#v\n\nerr: %v\n", page, err)
			for _, c := range page.Contents {
				//fmt.Printf("obj: %#v\n", *c.Key)
				parts := strings.Split(*c.Key, "/")
				curDir := &newDir
				curDir.size += c.Size
				for len(parts) > 1 && parts[1] != "" {
					if curDir.subdirs == nil {
						curDir.subdirs = make(map[string]*dir)
					}
					if d, ok := curDir.subdirs[parts[0]]; ok {
						curDir = d
					} else {
						d = &dir{}
						curDir.subdirs[parts[0]] = d
						curDir = d
					}
					curDir.size += c.Size
					parts = parts[1:]
				}
				if len(parts) == 1 {
					curDir.children = append(curDir.children, dirItem{Name: parts[0], Size: c.Size, Time: c.LastModified,
						ETag: *c.ETag, ChecksumAlgorithm: c.ChecksumAlgorithm, StorageClass: c.StorageClass, Owner: c.Owner, RestoreStatus: c.RestoreStatus})
				} else {
					curDir.children = append(curDir.children, dirItem{Name: parts[0] + "/", Size: c.Size, Time: c.LastModified,
						ETag: *c.ETag, ChecksumAlgorithm: c.ChecksumAlgorithm, StorageClass: c.StorageClass, Owner: c.Owner, RestoreStatus: c.RestoreStatus})
				}
			}
		}
		ensureEntries(&newDir, make(map[string]*dir))
		bucketDir = newDir
		bucketDirUpdate = time.Now()
	}
}

func ensureEntries(d *dir, tmp map[string]*dir) {
	if d.subdirs == nil {
		return
	}
	for k, v := range d.subdirs {
		tmp[k+"/"] = v
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
		d.children = append(d.children, dirItem{Name: k})
		delete(tmp, k)
	}
	sort.Slice(d.children, func(i, j int) bool { return numstr.LessThanFold(d.children[i].Name, d.children[j].Name) })
	for _, recursive := range d.subdirs {
		ensureEntries(recursive, tmp)
	}
}
