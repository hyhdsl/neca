package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/hyhdsl/neca/templates"
)

// BundleResult 是平台包生成结果；Messages 供 CLI 沿用旧脚本的 [OK] 文案。
type BundleResult struct {
	Platforms []string
	Messages  []string
	Warnings  []string
}

// RenderBundles 为 CN 生成选中平台包。
//
// 前置条件：clients/<CN>/<CN>.p12、pass、ca.crt 已由 p12 导出步骤生成。
// 一键安装脚本默认不生成；Windows/Linux/macOS 脚本需 InstallerSet 显式开启。
// Linux 生成 install.sh/uninstall.sh；macOS 生成 install.command/uninstall.command
// 与可选的 mobileconfig，脚本本身保持 ASCII。
func RenderBundles(ctx context.Context, cfg *Config, cn string, platforms []string, installers InstallerSet) (*BundleResult, error) {
	base := filepath.Join(cfg.ClientsDir, cn)
	p12Path := filepath.Join(base, cn+".p12")
	passPath := filepath.Join(base, cn+".p12.pass")
	caPath := filepath.Join(base, "ca.crt")
	for _, path := range []string{p12Path, passPath, caPath} {
		if _, err := os.Stat(path); err != nil {
			return nil, preconditionf("生成平台包", "缺少公共文件 %s: %v", path, err)
		}
	}

	result := &BundleResult{}
	for _, platform := range platforms {
		switch platform {
		case PlatformIOS:
			dir := filepath.Join(base, PlatformDir(platform))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, runtimef("生成平台包", "%s: %v", dir, err)
			}
			if err := writeIOSProfile(ctx, cfg, cn, dir, p12Path, passPath, caPath); err != nil {
				return nil, err
			}
			result.Messages = append(result.Messages, fmt.Sprintf("iOS 包: %s", dir))
			result.Platforms = append(result.Platforms, platform)
		case PlatformWindows, PlatformLinux, PlatformMacOS, PlatformAndroid:
			dir := filepath.Join(base, PlatformDir(platform))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, runtimef("生成平台包", "%s: %v", dir, err)
			}
			if err := copyBundleCommon(p12Path, passPath, caPath, dir); err != nil {
				return nil, err
			}
			switch platform {
			case PlatformWindows:
				if err := writeWindowsInstructions(dir, cn); err != nil {
					return nil, err
				}
				if installers.Enabled(PlatformWindows) {
					if err := renderWindowsScripts(dir, cn, cfg); err != nil {
						return nil, err
					}
				}
				result.Messages = append(result.Messages, fmt.Sprintf("Windows 包: %s", dir))
			case PlatformAndroid:
				if err := writeAndroidInstructions(dir, cn, cfg.ServerID); err != nil {
					return nil, err
				}
				result.Messages = append(result.Messages, fmt.Sprintf("Android 包: %s", dir))
			case PlatformLinux:
				hasInstaller := installers.Enabled(PlatformLinux)
				if err := writeLinuxInstructions(dir, cn, cfg.ServerID, hasInstaller); err != nil {
					return nil, err
				}
				if hasInstaller {
					if err := renderUnixScripts(dir, PlatformLinux, cn, cfg); err != nil {
						return nil, err
					}
				}
				result.Messages = append(result.Messages, fmt.Sprintf("Linux 包: %s", dir))
			case PlatformMacOS:
				hasInstaller := installers.Enabled(PlatformMacOS)
				if err := writeMacInstructions(dir, cn, cfg.ServerID, hasInstaller); err != nil {
					return nil, err
				}
				if hasInstaller {
					if err := writeIOSProfile(ctx, cfg, cn, dir, p12Path, passPath, caPath); err != nil {
						return nil, err
					}
					if err := renderUnixScripts(dir, PlatformMacOS, cn, cfg); err != nil {
						return nil, err
					}
				}
				result.Messages = append(result.Messages, fmt.Sprintf("macOS 包: %s", dir))
			}
			result.Platforms = append(result.Platforms, platform)
		default:
			result.Warnings = append(result.Warnings, "未知平台: "+platform)
		}
	}
	return result, nil
}

// copyBundleCommon 把公共文件复制到平台目录。
func copyBundleCommon(p12Path, passPath, caPath, dir string) error {
	files := []struct {
		src  string
		dst  string
		mode os.FileMode
	}{
		{p12Path, filepath.Join(dir, filepath.Base(p12Path)), 0o600},
		{passPath, filepath.Join(dir, filepath.Base(passPath)), 0o600},
		{caPath, filepath.Join(dir, "ca.crt"), 0o644},
	}
	for _, file := range files {
		if err := copyFileMode(file.src, file.dst, file.mode); err != nil {
			return err
		}
	}
	return nil
}

// copyFileMode 复制普通文件并设置权限。
func copyFileMode(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return runtimef("复制平台包文件", "%s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, mode); err != nil {
		return runtimef("写入平台包文件", "%s: %v", dst, err)
	}
	if err := os.Chmod(dst, mode); err != nil {
		return runtimef("设置平台包权限", "%s: %v", dst, err)
	}
	return nil
}

// renderUnixScripts 渲染 Linux/macOS 安装/卸载脚本（纯 ASCII，0755）。
func renderUnixScripts(dir, platform, cn string, cfg *Config) error {
	data := map[string]string{
		"CN":        cn,
		"Server":    cfg.ServerID,
		"CA":        cfg.CACN,
		"ProfileID": "work.example.vpn." + strings.ToLower(cn),
	}
	files := map[string]string{}
	switch platform {
	case PlatformLinux:
		files["install.sh"] = "linux/install.sh.tmpl"
		files["uninstall.sh"] = "linux/uninstall.sh.tmpl"
	case PlatformMacOS:
		files["install.command"] = "macos/install.command.tmpl"
		files["uninstall.command"] = "macos/uninstall.command.tmpl"
	default:
		return runtimef("生成平台脚本", "不支持平台 %s", platform)
	}
	for name, templatePath := range files {
		rendered, err := renderEmbeddedTemplate(templatePath, data)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(rendered), 0o755); err != nil {
			return runtimef("写入平台脚本", "%s: %v", path, err)
		}
		if err := os.Chmod(path, 0o755); err != nil {
			return runtimef("设置平台脚本权限", "%s: %v", path, err)
		}
	}
	return nil
}

// renderWindowsScripts 渲染 Windows PS1/cmd 脚本（纯 ASCII）。
func renderWindowsScripts(dir, cn string, cfg *Config) error {
	data := map[string]string{
		"CN":      cn,
		"Profile": "ExamplePanel-VPN-" + cn,
		"Server":  cfg.ServerID,
	}
	files := map[string]string{
		"install.ps1":   "win/install.ps1.tmpl",
		"uninstall.ps1": "win/uninstall.ps1.tmpl",
		"connect.ps1":   "win/connect.ps1.tmpl",
	}
	for name, templatePath := range files {
		rendered, err := renderEmbeddedTemplate(templatePath, data)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(rendered), 0o644); err != nil {
			return runtimef("写入 Windows 脚本", "%s: %v", name, err)
		}
	}
	cmdFiles := map[string]string{
		"安装-" + cn + ".cmd": "@echo off\nchcp 65001 >nul\npowershell -NoProfile -ExecutionPolicy Bypass -File \"%~dp0install.ps1\"\n",
		"卸载-" + cn + ".cmd": "@echo off\nchcp 65001 >nul\npowershell -NoProfile -ExecutionPolicy Bypass -File \"%~dp0uninstall.ps1\"\n",
		"连接-" + cn + ".cmd": "@echo off\nchcp 65001 >nul\npowershell -NoProfile -ExecutionPolicy Bypass -File \"%~dp0connect.ps1\"\n",
	}
	for name, content := range cmdFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			return runtimef("写入 Windows 脚本", "%s: %v", name, err)
		}
	}
	return nil
}

// renderEmbeddedTemplate 渲染 templates/ 中的模板。
func renderEmbeddedTemplate(name string, data any) (string, error) {
	raw, err := templates.FS.ReadFile(name)
	if err != nil {
		return "", runtimef("读取模板", "%s: %v", name, err)
	}
	tmpl, err := template.New(name).Parse(string(raw))
	if err != nil {
		return "", runtimef("解析模板", "%s: %v", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", runtimef("渲染模板", "%s: %v", name, err)
	}
	return buf.String(), nil
}

// writeWindowsInstructions 写 Windows 使用说明（中文，UTF-8）。
func writeWindowsInstructions(dir, cn string) error {
	content := "Example VPN (" + cn + ") — Windows 一键安装\n\n" +
		"1. 双击 安装-" + cn + ".cmd → UAC 点“是”（信任 CA、导入证书、创建配置 ExamplePanel-VPN-" + cn + "）。\n" +
		"2. 连接：双击 连接-" + cn + ".cmd（会先断开其它已连 VPN）。\n" +
		"3. 卸载：双击 卸载-" + cn + ".cmd（默认保留 CA；加 -RemoveCA 可一并删除）。\n" +
		"注意：Windows 同一时间只能连一个 VPN；脚本需与 p12、密码、ca.crt 同目录。\n"
	return writeInstructions(dir, content)
}

// writeAndroidInstructions 写 Android 使用说明。
func writeAndroidInstructions(dir, cn, serverID string) error {
	content := "Example VPN (" + cn + ") — Android 系统原生 IKEv2\n\n" +
		"1. 安装 CA：设置 → 密码与安全 → 系统安全 → 加密与凭据 → 安装证书 → CA 证书 → ca.crt\n" +
		"2. 安装用户证书：同页 → VPN 和应用用户证书 → " + cn + ".p12 → 输入 " + cn + ".p12.pass 里的密码\n" +
		"   （p12 默认 3DES 加密、只含客户端证书，密码为 12 位字母数字；CA 需单独安装）\n" +
		"3. 添加 VPN（IKEv2/IPSec RSA）：服务器地址与 IPSec 标识符都填 " + serverID + "；\n" +
		"   用户证书 " + cn + "；CA 证书 ExampleOrg VPN CA；服务器证书（来自服务器）\n" +
		"4. 连接。失败后开关点不动时重启手机再试。\n"
	return writeInstructions(dir, content)
}

// writeLinuxInstructions 写 Linux 使用说明。
func writeLinuxInstructions(dir, cn, serverID string, hasInstaller bool) error {
	note := "注意：当前包未包含一键脚本；请按说明手工配置，或重新生成并加 --installers linux。\n"
	if hasInstaller {
		note = "一键安装：sudo ./install.sh（需 root）；卸载：sudo ./uninstall.sh。\n"
	}
	content := "Example VPN (" + cn + ") — Linux (strongSwan / NetworkManager)\n\n" +
		"1. 信任 CA：install.sh 会把 ca.crt 放到 /etc/swanctl/x509ca/ 并更新系统 CA。\n" +
		"2. 导入客户端证书：脚本用 " + cn + ".p12.pass 解密 p12，写入 /etc/swanctl/x509 与 private。\n" +
		"3. 配置 IKEv2：服务器地址与远端标识填 " + serverID + "，本地标识填 " + cn + "。\n" +
		"4. 脚本会执行 swanctl --load-all；之后用系统网络界面或 swanctl 发起连接。\n" + note
	return writeInstructions(dir, content)
}

// writeMacInstructions 写 macOS 使用说明。
func writeMacInstructions(dir, cn, serverID string, hasInstaller bool) error {
	note := "注意：当前包未包含一键脚本；请按说明手工配置，或重新生成并加 --installers macos。\n"
	if hasInstaller {
		note = "一键安装：双击 install.command（会请求管理员权限并安装 mobileconfig）；卸载：双击 uninstall.command。\n"
	}
	content := "Example VPN (" + cn + ") — macOS IKEv2\n\n" +
		"1. install.command 会把 ca.crt 加入系统钥匙串并信任，导入 p12 客户端证书。\n" +
		"2. 再安装同目录的 " + cn + ".mobileconfig（IKEv2 配置）。\n" +
		"3. 服务器地址与远端标识：" + serverID + "；本地标识：" + cn + "；认证方式：证书。\n" +
		"4. 连接。\n" + note
	return writeInstructions(dir, content)
}

// writeInstructions 写 UTF-8 使用说明。
func writeInstructions(dir, content string) error {
	path := filepath.Join(dir, "使用说明.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return runtimef("写入使用说明", "%s: %v", path, err)
	}
	return nil
}

// writeIOSProfile 生成 iOS mobileconfig（手写 plist XML）。
func writeIOSProfile(ctx context.Context, cfg *Config, cn, dir, p12Path, passPath, caPath string) error {
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return runtimef("生成 iOS 包", "读取 CA: %v", err)
	}
	caDER, err := pemToDER(caPEM)
	if err != nil {
		return runtimef("生成 iOS 包", "CA PEM: %v", err)
	}
	p12, err := os.ReadFile(p12Path)
	if err != nil {
		return runtimef("生成 iOS 包", "读取 p12: %v", err)
	}
	pass, err := os.ReadFile(passPath)
	if err != nil {
		return runtimef("生成 iOS 包", "读取密码: %v", err)
	}
	password := strings.TrimSpace(string(pass))
	lid := "work.example.vpn." + strings.ToLower(cn)
	uidCA, uidID, uidVPN, uidTop := uuidV4(), uuidV4(), uuidV4(), uuidV4()
	xml := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
		"<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n" +
		"<plist version=\"1.0\"><dict>\n" +
		"<key>PayloadType</key><string>Configuration</string>\n" +
		"<key>PayloadVersion</key><integer>1</integer>\n" +
		"<key>PayloadIdentifier</key><string>" + xmlEscape(lid) + "</string>\n" +
		"<key>PayloadUUID</key><string>" + uidTop + "</string>\n" +
		"<key>PayloadDisplayName</key><string>Example VPN (" + xmlEscape(cn) + ")</string>\n" +
		"<key>PayloadOrganization</key><string>ExampleOrg</string>\n" +
		"<key>PayloadDescription</key><string>CA + client cert + IKEv2</string>\n" +
		"<key>PayloadRemovalDisallowed</key><false/>\n" +
		"<key>PayloadContent</key><array>\n" +
		vpnCA(caDER, lid, uidCA) +
		vpnIdentity(cn, p12, password, lid, uidID) +
		vpnProfile(cn, cfg.ServerID, lid, uidVPN, uidID) +
		"</array>\n</dict></plist>\n"
	path := filepath.Join(dir, cn+".mobileconfig")
	if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
		return runtimef("生成 iOS 包", "%s: %v", path, err)
	}
	return nil
}

func vpnCA(caDER []byte, lid, uid string) string {
	return "<dict>" +
		"<key>PayloadType</key><string>com.apple.security.root</string>" +
		"<key>PayloadVersion</key><integer>1</integer>" +
		"<key>PayloadIdentifier</key><string>" + xmlEscape(lid+".ca") + "</string>" +
		"<key>PayloadUUID</key><string>" + uid + "</string>" +
		"<key>PayloadDisplayName</key><string>ExampleOrg VPN CA</string>" +
		"<key>PayloadCertificateFileName</key><string>ca.crt</string>" +
		"<key>PayloadContent</key><data>" + base64.StdEncoding.EncodeToString(caDER) + "</data>" +
		"</dict>"
}

func vpnIdentity(cn string, p12 []byte, password, lid, uid string) string {
	return "<dict>" +
		"<key>PayloadType</key><string>com.apple.security.pkcs12</string>" +
		"<key>PayloadVersion</key><integer>1</integer>" +
		"<key>PayloadIdentifier</key><string>" + xmlEscape(lid+".id") + "</string>" +
		"<key>PayloadUUID</key><string>" + uid + "</string>" +
		"<key>PayloadDisplayName</key><string>" + xmlEscape(cn+" client cert") + "</string>" +
		"<key>PayloadCertificateFileName</key><string>" + xmlEscape(cn+".p12") + "</string>" +
		"<key>Password</key><string>" + xmlEscape(password) + "</string>" +
		"<key>PayloadContent</key><data>" + base64.StdEncoding.EncodeToString(p12) + "</data>" +
		"</dict>"
}

func vpnProfile(cn, server, lid, uid, idUID string) string {
	return "<dict>" +
		"<key>PayloadType</key><string>com.apple.vpn.managed</string>" +
		"<key>PayloadVersion</key><integer>1</integer>" +
		"<key>PayloadIdentifier</key><string>" + xmlEscape(lid+".vpn") + "</string>" +
		"<key>PayloadUUID</key><string>" + uid + "</string>" +
		"<key>PayloadDisplayName</key><string>Example VPN (" + xmlEscape(cn) + ")</string>" +
		"<key>UserDefinedName</key><string>Example VPN " + xmlEscape(cn) + "</string>" +
		"<key>VPNType</key><string>IKEv2</string>" +
		"<key>IKEv2</key><dict>" +
		"<key>AuthenticationMethod</key><string>Certificate</string>" +
		"<key>RemoteAddress</key><string>" + xmlEscape(server) + "</string>" +
		"<key>RemoteIdentifier</key><string>" + xmlEscape(server) + "</string>" +
		"<key>LocalIdentifier</key><string>" + xmlEscape(cn) + "</string>" +
		"<key>PayloadCertificateUUID</key><string>" + idUID + "</string>" +
		"<key>ServerCertificateIssuerCommonName</key><string>ExampleOrg VPN CA</string>" +
		"<key>ServerCertificateCommonName</key><string>" + xmlEscape(server) + "</string>" +
		"<key>DeadPeerDetectionRate</key><string>Medium</string>" +
		"</dict>" +
		"<key>IPv4</key><dict><key>OverridePrimary</key><integer>0</integer></dict>" +
		"</dict>"
}

// pemToDER 解出 PEM 中的第一张证书 DER。
func pemToDER(data []byte) ([]byte, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("不是 PEM 证书")
	}
	return block.Bytes, nil
}

// uuidV4 生成大写 UUID v4。
func uuidV4() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return strings.ToUpper(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]))
}

// xmlEscape 转义 XML 文本。
func xmlEscape(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;")
	return replacer.Replace(s)
}
