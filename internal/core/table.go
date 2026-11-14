package core

import (
	"strings"

	"golang.org/x/text/width"
)

// DisplayWidth 返回字符串在终端中的显示宽度。
//
// 规则与旧脚本的 Python 实现一致：East Asian Wide/Fullwidth 记 2，
// 其余（含 Narrow/Halfwidth/Ambiguous）记 1（开发文档 §8.8）。
func DisplayWidth(s string) int {
	w := 0
	for _, r := range s {
		switch width.LookupRune(r).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			w += 2
		default:
			w++
		}
	}
	return w
}

// FormatTable 把二维表格渲染为对齐文本。
//
// 第一行视为表头，其后紧跟一行与列宽等长的 '-'。列间用两个空格分隔，
// 数据行去掉行尾空格，与旧脚本的 Python format_table 逐字符一致。
func FormatTable(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}

	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}

	widths := make([]int, cols)
	for _, row := range rows {
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			if w := DisplayWidth(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}

	var builder strings.Builder
	for index, row := range rows {
		cells := make([]string, cols)
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			cells[i] = cell + strings.Repeat(" ", widths[i]-DisplayWidth(cell))
		}
		builder.WriteString(strings.TrimRight(strings.Join(cells, "  "), " "))
		builder.WriteByte('\n')
		if index == 0 {
			dashes := make([]string, cols)
			for i := 0; i < cols; i++ {
				dashes[i] = strings.Repeat("-", widths[i])
			}
			builder.WriteString(strings.Join(dashes, "  "))
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}
