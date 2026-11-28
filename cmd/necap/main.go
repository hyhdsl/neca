// 本文件启动 necap 认证服务，并提供离线初始化命令。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hyhdsl/neca/internal/necap"
	"github.com/hyhdsl/neca/internal/version"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runInit(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}
	runServe()
}

func runServe() {
	configPath := flag.String("config", "", "necap JSON 配置路径")
	showVersion := flag.Bool("version", false, "显示版本后退出")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.Get().String())
		return
	}
	if *configPath != "" {
		if err := os.Setenv("NECAP_CONFIG", *configPath); err != nil {
			log.Fatalf("设置 NECAP_CONFIG: %v", err)
		}
	}
	cfg, err := necap.LoadConfig()
	if err != nil {
		log.Fatalf("加载 necap 配置: %v", err)
	}
	key, err := necap.LoadOrCreateMasterKey(cfg.MasterKeyFile)
	if err != nil {
		log.Fatalf("加载 necap 主密钥: %v", err)
	}
	store, err := necap.OpenStore(context.Background(), cfg.DatabasePath, key)
	if err != nil {
		log.Fatalf("打开 necap 数据库: %v", err)
	}
	defer store.Close()
	auth, err := necap.NewAuthService(cfg, store)
	if err != nil {
		log.Fatalf("初始化 necap 认证: %v", err)
	}
	server := necap.NewServer(auth)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("necap %s listening on unix:%s", version.Get().Version, cfg.SocketPath)
	if err := server.ListenAndServe(ctx, cfg.SocketPath); err != nil {
		log.Fatalf("necap: %v", err)
	}
}

func runInit(args []string) {
	fs := flag.NewFlagSet("necap init", flag.ExitOnError)
	configPath := fs.String("config", necap.DefaultConfigPath, "necap JSON 配置路径")
	rpID := fs.String("rp-id", "", "WebAuthn RP ID（必填）")
	rpName := fs.String("rp-name", "neca Admin", "WebAuthn RP 显示名")
	origins := fs.String("origins", "", "逗号分隔的 Origin 列表（必填）")
	database := fs.String("database", necap.DefaultDatabasePath, "SQLite 数据库路径")
	keyFile := fs.String("master-key-file", necap.DefaultMasterKeyPath, "TOTP 主密钥路径")
	socket := fs.String("socket", necap.DefaultSocketPath, "Unix socket 路径")
	username := fs.String("username", "admin", "管理员用户名")
	hashFile := fs.String("password-hash-file", "/opt/ipsec/web-secrets/password.hash", "现有 bcrypt 密码哈希文件")
	uid := fs.Int("uid", -1, "生成文件的属主 UID（运行 necap 的用户）")
	gid := fs.Int("gid", -1, "生成文件的属组 GID")
	fs.Parse(args)

	if strings.TrimSpace(*rpID) == "" || strings.TrimSpace(*origins) == "" {
		log.Fatal("necap init 需要 --rp-id 与 --origins")
	}
	hash, err := os.ReadFile(*hashFile)
	if err != nil {
		log.Fatalf("读取密码哈希文件: %v", err)
	}
	hash = []byte(strings.TrimSpace(string(hash)))
	if _, err := bcrypt.Cost(hash); err != nil {
		log.Fatalf("密码哈希文件不是有效 bcrypt: %v", err)
	}
	cfg := necap.Config{
		SchemaVersion: necap.CurrentConfigSchemaVersion,
		RPID:          *rpID, RPName: *rpName,
		Origins:          strings.Split(*origins, ","),
		UserVerification: "preferred",
		DatabasePath:     *database, MasterKeyFile: *keyFile, SocketPath: *socket,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	data = append(data, '\n')
	if _, err := necap.ParseConfig(data, *configPath); err != nil {
		log.Fatalf("校验 necap 配置: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*configPath), 0o700); err != nil {
		log.Fatalf("创建 necap 配置目录: %v", err)
	}
	if err := os.Chmod(filepath.Dir(*configPath), 0o700); err != nil {
		log.Fatalf("设置 necap 配置目录权限: %v", err)
	}
	file, err := os.OpenFile(*configPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		log.Fatalf("创建 necap 配置: %v", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		log.Fatalf("写入 necap 配置: %v", err)
	}
	if err := file.Close(); err != nil {
		log.Fatalf("关闭 necap 配置: %v", err)
	}
	key, err := necap.LoadOrCreateMasterKey(*keyFile)
	if err != nil {
		log.Fatalf("创建主密钥: %v", err)
	}
	store, err := necap.OpenStore(context.Background(), *database, key)
	if err != nil {
		log.Fatalf("创建 necap 数据库: %v", err)
	}
	defer store.Close()
	count, err := store.CountUsers(context.Background())
	if err != nil {
		log.Fatalf("检查 necap 用户: %v", err)
	}
	if count != 0 {
		log.Fatal("necap 数据库已有用户，拒绝重复初始化")
	}
	user, err := store.CreateUser(context.Background(), *username, hash, "admin")
	if err != nil {
		log.Fatalf("创建 necap 管理员: %v", err)
	}
	if *uid >= 0 || *gid >= 0 {
		if os.Geteuid() != 0 {
			log.Fatal("指定 --uid/--gid 时需要 root 运行 necap init")
		}
		if *uid < 0 {
			*uid = 10002
		}
		if *gid < 0 {
			*gid = *uid
		}
		for _, path := range []string{filepath.Dir(*configPath), *configPath, *keyFile, *database, *database + "-wal", *database + "-shm"} {
			if _, err := os.Stat(path); err != nil {
				continue
			}
			if err := os.Chown(path, *uid, *gid); err != nil {
				log.Fatalf("设置 %s 属主: %v", path, err)
			}
		}
	}
	fmt.Printf("[OK]   necap 初始化完成: config=%s db=%s user=%s\n", *configPath, *database, user.Username)
}
