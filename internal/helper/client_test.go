package helper

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"testing"
)

func startUnixHelper(t *testing.T, handler http.Handler) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "helper.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler}
	go server.Serve(listener)
	t.Cleanup(func() { _ = server.Close() })
	return socket
}

func TestClientStatusAndExecute(t *testing.T) {
	socket := startUnixHelper(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/healthz":
			_ = json.NewEncoder(w).Encode(Status{OK: true, Protocol: ProtocolVersion, Version: "test", ReadOnly: false})
		case "/v1/execute":
			_ = json.NewEncoder(w).Encode(Response{OK: true, Protocol: ProtocolVersion, Result: &Result{CN: "TMP01"}})
		default:
			http.NotFound(w, r)
		}
	}))
	client := NewClient(socket)
	status, err := client.Status(context.Background())
	if err != nil || !status.OK || status.ReadOnly {
		t.Fatalf("Status = %+v, %v", status, err)
	}
	response, err := client.Execute(context.Background(), Request{RequestID: "req-1", Op: OpIssue, CN: "TMP01"})
	if err != nil || !response.OK || response.Result.CN != "TMP01" {
		t.Fatalf("Execute = %+v, %v", response, err)
	}
}
