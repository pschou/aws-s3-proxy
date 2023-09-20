package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/pschou/go-sorting/numstr"
)

var (
	bucketDir       dir
	bucketDirLock   sync.Mutex
	bucketDirUpdate time.Time
)

type dir struct {
	children []dirItem
	subdirs  map[string]*dir
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

func jsonList(path string, w http.ResponseWriter) {
	w.Write([]byte("["))
	defer w.Write([]byte("]"))

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
			http.Error(w, "404 path not found: "+path, http.StatusNotFound)
			return
		}
		if d, ok := curDir.subdirs[parts[0]]; ok {
			curDir = d
		} else {
			http.Error(w, "404 path not found: "+path, http.StatusNotFound)
			return
		}
		parts = parts[1:]
	}

	encoder := json.NewEncoder(w)
	for i, c := range curDir.children {
		if i > 0 {
			w.Write([]byte(","))
		}

		err := encoder.Encode(c)
		if err != nil {
			return
		}
	}
}

func dirList(path string, w http.ResponseWriter) {
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
			http.Error(w, "404 path not found: "+path, http.StatusNotFound)
			return
		}
		if d, ok := curDir.subdirs[parts[0]]; ok {
			curDir = d
		} else {
			http.Error(w, "404 path not found: "+path, http.StatusNotFound)
			return
		}
		parts = parts[1:]
	}
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")

	fmt.Fprintf(w,
		`<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 3.2 Final//EN">
<html>
 <head>
  <title>Index of %s</title>
 </head>
 <body>
<h1>Index of %s</h1>
<table><tr><th>Name</th><th>Last modified</th><th>Size</th></tr><tr><th colspan="3"><hr></th></tr>
`, path, path)
	if path != "/" {
		fmt.Fprintf(w, ` <tr><td><a href="..">Parent Directory</a></td><td align="right"></td><td align="right">-</td></tr>
`)
	}
	for _, c := range curDir.children {
		var timeStr string
		name := c.Name
		fSize := c.Size
		fTime := c.Time
		{ // Try to flatten directories which don't exist but have files which are in said bucket
			longname := name
			flatten := curDir
			for strings.HasSuffix(name, "/") && flatten.subdirs != nil {
				if childDir, ok := flatten.subdirs[name[:len(name)-1]]; ok && len(childDir.children) == 1 && fTime == nil {
					flatten = childDir
					name = childDir.children[0].Name
					longname = longname + childDir.children[0].Name
					fTime = childDir.children[0].Time
					fSize = childDir.children[0].Size
					c = childDir.children[0]
				} else {
					break
				}
			}
			name = longname
		}
		if fTime != nil && !fTime.IsZero() {
			timeStr = "&nbsp; " + fTime.UTC().Format(time.DateTime)
		}
		if fSize > 0 || !strings.HasSuffix(name, "/") {
			fmt.Fprintf(w,
				` <tr><td><a href=%q>%s</a></td><td align="right">%s</td><td align="right">&nbsp; %d</td></tr>
`, name, name, timeStr, fSize)
		} else {
			fmt.Fprintf(w,
				` <tr><td><a href=%q>%s</a></td><td align="right">%s</td><td align="right">-</td></tr>
`, name, name, timeStr)
		}
	}

	fmt.Fprintf(w,
		`</table>
</body></html>`)

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
		ensureEntries(&newDir, make(map[string]struct{}))
		bucketDir = newDir
		bucketDirUpdate = time.Now()
	}
}

func ensureEntries(d *dir, tmp map[string]struct{}) {
	if d.subdirs == nil {
		return
	}
	for k, _ := range d.subdirs {
		tmp[k+"/"] = struct{}{}
	}
	for _, d := range d.children {
		if strings.HasSuffix(d.Name, "/") {
			if _, ok := tmp[d.Name]; ok {
				delete(tmp, d.Name)
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
