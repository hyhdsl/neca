// 本文件测试 Web 证书视图的导入元数据字段。
package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestListWebCertsIncludesImportMetadata(t *testing.T) {
	root := t.TempDir()
	clients := filepath.Join(root, "clients")
	meta := filepath.Join(root, "meta")
	for _, dir := range []string{filepath.Join(clients, "TMP01"), meta} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(clients, "TMP01", "TMP01.crt"), []byte("CERT"), 0o600); err != nil {
		t.Fatal(err)
	}
	metaFile := MetaPath(meta, "TMP01")
	if err := os.WriteFile(metaFile, []byte("CREATED=2026-11-27\nNOT_BEFORE=Nov 26 00:00:00 2026 GMT\nIMPORT_SOURCE=P12\nPLATFORMS=windows\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{ClientsDir: clients, MetaDir: meta, CRLFile: filepath.Join(root, "missing.crl")}
	store := NewFileIndexStore(filepath.Join(root, "index.txt"), filepath.Join(root, "lock"))
	list, err := ListWebCerts(context.Background(), cfg, store, NewOpenSSL(importFakeRunner(t, false)))
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Certs) != 1 {
		t.Fatalf("certs = %+v", list.Certs)
	}
	cert := list.Certs[0]
	if cert.Created != "2026-11-27" || cert.NotBefore != "Nov 26 00:00:00 2026 GMT" || cert.ImportSource != "p12" {
		t.Fatalf("import metadata = %+v", cert)
	}
}
