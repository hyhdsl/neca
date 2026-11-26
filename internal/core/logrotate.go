// 本文件轮转审计和访问日志。
package core

import (
	"fmt"
	"os"
	"strconv"
	"sync"
)

// RotateLog rotates path to path.1 ... path.keep when its size is at least
// maxBytes. maxBytes <= 0 disables rotation. Secure erasure is not promised.
func RotateLog(path string, maxBytes int64, keep int) error {
	if path == "" || maxBytes <= 0 {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Size() < maxBytes {
		return nil
	}
	return rotateLogNow(path, keep)
}

func rotateLogNow(path string, keep int) error {
	if keep < 1 {
		keep = 1
	}
	_ = os.Remove(path + "." + strconv.Itoa(keep))
	for index := keep - 1; index >= 1; index-- {
		old := path + "." + strconv.Itoa(index)
		next := path + "." + strconv.Itoa(index+1)
		if _, err := os.Stat(old); err == nil {
			if err := os.Rename(old, next); err != nil {
				return err
			}
		}
	}
	return os.Rename(path, path+".1")
}

// RotatingFileWriter is a concurrency-safe append writer used by the Web
// audit log. It rotates lazily before a write would cross maxBytes.
type RotatingFileWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	keep     int
	file     *os.File
}

func NewRotatingFileWriter(path string, maxBytes int64, keep int) *RotatingFileWriter {
	if keep < 1 {
		keep = 1
	}
	return &RotatingFileWriter{path: path, maxBytes: maxBytes, keep: keep}
}

func (w *RotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		file, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return 0, err
		}
		w.file = file
	}
	if w.maxBytes > 0 {
		if info, err := w.file.Stat(); err == nil && info.Size()+int64(len(p)) > w.maxBytes {
			_ = w.file.Close()
			w.file = nil
			if err := rotateLogNow(w.path, w.keep); err != nil {
				return 0, fmt.Errorf("rotate: %w", err)
			}
			file, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
			if err != nil {
				return 0, err
			}
			w.file = file
		}
	}
	return w.file.Write(p)
}

func (w *RotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}
