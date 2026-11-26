// 本文件定义配置来源端口接口。
package ports

import (
	"context"
	"fmt"
)

// Source 是一层配置来源，是配置系统的扩展点。
//
// 典型实现：环境变量（M1 的首要来源）、配置文件（--config）、未来 Web 面板
// 的设置或数据库覆盖。一个 Source 只负责读取自己那层，不填充默认值、
// 不做类型校验——默认值与校验属于 core/config 的职责。
type Source interface {
	// Name 返回稳定、可用于日志与错误信息的来源名，例如 "env"、"file"。
	Name() string
	// Load 返回本层配置的 key/value。key 与旧脚本环境变量名保持一致
	// （PKI_DIR、CRL_FILE 等，开发文档 §7）；返回错误时应说明来源。
	Load(ctx context.Context) (map[string]string, error)
}

// Resolver 按注册顺序合并多层配置，先注册的来源优先。
//
// 例如 NewResolver(envSource, fileSource) 表示环境变量覆盖配置文件，
// 与开发文档 §7 的"环境变量优先、否则默认值"一致。Resolver 不修改任何
// 输入，也不填充默认值——默认值由调用方在合并结果上补齐。
type Resolver struct {
	sources []Source
}

// NewResolver 创建配置合并器，传入顺序即优先级顺序。
// nil 来源会被忽略，便于测试或可选配置层直接传 nil。
func NewResolver(sources ...Source) *Resolver {
	filtered := make([]Source, 0, len(sources))
	for _, src := range sources {
		if src != nil {
			filtered = append(filtered, src)
		}
	}
	return &Resolver{sources: filtered}
}

// Resolve 依次加载所有来源并合并；先出现的 key 不会被后者覆盖。
//
// 任一来源失败时立即返回带来源名的错误，避免"半套配置"被静默使用。
// 返回的 map 归调用方所有，可安全修改。
func (r *Resolver) Resolve(ctx context.Context) (map[string]string, error) {
	merged := make(map[string]string)
	for _, src := range r.sources {
		values, err := src.Load(ctx)
		if err != nil {
			return nil, fmt.Errorf("config source %q: %w", src.Name(), err)
		}
		for key, value := range values {
			if _, exists := merged[key]; exists {
				continue // 先注册的来源优先
			}
			merged[key] = value
		}
	}
	return merged, nil
}
