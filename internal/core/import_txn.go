// 本文件实现 cert 导入的小型事务标记与启动恢复。
package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hyhdsl/neca/internal/core/ports"
)

// importTxnState 是 .txn/import-*.json 的内容；不保存密钥或口令。
type importTxnState struct {
	CN      string `json:"cn"`
	Serial  string `json:"serial"`
	Phase   string `json:"phase"`
	Created string `json:"created"`
}

type importTxn struct {
	path  string
	state importTxnState
}

func importTxnDir(cfg *Config) string {
	return filepath.Join(cfg.PKIDir, ".txn")
}

func beginImportTxn(cfg *Config, cn, serial string) (*importTxn, error) {
	dir := importTxnDir(cfg)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, runtimef("创建导入事务目录", "%v", err)
	}
	file, err := os.CreateTemp(dir, "import-*.json")
	if err != nil {
		return nil, runtimef("创建导入事务标记", "%v", err)
	}
	path := file.Name()
	_ = file.Close()
	_ = os.Chmod(path, 0o600)
	txn := &importTxn{path: path, state: importTxnState{
		CN: cn, Serial: serial, Phase: "prepared", Created: time.Now().UTC().Format(time.RFC3339),
	}}
	if err := txn.write(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return txn, nil
}

func (t *importTxn) write() error {
	data, err := json.Marshal(t.state)
	if err != nil {
		return runtimef("编码导入事务", "%v", err)
	}
	dir := filepath.Dir(t.path)
	tmp, err := os.CreateTemp(dir, ".txn-*.tmp")
	if err != nil {
		return runtimef("写入导入事务", "%v", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return runtimef("写入导入事务", "%v", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return runtimef("同步导入事务", "%v", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return runtimef("关闭导入事务", "%v", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return runtimef("设置导入事务权限", "%v", err)
	}
	if err := os.Rename(tmpPath, t.path); err != nil {
		_ = os.Remove(tmpPath)
		return runtimef("替换导入事务", "%v", err)
	}
	return nil
}

func (t *importTxn) setPhase(phase string) error {
	t.state.Phase = phase
	return t.write()
}

func (t *importTxn) commit() error {
	return t.setPhase("committed")
}

func (t *importTxn) remove() error {
	if t == nil || t.path == "" {
		return nil
	}
	if err := os.Remove(t.path); err != nil && !os.IsNotExist(err) {
		return runtimef("删除导入事务", "%v", err)
	}
	return nil
}

// PendingImportTransaction 返回未完成导入事务的诊断错误。
func PendingImportTransaction(cfg *Config) error {
	return checkPendingImportTxn(cfg)
}

// checkPendingImportTxn 阻止只读校验在未知中间态上继续；实际导入会先恢复。
func checkPendingImportTxn(cfg *Config) error {
	paths, err := filepath.Glob(filepath.Join(importTxnDir(cfg), "import-*.json"))
	if err != nil {
		return runtimef("检查导入事务", "%v", err)
	}
	if len(paths) > 0 {
		return preconditionf("导入证书", "存在未完成导入事务，请先执行实际导入或 doctor 恢复: %s", filepath.Base(paths[0]))
	}
	return nil
}

// RecoverImportTransactions 获取 PKI 锁后回滚未提交事务；committed 只清理标记。
func RecoverImportTransactions(ctx context.Context, cfg *Config, store *FileIndexStore) error {
	return withPkiLock(cfg.LockFile, func() error {
		return recoverImportTransactionsLocked(ctx, cfg, store)
	})
}

func recoverImportTransactionsLocked(ctx context.Context, cfg *Config, store *FileIndexStore) error {
	paths, err := filepath.Glob(filepath.Join(importTxnDir(cfg), "import-*.json"))
	if err != nil {
		return runtimef("读取导入事务", "%v", err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return runtimef("读取导入事务", "%s: %v", filepath.Base(path), err)
		}
		var state importTxnState
		if err := json.Unmarshal(data, &state); err != nil {
			return runtimef("解析导入事务", "%s: %v", filepath.Base(path), err)
		}
		if strings.EqualFold(state.Phase, "committed") {
			_ = os.Remove(path)
			continue
		}
		if err := validateCN(state.CN); err != nil {
			return runtimef("恢复导入事务", "%s: %v", filepath.Base(path), err)
		}
		_ = os.Remove(filepath.Join(cfg.PKIDir, state.CN+".crt"))
		_ = os.Remove(filepath.Join(cfg.PKIDir, state.CN+".key"))
		_ = os.RemoveAll(filepath.Join(cfg.ClientsDir, state.CN))
		_ = os.Remove(MetaPath(cfg.MetaDir, state.CN))
		if state.Serial != "" {
			if err := store.UpdateLocked(ctx, func(idx *ports.Index) error {
				kept := idx.Rows[:0]
				for _, row := range idx.Rows {
					if !strings.EqualFold(row.Serial, state.Serial) {
						kept = append(kept, row)
					}
				}
				idx.Rows = kept
				return nil
			}); err != nil {
				return runtimef("恢复导入索引", "%s: %v", filepath.Base(path), err)
			}
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return runtimef("清理导入事务", "%s: %v", filepath.Base(path), err)
		}
	}
	return nil
}
