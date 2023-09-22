package types

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

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
