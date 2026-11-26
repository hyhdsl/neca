// 本文件生成各平台安装脚本。
package core

import (
	"fmt"
	"strings"
)

// InstallerSet 表示每个平台是否生成一键安装脚本。
type InstallerSet map[string]bool

// ParseInstallers 解析 --installers / NECA_INSTALLERS：
// none（默认）、all、或 windows,linux,macos 列表。
func ParseInstallers(raw string) (InstallerSet, error) {
	set := InstallerSet{}
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || raw == "none" {
		return set, nil
	}
	if raw == "all" {
		set[PlatformWindows] = true
		set[PlatformLinux] = true
		set[PlatformMacOS] = true
		return set, nil
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		platform, err := NormalizePlatform(part)
		if err != nil {
			return nil, err
		}
		switch platform {
		case PlatformWindows, PlatformLinux, PlatformMacOS:
			set[platform] = true
		default:
			return nil, fmt.Errorf("平台 %s 不支持一键脚本", platform)
		}
	}
	return set, nil
}

// Enabled 判断某平台是否启用一键脚本。
func (s InstallerSet) Enabled(platform string) bool {
	return s != nil && s[platform]
}
