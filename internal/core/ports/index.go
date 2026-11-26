// 本文件定义索引存储端口接口。
package ports

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// IndexStatus 是 index.txt 第一列的状态码。
// openssl ca 只会写入 V（有效）、R（已吊销）、E（已过期）三种值。
type IndexStatus string

const (
	// IndexValid 表示证书已签发且当前有效。
	IndexValid IndexStatus = "V"
	// IndexRevoked 表示证书已吊销，这是 CRL 的唯一来源。
	IndexRevoked IndexStatus = "R"
	// IndexExpired 表示证书已过期（openssl 查询时维护）。
	IndexExpired IndexStatus = "E"
)

// ErrIndexLocked 是 IndexStore.Update 在拿不到全局锁时返回的哨兵错误。
//
// CLI 层应把它映射为退出码 3（前置条件不满足，开发文档 §6.3），
// 而不是笼统的运行期失败（退出码 2）。
var ErrIndexLocked = errors.New("index is locked by another process")

// Row 是 index.txt 的一行，字段顺序与 openssl ca 数据库一致：
//
//	状态  到期时间  吊销时间  序列号  证书文件  subject
//
// 时间格式为 YYMMDDHHMMSSZ（UTC）；未吊销时吊销时间为空。
// 该结构只描述格式，不负责订阅/写入文件——那是 IndexStore 实现的事。
type Row struct {
	Status    IndexStatus // V / R / E
	Expires   string      // 到期时间，YYMMDDHHMMSSZ
	RevokedAt string      // 吊销时间；未吊销时为空
	Serial    string      // 大写十六进制序列号
	File      string      // 证书文件名（相对 CADB_DIR）
	Subject   string      // 完整 subject，例如 /C=CN/O=ORG/CN=TMP01
}

// ParseRow 解析一行 index.txt。
//
// 只做格式与状态码校验，不解析 subject 内容；需要 CN 时调用 Row.CN。
// 调用方负责跳过空行，并处理 Windows 上可能出现的 CRLF 行尾。
func ParseRow(line string) (Row, error) {
	line = strings.TrimRight(line, "\r\n")
	fields := strings.Split(line, "\t")
	if len(fields) != 6 {
		return Row{}, fmt.Errorf("ports: index row must have 6 tab-separated fields, got %d", len(fields))
	}

	status := IndexStatus(fields[0])
	switch status {
	case IndexValid, IndexRevoked, IndexExpired:
	default:
		return Row{}, fmt.Errorf("ports: unknown index status %q", fields[0])
	}

	return Row{
		Status:    status,
		Expires:   fields[1],
		RevokedAt: fields[2],
		Serial:    fields[3],
		File:      fields[4],
		Subject:   fields[5],
	}, nil
}

// String 按 index.txt 的 6 字段格式序列化一行，供实现原子写回时使用。
// 它不做校验：调用方应先保证 Row 合法。
func (r Row) String() string {
	return strings.Join([]string{
		string(r.Status),
		r.Expires,
		r.RevokedAt,
		r.Serial,
		r.File,
		r.Subject,
	}, "\t")
}

// CN 从 Subject 提取 Common Name。
//
// index.txt 里的 subject 是 openssl 的默认单行格式，例如：
//
//	/C=CN/O=ExampleOrg/CN=TMP01
//
// CN 字符集受文档 §2 限制（A-Z a-z 0-9 . _ -），因此按 "/CN=" 前缀
// 匹配是安全的。找不到 CN 时返回空字符串，由调用方按数据损坏处理。
func (r Row) CN() string {
	for _, part := range strings.Split(r.Subject, "/") {
		if strings.HasPrefix(part, "CN=") {
			return strings.TrimPrefix(part, "CN=")
		}
	}
	return ""
}

// Index 是一次锁内读-改-写看到的内存视图。
//
// 实现可以在 fn 返回前随意修改 Rows；fn 返回错误时实现必须丢弃修改，
// 保证索引文件保持原样（开发文档 §11）。
type Index struct {
	Rows []Row
}

// Find 返回序列号匹配的行下标（不区分大小写），找不到返回 -1。
func (idx *Index) Find(serial string) int {
	for i := range idx.Rows {
		if strings.EqualFold(idx.Rows[i].Serial, serial) {
			return i
		}
	}
	return -1
}

// IndexStore 抽象 index.txt 的读写，是索引存储的唯一扩展点。
//
// 唯一事实源原则（开发文档 §10）不变：生产实现只能是 index.txt 文件后端；
// 接口存在的目的是让测试使用内存实现、让未来只读查询层（Web 面板）复用
// 同一契约，而不是引入第二份证书状态存储。
type IndexStore interface {
	// Load 返回索引的只读快照；不持有、也不暴露文件锁。
	// 返回的切片归调用方所有，实现不得复用其底层数组。
	Load(ctx context.Context) ([]Row, error)

	// Update 在全局锁内执行一次读-改-写：
	//   - fn 返回错误（或 panic）时实现不得落盘；
	//   - fn 返回 nil 时实现负责 fsync + 原子替换（开发文档 §8.3）；
	//   - 拿不到锁时返回包装了 ErrIndexLocked 的错误。
	Update(ctx context.Context, fn func(idx *Index) error) error
}
