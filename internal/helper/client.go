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

var ErrUnavailable = errors.New("web helper unavailable")

type Client struct {
	socket string
	http   *http.Client
}

func NewClient(socket string) *Client {
	if strings.TrimSpace(socket) == "" {
		socket = DefaultSocketPath
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socket)
		},
		DisableKeepAlives: true,
	}
	return &Client{socket: socket, http: &http.Client{Transport: transport}}
}

func (c *Client) Socket() string { return c.socket }

func (c *Client) Status(ctx context.Context) (*Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var status Status
	if err := c.do(ctx, http.MethodGet, "/healthz", nil, &status); err != nil {
		return nil, err
	}
	if !status.OK || status.Protocol != ProtocolVersion {
		return nil, fmt.Errorf("%w: 协议不匹配（%q）", ErrUnavailable, status.Protocol)
	}
	return &status, nil
}

func (c *Client) Execute(ctx context.Context, req Request) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	var response Response
	if err := c.do(ctx, http.MethodPost, "/v1/execute", req, &response); err != nil {
		return nil, err
	}
	if response.Protocol != ProtocolVersion {
		return nil, fmt.Errorf("helper 协议不匹配: %q", response.Protocol)
	}
	return &response, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://unix"+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%w: 请求超时", ErrUnavailable)
		}
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var failure Response
		if json.Unmarshal(data, &failure) == nil && failure.Error != nil {
			return fmt.Errorf("%s: %s", failure.Error.Code, failure.Error.Message)
		}
		return fmt.Errorf("helper HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("解析 helper 响应: %w", err)
	}
	return nil
}
