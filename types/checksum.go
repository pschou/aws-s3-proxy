package main

type Checksums struct {
	// This header can be used as a data integrity check to verify that the data
	// received is the same data that was originally sent. This header specifies the
	// base64-encoded, 32-bit CRC32 checksum of the object. For more information, see
	// Checking object integrity (https://docs.aws.amazon.com/AmazonS3/latest/userguide/checking-object-integrity.html)
	// in the Amazon S3 User Guide.
	ChecksumCRC32 *string `json:",omitempty"`

	// The base64-encoded, 32-bit CRC32C checksum of the object. This will only be
	// present if it was uploaded with the object. With multipart uploads, this may not
	// be a checksum value of the object. For more information about how checksums are
	// calculated with multipart uploads, see Checking object integrity (https://docs.aws.amazon.com/AmazonS3/latest/userguide/checking-object-integrity.html#large-object-checksums)
	// in the Amazon S3 User Guide.
	ChecksumCRC32C *string `json:",omitempty"`

	// The base64-encoded, 160-bit SHA-1 digest of the object. This will only be
	// present if it was uploaded with the object. With multipart uploads, this may not
	// be a checksum value of the object. For more information about how checksums are
	// calculated with multipart uploads, see Checking object integrity (https://docs.aws.amazon.com/AmazonS3/latest/userguide/checking-object-integrity.html#large-object-checksums)
	// in the Amazon S3 User Guide.
	ChecksumSHA1 *string `json:",omitempty"`

	// The base64-encoded, 256-bit SHA-256 digest of the object. This will only be
	// present if it was uploaded with the object. With multipart uploads, this may not
	// be a checksum value of the object. For more information about how checksums are
	// calculated with multipart uploads, see Checking object integrity (https://docs.aws.amazon.com/AmazonS3/latest/userguide/checking-object-integrity.html#large-object-checksums)
	// in the Amazon S3 User Guide.
	ChecksumSHA256 *string `json:",omitempty"`
}
