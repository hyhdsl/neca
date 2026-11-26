// 本文件提供 Web 端只读证书数据。
package core

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hyhdsl/neca/internal/core/ports"
)

// WebCert 是 Web 面板使用的只读证书视图。
//
// 与 CLI 的 CertInfo 不同，它只从 clients/<CN>/<CN>.crt 读取证书副本，
// 不扫描 pki 根目录；这样 Web 容器可以完全不挂载 ca.key 与客户端私钥。
// Status 使用稳定英文值，StatusText 才用于页面展示。
type WebCert struct {
	CN          string    `json:"cn"`
	Serial      string    `json:"serial"`
	Status      string    `json:"status"`
	StatusText  string    `json:"statusText"`
	Platforms   []string  `json:"platforms"`
	Expires     string    `json:"expires"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Subject     string    `json:"subject,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	SAN         string    `json:"san,omitempty"`
	Note        string    `json:"note,omitempty"`
}

// WebCertList 是 Web 列表结果。
type WebCertList struct {
	Certs      []WebCert `json:"certs"`
	CRLRevoked int       `json:"crlRevoked"`
}

// ListWebCerts 扫描 clients 目录并关联 index.txt 状态。
func ListWebCerts(ctx context.Context, cfg *Config, store ports.IndexStore, oss *OpenSSL) (*WebCertList, error) {
	rows, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}
	index := make(map[string]ports.Row, len(rows))
	for _, row := range rows {
		if _, exists := index[strings.ToUpper(row.Serial)]; !exists {
			index[strings.ToUpper(row.Serial)] = row
		}
	}

	entries, err := os.ReadDir(cfg.ClientsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return &WebCertList{Certs: []WebCert{}}, nil
	}
	if err != nil {
		return nil, runtimef("扫描客户端目录", "%s: %v", cfg.ClientsDir, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	result := &WebCertList{Certs: []WebCert{}}
	for _, name := range names {
		certPath := filepath.Join(cfg.ClientsDir, name, name+".crt")
		if _, err := os.Stat(certPath); err != nil {
			continue
		}
		cert, err := webCertFromFile(ctx, cfg, index, oss, certPath)
		if err != nil {
			return nil, err
		}
		result.Certs = append(result.Certs, cert)
	}
	result.CRLRevoked, _ = oss.CRLRevokedCount(ctx, cfg.CRLFile)
	return result, nil
}

// GetWebCert 返回单个证书详情；CN 必须是合法的客户端目录名。
func GetWebCert(ctx context.Context, cfg *Config, store ports.IndexStore, oss *OpenSSL, cn string) (*WebCert, error) {
	if err := validateCN(cn); err != nil {
		return nil, err
	}
	certPath := filepath.Join(cfg.ClientsDir, cn, cn+".crt")
	if _, err := os.Stat(certPath); err != nil {
		return nil, preconditionf("读取证书", "找不到客户端证书: %s", cn)
	}
	rows, err := store.Load(ctx)
	if err != nil {
		return nil, err
	}
	index := make(map[string]ports.Row, len(rows))
	for _, row := range rows {
		if _, exists := index[strings.ToUpper(row.Serial)]; !exists {
			index[strings.ToUpper(row.Serial)] = row
		}
	}
	cert, err := webCertFromFile(ctx, cfg, index, oss, certPath)
	if err != nil {
		return nil, err
	}
	if cert.Subject, err = oss.Subject(ctx, certPath); err != nil {
		return nil, err
	}
	if cert.Fingerprint, err = oss.FingerprintSHA256(ctx, certPath); err != nil {
		return nil, err
	}
	if cert.SAN, err = oss.SAN(ctx, certPath); err != nil {
		return nil, err
	}
	return &cert, nil
}

// VerifyWebCert 复用 CLI 的 p12 检查逻辑。
func VerifyWebCert(ctx context.Context, cfg *Config, oss *OpenSSL, cn string) (*VerifyReport, error) {
	if err := validateCN(cn); err != nil {
		return nil, err
	}
	result, err := VerifyClients(ctx, cfg, oss, []string{cn})
	if err != nil {
		return nil, err
	}
	if len(result.Reports) == 0 {
		return nil, preconditionf("检查 p12", "没有可检查的客户端: %s", cn)
	}
	return &result.Reports[0], nil
}

// ValidateCNName 供 Web 路由在进入 core 之前校验 URL 参数。
func ValidateCNName(cn string) error {
	return validateCN(cn)
}

func webCertFromFile(ctx context.Context, cfg *Config, index map[string]ports.Row, oss *OpenSSL, certPath string) (WebCert, error) {
	cn, err := oss.CN(ctx, certPath)
	if err != nil {
		return WebCert{}, err
	}
	if cn == "" {
		cn = filepath.Base(filepath.Dir(certPath))
	}
	serial, err := oss.Serial(ctx, certPath)
	if err != nil {
		return WebCert{}, err
	}
	expiresAt, rawEnd, err := oss.EndDate(ctx, certPath)
	if err != nil {
		return WebCert{}, err
	}
	status, statusText := "unindexed", "未索引"
	if row, ok := index[strings.ToUpper(serial)]; ok {
		switch row.Status {
		case ports.IndexValid:
			status, statusText = "valid", "有效"
		case ports.IndexRevoked:
			status, statusText = "revoked", "已吊销"
		case ports.IndexExpired:
			status, statusText = "expired", "已过期"
		}
	}
	platforms := metaPlatforms(cfg.MetaDir, cn)
	if len(platforms) == 0 {
		platforms = []string{}
	}
	return WebCert{
		CN: cn, Serial: serial, Status: status, StatusText: statusText,
		Platforms: platforms, Expires: strings.TrimSuffix(rawEnd, " GMT"),
		ExpiresAt: expiresAt.UTC(),
	}, nil
}
