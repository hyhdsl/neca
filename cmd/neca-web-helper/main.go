// 本文件启动 neca Web 的 root helper 进程，提供受控写操作代理。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/hyhdsl/neca/internal/core"
	"github.com/hyhdsl/neca/internal/helper"
	"github.com/hyhdsl/neca/internal/version"
)

func main() {
	configPath := flag.String("config", "", "JSON 配置路径")
	socket := flag.String("socket", "", "Unix socket 路径")
	gid := flag.Int("gid", helper.DefaultSocketGID, "socket 属组 GID")
	showVersion := flag.Bool("version", false, "显示版本后退出")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Get().String())
		return
	}
	if os.Geteuid() != 0 {
		log.Fatal("neca-web-helper 必须以 root 运行")
	}
	if *configPath != "" {
		if err := os.Setenv("NECA_CONFIG", *configPath); err != nil {
			log.Fatalf("设置 NECA_CONFIG: %v", err)
		}
	}
	if *socket == "" {
		*socket = strings.TrimSpace(os.Getenv("NECA_WEB_HELPER_SOCKET"))
	}
	if *socket == "" {
		*socket = helper.DefaultSocketPath
	}
	if err := os.Setenv("NECA_WEB_HELPER_SOCKET", *socket); err != nil {
		log.Fatalf("设置 NECA_WEB_HELPER_SOCKET: %v", err)
	}
	if raw := strings.TrimSpace(os.Getenv("NECA_WEB_HELPER_GID")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			log.Fatalf("NECA_WEB_HELPER_GID 非法: %q", raw)
		}
		*gid = value
	}

	cfg, err := core.LoadConfig()
	if err != nil {
		log.Fatalf("加载配置: %v", err)
	}
	server := helper.NewServer(cfg, cfg.ConfigFile)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.ListenAndServe(ctx, *socket, *gid); err != nil {
		log.Fatalf("helper: %v", err)
	}
}
