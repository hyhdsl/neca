package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hyhdsl/neca/internal/core"
	"github.com/hyhdsl/neca/internal/version"
	"github.com/hyhdsl/neca/internal/web"
)

func main() {
	configPath := flag.String("config", "", "JSON 配置路径（默认 NECA_CONFIG 或 /opt/ipsec/neca.json）")
	healthcheck := flag.Bool("healthcheck", false, "检查本机 /healthz 后退出")
	showVersion := flag.Bool("version", false, "显示版本后退出")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Get().String())
		return
	}
	if *configPath != "" {
		if err := os.Setenv("NECA_CONFIG", *configPath); err != nil {
			log.Fatalf("设置 NECA_CONFIG: %v", err)
		}
	}

	cfg, err := core.LoadConfigReadOnly()
	if err != nil {
		log.Fatalf("加载配置: %v", err)
	}
	if cfg.Web.Validate() != nil {
		log.Fatalf("Web 配置无效: %v", cfg.Web.Validate())
	}
	if *healthcheck {
		if err := checkHealth(cfg.Web.Listen); err != nil {
			log.Fatal(err)
		}
		return
	}
	if !cfg.Web.Enabled {
		log.Fatal("web.enabled=false；neca-web 镜像不会在此配置下启动")
	}

	passwordHash, err := readSecret("NECA_WEB_PASSWORD_HASH", "NECA_WEB_PASSWORD_HASH_FILE")
	if err != nil {
		log.Fatal(err)
	}
	sessionKey, err := readSessionKey()
	if err != nil {
		log.Fatal(err)
	}
	proxySecret := strings.TrimSpace(os.Getenv("NECA_WEB_PROXY_SECRET"))
	audit, closeAudit, err := openAudit()
	if err != nil {
		log.Fatal(err)
	}
	defer closeAudit()

	runner := core.NewExecRunner()
	server, err := web.New(web.Options{
		Config: cfg, Store: core.NewFileIndexStore(cfg.IndexFile, cfg.LockFile),
		OpenSSL: core.NewOpenSSL(runner), PasswordHash: passwordHash, SessionKey: sessionKey,
		ProxySecret: proxySecret, SecureCookies: parseBool(os.Getenv("NECA_WEB_SECURE_COOKIES")),
		HelperSocket: strings.TrimSpace(os.Getenv("NECA_WEB_HELPER_SOCKET")),
		Audit:        audit,
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("necad %s listening on %s (readOnly=%t)", version.Get().Version, cfg.Web.Listen, cfg.Web.ReadOnly)
	if err := server.ListenAndServe(ctx); err != nil {
		log.Fatalf("Web 服务: %v", err)
	}
}

func checkHealth(listen string) error {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + listen + "/healthz")
	if err != nil {
		return fmt.Errorf("healthz: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: HTTP %d", resp.StatusCode)
	}
	return nil
}

func readSecret(envName, fileEnv string) ([]byte, error) {
	if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
		return []byte(value), nil
	}
	if path := strings.TrimSpace(os.Getenv(fileEnv)); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取 %s: %w", path, err)
		}
		return []byte(strings.TrimSpace(string(data))), nil
	}
	return nil, fmt.Errorf("必须设置 %s 或 %s", envName, fileEnv)
}

func readSessionKey() ([]byte, error) {
	if path := strings.TrimSpace(os.Getenv("NECA_WEB_SESSION_KEY_FILE")); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取会话密钥: %w", err)
		}
		key := strings.TrimSpace(string(data))
		if decoded, err := base64.StdEncoding.DecodeString(key); err == nil {
			key = string(decoded)
		}
		return []byte(key), nil
	}
	raw := strings.TrimSpace(os.Getenv("NECA_WEB_SESSION_KEY"))
	if raw == "" {
		return nil, errors.New("必须设置 NECA_WEB_SESSION_KEY 或 NECA_WEB_SESSION_KEY_FILE")
	}
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil {
		return decoded, nil
	}
	return []byte(raw), nil
}

func openAudit() (interface{ Write([]byte) (int, error) }, func(), error) {
	path := strings.TrimSpace(os.Getenv("NECA_WEB_AUDIT_LOG"))
	if path == "" {
		return os.Stderr, func() {}, nil
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, func() {}, fmt.Errorf("打开 Web 审计日志 %s: %w", path, err)
	}
	return file, func() { _ = file.Close() }, nil
}

func parseBool(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return parsed
}
