// 本文件测试 table 相关逻辑。
package core

import (
	"strings"
	"testing"
)

// TestDisplayWidth 锁定 East Asian Width 规则：W/F 记 2，其余记 1。
func TestDisplayWidth(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"abc", 3},
		{"CN", 2},
		{"中文", 4},
		{"状态", 4},
		{"a一", 3}, // 1 + 2
		{"Ａ", 2},  // 全角 A（East Asian Fullwidth）
		{"±", 1},  // Ambiguous 记 1，与旧 Python 一致
		{"アイ", 4}, // 片假名 Wide
	}
	for _, tc := range cases {
		if got := DisplayWidth(tc.input); got != tc.want {
			t.Fatalf("DisplayWidth(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

// TestFormatTableASCII 用纯 ASCII 表锁定表头、分隔线与列间距。
func TestFormatTableASCII(t *testing.T) {
	rows := [][]string{
		{"A", "BB"},
		{"CC", "D"},
	}
	want := "A   BB\n--  --\nCC  D\n"
	if got := FormatTable(rows); got != want {
		t.Fatalf("FormatTable() = %q, want %q", got, want)
	}
}

// TestFormatTableChinese 校验中文列宽对齐（每行去掉行尾空格）。
func TestFormatTableChinese(t *testing.T) {
	rows := [][]string{
		{"CN", "状态"},
		{"TMP01", "有效"},
		{"X", "已吊销"},
	}
	got := FormatTable(rows)
	// 列宽：CN 列 5（TMP01），状态列 6（已吊销）。
	want := "CN" + strings.Repeat(" ", 5) + "状态\n" +
		"-----  ------\n" +
		"TMP01  有效\n" +
		"X" + strings.Repeat(" ", 6) + "已吊销\n"
	if got != want {
		t.Fatalf("FormatTable() = %q, want %q", got, want)
	}
}
