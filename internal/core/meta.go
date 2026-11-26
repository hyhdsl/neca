// 本文件读写证书 meta 文件。
package core

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Meta 是 pki/meta/<CN>.env 的键值集合。
//
// 保留键的插入顺序（旧脚本按固定顺序写文件，diff 对比依赖顺序），
// 读取时保留未知键，便于阶段二增加 SCHEMA/OWNER/DEVICE 等字段。
type Meta struct {
	order  []string
	values map[string]string
}

// NewMeta 创建空的元数据集合。
func NewMeta() *Meta {
	return &Meta{values: make(map[string]string)}
}

// ParseMeta 解析 KEY=VALUE 格式；空行与 # 注释被忽略，缺少 '=' 的行跳过
// （旧脚本用 sed 提取，对改坏的 meta 采取宽容策略）。
func ParseMeta(data []byte) (*Meta, error) {
	meta := NewMeta()
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		record := strings.TrimSpace(scanner.Text())
		if record == "" || strings.HasPrefix(record, "#") {
			continue
		}
		key, value, ok := strings.Cut(record, "=")
		if !ok || key == "" {
			continue
		}
		meta.Set(key, value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return meta, nil
}

// LoadMeta 读取 meta 文件；文件不存在时返回 (nil, nil)，由调用方决定默认值。
func LoadMeta(path string) (*Meta, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, runtimef("读取元数据", "%s: %v", path, err)
	}
	meta, err := ParseMeta(data)
	if err != nil {
		return nil, runtimef("解析元数据", "%s: %v", path, err)
	}
	return meta, nil
}

// Get 返回键值；不存在时返回空字符串。
func (m *Meta) Get(key string) string {
	if m == nil {
		return ""
	}
	return m.values[key]
}

// Set 写入键值；新键追加到末尾，已有键保持原位置。
func (m *Meta) Set(key, value string) {
	if m.values == nil {
		m.values = make(map[string]string)
	}
	if _, exists := m.values[key]; !exists {
		m.order = append(m.order, key)
	}
	m.values[key] = value
}

// Keys 返回键顺序的副本。
func (m *Meta) Keys() []string {
	return append([]string(nil), m.order...)
}

// Bytes 按稳定顺序渲染为 KEY=VALUE 行，末尾带换行。
func (m *Meta) Bytes() []byte {
	var builder strings.Builder
	for _, key := range m.order {
		fmt.Fprintf(&builder, "%s=%s\n", key, m.values[key])
	}
	return []byte(builder.String())
}

// WriteFile 原子写回 meta 文件（临时文件 + rename）。
func (m *Meta) WriteFile(path string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".meta-*.tmp")
	if err != nil {
		return runtimef("写入元数据", "创建临时文件: %v", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(m.Bytes()); err != nil {
		tmp.Close()
		return runtimef("写入元数据", "%v", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return runtimef("写入元数据", "fsync: %v", err)
	}
	if err := tmp.Close(); err != nil {
		return runtimef("写入元数据", "close: %v", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return runtimef("写入元数据", "chmod: %v", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return runtimef("写入元数据", "rename: %v", err)
	}
	return nil
}

// MetaPath 返回某个 CN 的元数据路径。
func MetaPath(dir, cn string) string {
	return filepath.Join(dir, cn+".env")
}

// WriteLegacyMeta 按旧脚本 meta_write 的字段与顺序写元数据：
//
//	CN / PLATFORMS / CREATED / EXPIRES / SERIAL
//
// M1 不写 SCHEMA=2：index sync 的产物需要与旧脚本逐字节一致（开发文档 §13.3），
// 新字段等 M2 写命令时再按 §8.4 增加。
func WriteLegacyMeta(path, cn, platforms, created, expires, serial string) error {
	meta := NewMeta()
	meta.Set("CN", cn)
	meta.Set("PLATFORMS", platforms)
	meta.Set("CREATED", created)
	meta.Set("EXPIRES", expires)
	meta.Set("SERIAL", serial)
	return meta.WriteFile(path)
}
