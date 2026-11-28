// 本文件实现 helper 到 necap 的受控 Unix socket 代理。
package helper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

var necapAllowedPaths = map[string]bool{
	"/v1/health":        true,
	"/v1/auth/password": true, "/v1/auth/totp": true, "/v1/auth/recovery": true,
	"/v1/auth/passkey/begin": true, "/v1/auth/passkey/finish": true,
	"/v1/factors/status":     true,
	"/v1/factors/totp/begin": true, "/v1/factors/totp/finish": true,
	"/v1/factors/passkey/begin": true, "/v1/factors/passkey/finish": true,
	"/v1/factors/totp/delete": true, "/v1/factors/passkey/delete": true,
	"/v1/factors/recovery/regenerate": true, "/v1/factors/policy": true,
}

type NeCapProxyRequest struct {
	Path    string          `json:"path"`
	Payload json.RawMessage `json:"payload"`
}

type NeCapProxyResponse struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

type NeCapClient struct {
	socket string
	http   *http.Client
}

func NewNeCapClient(socket string) *NeCapClient {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socket)
		},
		DisableKeepAlives: true,
	}
	return &NeCapClient{socket: socket, http: &http.Client{Transport: transport}}
}

func (c *NeCapClient) Forward(ctx context.Context, path string, payload []byte) (int, []byte, error) {
	if c == nil {
		return 0, nil, errors.New("necap 未配置")
	}
	if !necapAllowedPaths[path] {
		return 0, nil, fmt.Errorf("necap 路径不允许: %s", path)
	}
	if len(payload) > 1<<20 {
		return 0, nil, errors.New("necap 请求过大")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://necap"+path, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("连接 necap: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, body, nil
}

func (c *Client) NeCap(ctx context.Context, path string, payload any) (int, json.RawMessage, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	var response NeCapProxyResponse
	if err := c.do(ctx, http.MethodPost, "/v1/necap", NeCapProxyRequest{Path: path, Payload: data}, &response); err != nil {
		return 0, nil, err
	}
	if response.Status < 200 || response.Status >= 300 {
		var failure struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(response.Body, &failure)
		if strings.TrimSpace(failure.Message) == "" {
			failure.Message = "necap 请求失败"
		}
		return response.Status, response.Body, fmt.Errorf("%s", failure.Message)
	}
	return response.Status, response.Body, nil
}
