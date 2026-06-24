package file_storage

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"mooc-manus/pkg/logger"

	"go.uber.org/zap"
)

// LocalFileStorage 文件系统实现
// 文件落到 {rootDir}/{bucket}/{key}
type LocalFileStorage struct {
	rootDir string
}

// NewLocalFileStorage rootDir 为空时默认 ./data
func NewLocalFileStorage(rootDir string) *LocalFileStorage {
	if rootDir == "" {
		rootDir = "./data"
	}
	return &LocalFileStorage{rootDir: rootDir}
}

func (s *LocalFileStorage) fullPath(bucket, key string) string {
	return filepath.Join(s.rootDir, bucket, key)
}

// PutObject 写文件并计算 MD5 校验和。contentType 当前实现忽略（本地文件系统无 meta 概念）。
func (s *LocalFileStorage) PutObject(bucket, key string, reader io.Reader, size int64, contentType string) (string, error) {
	path := s.fullPath(bucket, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("mkdir failed: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create file failed: %w", err)
	}
	defer f.Close()

	hasher := md5.New()
	mw := io.MultiWriter(f, hasher)
	if _, err := io.Copy(mw, reader); err != nil {
		return "", fmt.Errorf("write file failed: %w", err)
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))
	return checksum, nil
}

func (s *LocalFileStorage) GetObject(bucket, key string) (io.ReadCloser, error) {
	path := s.fullPath(bucket, key)
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("object %s/%s: %w", bucket, key, os.ErrNotExist)
		}
		return nil, err
	}
	return f, nil
}

func (s *LocalFileStorage) CopyObject(srcBucket, srcKey, dstBucket, dstKey string) error {
	srcPath := s.fullPath(srcBucket, srcKey)
	dstPath := s.fullPath(dstBucket, dstKey)
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("mkdir failed: %w", err)
	}
	in, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open src failed: %w", err)
	}
	defer in.Close()
	out, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create dst failed: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}
	return nil
}

// RemoveObjects 批量删除。任一失败记录 warn 但继续；返回最后一个错误供调用方判断是否需要整体回滚。
func (s *LocalFileStorage) RemoveObjects(bucket string, keys []string) error {
	var lastErr error
	for _, key := range keys {
		path := s.fullPath(bucket, key)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			logger.Warn("RemoveObject failed",
				zap.String("bucket", bucket),
				zap.String("key", key),
				zap.Error(err))
			lastErr = err
		}
	}
	return lastErr
}

func (s *LocalFileStorage) Exists(bucket, key string) (bool, error) {
	path := s.fullPath(bucket, key)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (s *LocalFileStorage) GetSize(bucket, key string) (int64, error) {
	path := s.fullPath(bucket, key)
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
