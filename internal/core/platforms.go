package core

import (
	"fmt"
	"strings"
)

// 平台规范名。
const (
	PlatformWindows = "windows"
	PlatformLinux   = "linux"
	PlatformMacOS   = "macos"
	PlatformAndroid = "android"
	PlatformIOS     = "ios"
)

// platformAliases 兼容旧脚本与常见写法。
var platformAliases = map[string]string{
	"win": "windows", "windows": "windows", "w": "windows",
	"linux": "linux", "gnu/linux": "linux",
	"mac": "macos", "macos": "macos", "darwin": "macos", "osx": "macos",
	"and": "android", "android": "android", "a": "android", "安卓": "android",
	"ios": "ios", "apple": "ios", "i": "ios", "苹果": "ios",
}

// NormalizePlatform 返回规范平台名。
func NormalizePlatform(raw string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return "", fmt.Errorf("平台名不能为空")
	}
	if platform, ok := platformAliases[key]; ok {
		return platform, nil
	}
	return "", fmt.Errorf("未知平台: %q（可选 windows/linux/macos/android/ios）", raw)
}

// NormalizePlatforms 解析并去重平台列表，保持首次出现顺序。
func NormalizePlatforms(raw []string) ([]string, error) {
	seen := make(map[string]bool)
	var platforms []string
	for _, item := range raw {
		for _, part := range strings.Split(item, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			platform, err := NormalizePlatform(part)
			if err != nil {
				return nil, err
			}
			if !seen[platform] {
				seen[platform] = true
				platforms = append(platforms, platform)
			}
		}
	}
	if len(platforms) == 0 {
		return nil, fmt.Errorf("至少需要一个平台")
	}
	return platforms, nil
}

// DefaultPlatforms 是旧脚本的默认平台列表。
func DefaultPlatforms() []string {
	return []string{PlatformWindows, PlatformAndroid}
}

// PlatformDir 返回平台包目录名。
func PlatformDir(platform string) string {
	switch platform {
	case PlatformWindows:
		return "Windows"
	case PlatformLinux:
		return "Linux"
	case PlatformMacOS:
		return "macOS"
	case PlatformAndroid:
		return "Android"
	case PlatformIOS:
		return "iOS"
	default:
		return platform
	}
}
