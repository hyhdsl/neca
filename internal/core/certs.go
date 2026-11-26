// 本文件读取和解析 Web 证书元数据。
package core

import (
	"context"
	"path/filepath"
	"sort"
)

// isManagedCert 判断文件名是否是 neca 管理的客户端证书。
// CA 证书与服务器证书手工维护，列表/导出/同步都必须跳过（开发文档 §1.3）。
func isManagedCert(base string) bool {
	return base != "ca.crt" && base != "server.crt"
}

// listCertFiles 返回目录下按文件名排序的 .crt 文件；目录不存在时返回空。
func listCertFiles(dir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.crt"))
	if err != nil {
		return nil, runtimef("扫描证书", "%s: %v", dir, err)
	}
	sort.Strings(paths)
	return paths, nil
}

// scanSerialPaths 扫描多个目录的证书，建立"序列号 -> 证书路径"映射。
//
// 目录顺序即优先级（归档在前、当前 PKI 在后），同一序列号只保留首个路径，
// 与旧脚本 find_cert_by_serial 的扫描顺序一致。openssl 读取失败的证书跳过。
func scanSerialPaths(ctx context.Context, oss *OpenSSL, dirs ...string) (map[string]string, error) {
	paths := make(map[string]string)
	for _, dir := range dirs {
		files, err := listCertFiles(dir)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			serial, err := oss.Serial(ctx, file)
			if err != nil {
				continue
			}
			if _, exists := paths[serial]; !exists {
				paths[serial] = file
			}
		}
	}
	return paths, nil
}
