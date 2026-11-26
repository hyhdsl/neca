// 本文件定义核心数据模型。
package core

// CertInfo 是证书列表中的一行，CLI 与未来 Web/JSON 共用同一结构。
type CertInfo struct {
	CN        string // Common Name
	Status    string // 有效 / 已吊销 / 未索引
	Serial    string // 大写十六进制序列号
	Platforms string // meta 中的 PLATFORMS，缺失时为 "-"
	Expires   string // 到期时间原始文本（已去掉 GMT 后缀）
	Note      string // 备注，例如 "文件已删除"
}

// ListResult 是 list 命令的结构化结果。
type ListResult struct {
	Certs      []CertInfo // 按证书文件名排序
	CRLRevoked int        // CRL 中的吊销条目数
}

// VerifyReport 是单个客户端包的检查结果。
type VerifyReport struct {
	CN          string   // 客户端 CN
	CertCount   int      // p12 内证书数
	PasswordLen int      // 密码文件字节数（含换行，与旧脚本 wc -c 一致）
	Warnings    []string // 需要输出到 stderr 的告警
	OKMessage   string   // 成功行；p12 不存在时为空
}

// VerifyResult 是 verify 命令的结构化结果。
type VerifyResult struct {
	Reports []VerifyReport
}

// ExportKind 区分已签发/已吊销两种导出。
type ExportKind string

const (
	// ExportIssued 导出 index 中状态为 V 的证书。
	ExportIssued ExportKind = "issued"
	// ExportRevoked 导出 index 中状态为 R 的证书。
	ExportRevoked ExportKind = "revoked"
)

// ExportResult 是 export 命令的结构化结果。
type ExportResult struct {
	Kind  ExportKind // 导出类型
	Path  string     // 实际写出的 CSV 路径
	Count int        // 数据行数
}

// SyncResult 是 index sync 的结构化结果。
type SyncResult struct {
	AddedIndex []string // 补充了索引的证书文件名（按处理顺序）
	AddedMeta  []string // 补充了元数据的 CN
}
