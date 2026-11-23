package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/hyhdsl/neca/internal/core/ports"
)

// FileIndexStore 是 ports.IndexStore 的 index.txt 文件实现。
//
// 契约（开发文档 §8.3）：
//   - Load 只读快照，不持锁；
//   - Update 在全局 flock 内完成读取、修改、临时文件 fsync 与原子 rename；
//   - 拿不到锁返回包装了 ports.ErrIndexLocked 的错误，CLI 映射为退出码 3。
type FileIndexStore struct {
	path     string // index.txt 路径
	lockPath string // 全局锁文件
}

// NewFileIndexStore 创建文件索引存储。
func NewFileIndexStore(path, lockPath string) *FileIndexStore {
	return &FileIndexStore{path: path, lockPath: lockPath}
}

// 编译期断言：文件实现必须满足 ports.IndexStore。
var _ ports.IndexStore = (*FileIndexStore)(nil)

// Path 返回索引文件路径（测试与诊断用）。
func (s *FileIndexStore) Path() string { return s.path }

// Load 读取 index.txt 并解析为行快照。
//
// 空行被忽略；格式错误的行会带行号报错，避免把损坏数据当有效证书。
func (s *FileIndexStore) Load(ctx context.Context) ([]ports.Row, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, runtimef("读取索引", "%s: %v", s.path, err)
	}
	rows, err := parseIndexData(data)
	if err != nil {
		return nil, runtimef("解析索引", "%s: %v", s.path, err)
	}
	return rows, nil
}

// parseIndexData 解析完整索引内容，保留文件顺序。
func parseIndexData(data []byte) ([]ports.Row, error) {
	var rows []ports.Row
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	line := 0
	for scanner.Scan() {
		line++
		record := strings.TrimRight(scanner.Text(), "\r\n")
		if record == "" {
			continue
		}
		row, err := ports.ParseRow(record)
		if err != nil {
			return nil, fmt.Errorf("第 %d 行: %w", line, err)
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

// Update 在全局锁内执行读-改-写。
//
// fn 返回错误时直接返回，文件保持原样；成功时写临时文件、fsync 后 rename，
// 保证读者要么看到旧索引、要么看到新索引（开发文档 §11）。
func (s *FileIndexStore) Update(ctx context.Context, fn func(idx *ports.Index) error) error {
	return withPkiLock(s.lockPath, func() error {
		return s.UpdateLocked(ctx, fn)
	})
}

// UpdateLocked 执行读-改-写但不加锁，供已经持有 PKI 全局锁的写操作复用。
// 调用方必须保证外层已通过 withPkiLock 持锁，否则会失去并发保护。
func (s *FileIndexStore) UpdateLocked(ctx context.Context, fn func(idx *ports.Index) error) error {
	rows, err := s.Load(ctx)
	if err != nil {
		return err
	}
	index := &ports.Index{Rows: rows}
	if err := fn(index); err != nil {
		return err
	}
	return s.writeAtomic(index.Rows)
}

// writeAtomic 把行集合原子写回 index.txt。
func (s *FileIndexStore) writeAtomic(rows []ports.Row) error {
	dir := filepath.Dir(s.path)
	mode := fs.FileMode(0o644)
	var info fs.FileInfo
	if fi, err := os.Stat(s.path); err == nil {
		mode = fi.Mode().Perm()
		info = fi
	}

	tmp, err := os.CreateTemp(dir, ".index-*.tmp")
	if err != nil {
		return runtimef("写入索引", "创建临时文件: %v", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		// rename 成功后临时文件已不存在；失败时清理，避免残留。
		_ = os.Remove(tmpPath)
	}()

	writer := bufio.NewWriter(tmp)
	for _, row := range rows {
		if _, err := writer.WriteString(row.String() + "\n"); err != nil {
			tmp.Close()
			return runtimef("写入索引", "%v", err)
		}
	}
	if err := writer.Flush(); err != nil {
		tmp.Close()
		return runtimef("写入索引", "%v", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return runtimef("写入索引", "fsync: %v", err)
	}
	if err := tmp.Close(); err != nil {
		return runtimef("写入索引", "close: %v", err)
	}
	if info != nil {
		if err := preserveIndexOwnership(tmpPath, info); err != nil {
			return runtimef("写入索引", "chown: %v", err)
		}
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return runtimef("写入索引", "chmod: %v", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return runtimef("写入索引", "rename: %v", err)
	}
	// 目录 fsync 失败不致命：数据已 rename 成功，最坏情况是掉电后丢失本次写入。
	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}
