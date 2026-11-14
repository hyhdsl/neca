package core

import (
	"context"
	"strings"
	"time"
)

// opensslTimeout 是单个 openssl 调用的默认超时。
const opensslTimeout = 60 * time.Second

// OpenSSL 封装 M1 需要的 openssl 查询命令。
//
// 所有调用都经过 Runner，测试可注入假 Runner，未来 --dry-run 也只需
// 在这里判断；业务代码不直接拼 exec.Command（开发文档 §3 约束 2）。
type OpenSSL struct {
	runner Runner
}

// NewOpenSSL 创建封装；runner 为 nil 时使用默认 ExecRunner。
func NewOpenSSL(runner Runner) *OpenSSL {
	if runner == nil {
		runner = NewExecRunner()
	}
	return &OpenSSL{runner: runner}
}

// run 执行一条 openssl 命令。
func (o *OpenSSL) run(ctx context.Context, args ...string) (Result, error) {
	return o.runner.Run(ctx, opensslTimeout, "openssl", args...)
}

// Serial 读取证书序列号并统一为大写十六进制。
func (o *OpenSSL) Serial(ctx context.Context, certPath string) (string, error) {
	res, err := o.run(ctx, "x509", "-in", certPath, "-noout", "-serial")
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(res.Stdout)
	value = strings.TrimSpace(strings.TrimPrefix(value, "serial="))
	if value == "" {
		return "", runtimef("读取序列号", "证书没有序列号: %s", certPath)
	}
	return strings.ToUpper(value), nil
}

// EndDate 返回证书到期时间与 openssl 的原始值（"Nov 14 20:06:48 2027 GMT"）。
//
// 原始值用于列表的"到期"列与 meta 的 EXPIRES 字段，必须逐字节兼容旧脚本。
func (o *OpenSSL) EndDate(ctx context.Context, certPath string) (time.Time, string, error) {
	return o.dateField(ctx, certPath, "-enddate", "notAfter=")
}

// StartDate 返回证书生效时间与原始值。
func (o *OpenSSL) StartDate(ctx context.Context, certPath string) (time.Time, string, error) {
	return o.dateField(ctx, certPath, "-startdate", "notBefore=")
}

// dateField 解析 openssl x509 的 -startdate / -enddate 输出。
func (o *OpenSSL) dateField(ctx context.Context, certPath, flag, prefix string) (time.Time, string, error) {
	res, err := o.run(ctx, "x509", "-in", certPath, "-noout", flag)
	if err != nil {
		return time.Time{}, "", err
	}
	raw := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(res.Stdout), prefix))
	if raw == "" {
		return time.Time{}, "", runtimef("解析证书时间", "%s 输出为空: %s", flag, certPath)
	}
	// openssl 使用 "Jan _2 15:04:05 2006 GMT"（日为单位数时前面补空格）。
	t, err := time.Parse("Jan _2 15:04:05 2006 MST", raw)
	if err != nil {
		return time.Time{}, "", runtimef("解析证书时间", "%q: %v", raw, err)
	}
	return t, raw, nil
}

// Subject 返回 openssl 默认格式的 subject（去掉 "subject=" 前缀）。
//
// 默认格式形如 "C = CN, O = ExampleOrg, CN = TMP01"，index.txt 的
// subject 列用的就是它，因此 index sync 必须使用同一格式。
func (o *OpenSSL) Subject(ctx context.Context, certPath string) (string, error) {
	res, err := o.run(ctx, "x509", "-in", certPath, "-noout", "-subject")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(res.Stdout), "subject=")), nil
}

// CN 从证书 subject 提取 Common Name。
//
// 使用 -nameopt sep_multiline（开发文档 §8.2），输出形如：
//
//	subject=
//	    C=CN
//	    O=ExampleOrg
//	    CN=TMP01
//
// 逐行解析避免默认格式里逗号与空格带来的歧义。
func (o *OpenSSL) CN(ctx context.Context, certPath string) (string, error) {
	res, err := o.run(ctx, "x509", "-in", certPath, "-noout", "-subject", "-nameopt", "sep_multiline")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "CN=") {
			return strings.TrimPrefix(line, "CN="), nil
		}
	}
	return "", nil
}

// FingerprintSHA256 返回 SHA256 指纹（大写、冒号分隔，如 "AB:CD:..."）。
func (o *OpenSSL) FingerprintSHA256(ctx context.Context, certPath string) (string, error) {
	res, err := o.run(ctx, "x509", "-in", certPath, "-noout", "-fingerprint", "-sha256")
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(res.Stdout)
	if idx := strings.Index(value, "="); idx >= 0 {
		value = strings.TrimSpace(value[idx+1:])
	}
	if value == "" {
		return "", runtimef("读取指纹", "%s 输出为空", certPath)
	}
	return value, nil
}

// SAN 返回去空格、逗号连接的 Subject Alternative Name，复刻旧脚本
// 的 "tail -n +2 | tr -d ' ' | paste -sd," 行为；无扩展时返回空字符串。
func (o *OpenSSL) SAN(ctx context.Context, certPath string) (string, error) {
	res, err := o.run(ctx, "x509", "-in", certPath, "-noout", "-ext", "subjectAltName")
	if err != nil {
		// 没有扩展时 openssl 以非零退出并在 stderr 报 "No extensions"；
		// 旧脚本用 2>/dev/null 把它当作空值，这里保持一致。
		return "", nil
	}
	lines := strings.Split(strings.TrimRight(res.Stdout, "\r\n"), "\n")
	if len(lines) <= 1 {
		return "", nil
	}
	parts := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		item := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
		if item != "" {
			parts = append(parts, item)
		}
	}
	return strings.Join(parts, ","), nil
}

// CRLRevokedCount 统计 CRL 中的吊销条目数（"Serial Number" 出现次数）。
//
// 与旧脚本一致：CRL 缺失或解析失败时返回 0，不阻断列表命令。
func (o *OpenSSL) CRLRevokedCount(ctx context.Context, crlPath string) (int, error) {
	res, err := o.run(ctx, "crl", "-in", crlPath, "-noout", "-text")
	if err != nil {
		return 0, nil
	}
	return strings.Count(res.Stdout, "Serial Number"), nil
}

// PKCS12Info 返回 pkcs12 -info 的输出（stdout+stderr 合并，取前 8 行）。
//
// 旧脚本把错误文本也并入 info 继续判断，这里保持宽容：不因密码错误
// 直接失败，而是让 verify 输出与旧脚本一致的告警。
func (o *OpenSSL) PKCS12Info(ctx context.Context, p12Path, passFile string) (string, error) {
	res, _ := o.run(ctx, "pkcs12", "-in", p12Path, "-passin", "file:"+passFile, "-noout", "-info")
	out := res.Stdout + res.Stderr
	lines := strings.Split(strings.TrimRight(out, "\r\n"), "\n")
	if len(lines) > 8 {
		lines = lines[:8]
	}
	return strings.Join(lines, "\n"), nil
}

// PKCS12CertCount 统计 p12 内的证书数量；失败时返回 0（旧脚本行为）。
func (o *OpenSSL) PKCS12CertCount(ctx context.Context, p12Path, passFile string) (int, error) {
	res, err := o.run(ctx, "pkcs12", "-in", p12Path, "-passin", "file:"+passFile, "-nokeys")
	if err != nil {
		return 0, nil
	}
	return strings.Count(res.Stdout, "BEGIN CERTIFICATE"), nil
}

// IndexExpiry 把证书到期时间格式化为 index.txt 的 YYMMDDHHMMSSZ（UTC）。
func IndexExpiry(t time.Time) string {
	return t.UTC().Format("060102150405Z")
}

// daysUntil 返回剩余整天数，与旧脚本 $(( (e - now) / 86400 )) 一致
// （向零截断，已过期时为负数）。
func daysUntil(t time.Time, now time.Time) int64 {
	return (t.Unix() - now.Unix()) / 86400
}
