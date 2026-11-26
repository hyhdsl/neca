// 本文件测试 csv 相关逻辑。
package core

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestQuoteCSVField 校验引号翻倍规则与旧脚本 csvq 一致。
func TestQuoteCSVField(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"CN", "\"CN\""},
		{"A\"B", "\"A\"\"B\""},
		{"", "\"\""},
		{"中文", "\"中文\""},
	}
	for _, tc := range cases {
		if got := QuoteCSVField(tc.input); got != tc.want {
			t.Fatalf("QuoteCSVField(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestWriteCSV 校验 BOM、表头顺序与字段转义。
func TestWriteCSV(t *testing.T) {
	var buf bytes.Buffer
	header := []string{"CN", "备注"}
	rows := [][]string{{"A\"B", "有效"}}
	if err := WriteCSV(&buf, header, rows); err != nil {
		t.Fatalf("WriteCSV() error = %v", err)
	}
	want := "\xEF\xBB\xBF" + "\"CN\",\"备注\"\n" + "\"A\"\"B\",\"有效\"\n"
	if got := buf.String(); got != want {
		t.Fatalf("WriteCSV() = %q, want %q", got, want)
	}
}

// TestWriteCSVFile 校验文件落盘与目录自动创建。
func TestWriteCSVFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "out.csv")
	if err := WriteCSVFile(path, []string{"CN"}, [][]string{{"TMP01"}}); err != nil {
		t.Fatalf("WriteCSVFile() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取导出文件失败：%v", err)
	}
	if !bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("导出文件缺少 UTF-8 BOM")
	}
	if !bytes.Contains(data, []byte("\"TMP01\"")) {
		t.Fatalf("导出内容缺少数据行：%q", data)
	}
}
