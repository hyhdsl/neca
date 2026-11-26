// 本文件测试 protocol 相关逻辑。
package helper

import "testing"

func TestRequestValidate(t *testing.T) {
	valid := Request{RequestID: "req-1", Op: OpIssue, CN: "TMP01", Days: 365, Platforms: []string{"windows, android"}, P12Profile: "compat"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid request: %v", err)
	}
	tests := []Request{
		{RequestID: "", Op: OpIssue, CN: "TMP01"},
		{RequestID: "bad id", Op: OpIssue, CN: "TMP01"},
		{RequestID: "req", Op: "delete", CN: "TMP01"},
		{RequestID: "req", Op: OpIssue, CN: "../etc"},
		{RequestID: "req", Op: OpIssue, CN: "TMP01", Days: 4000},
		{RequestID: "req", Op: OpIssue, CN: "TMP01", Platforms: []string{"solaris"}},
		{RequestID: "req", Op: OpIssue, CN: "TMP01", Installers: []string{"android"}},
		{RequestID: "req", Op: OpIssue, CN: "TMP01", P12Profile: "weird"},
	}
	for i, req := range tests {
		if err := req.Validate(); err == nil {
			t.Fatalf("case %d should fail: %+v", i, req)
		}
	}
}
