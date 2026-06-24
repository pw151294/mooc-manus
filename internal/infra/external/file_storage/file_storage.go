package file_storage

import "io"

// FileStorage 文件存储抽象接口
// 当前提供 LocalFileStorage 实现；未来若接入 MinIO/S3 只需新增对应实现，无需改动 Service 层。
type FileStorage interface {
	// PutObject 上传对象。返回存储后的校验和（MD5 lowercase hex）。
	PutObject(bucket, key string, reader io.Reader, size int64, contentType string) (checksum string, err error)

	// GetObject 读取对象。调用方负责 Close。
	GetObject(bucket, key string) (io.ReadCloser, error)

	// CopyObject 在同一存储后端内拷贝对象。
	CopyObject(srcBucket, srcKey, dstBucket, dstKey string) error

	// RemoveObjects 批量删除对象。任一失败仅记 warn 不中断。
	RemoveObjects(bucket string, keys []string) error

	// Exists 判断对象是否存在。
	Exists(bucket, key string) (bool, error)

	// GetSize 返回对象大小（字节）。对象不存在返回 (0, error)。
	GetSize(bucket, key string) (int64, error)
}
