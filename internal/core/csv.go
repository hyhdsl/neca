package core

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// QuoteCSVField 给字段加双引号并把内部双引号翻倍，复刻旧脚本的 csvq()。
//
// 旧导出对每个字段都加引号（即使不需要），这样输出与旧脚本逐字节一致。
func QuoteCSVField(field string) string {
	return `"` + strings.ReplaceAll(field, `"`, `""`) + `"`
}

// WriteCSV 写出 UTF-8 BOM、表头与数据行，全部字段强制加引号。
func WriteCSV(w io.Writer, header []string, rows [][]string) error {
	writer := bufio.NewWriter(w)
	// 旧脚本先写 BOM 再写表头；Excel 依赖 BOM 识别 UTF-8。
	if _, err := writer.WriteString("\xEF\xBB\xBF"); err != nil {
		return err
	}
	if err := writeCSVLine(writer, header); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeCSVLine(writer, row); err != nil {
			return err
		}
	}
	return writer.Flush()
}

// writeCSVLine 写一行强制加引号的 CSV。
func writeCSVLine(w *bufio.Writer, fields []string) error {
	quoted := make([]string, len(fields))
	for i, field := range fields {
		quoted[i] = QuoteCSVField(field)
	}
	if _, err := w.WriteString(strings.Join(quoted, ",")); err != nil {
		return err
	}
	return w.WriteByte('\n')
}

// WriteCSVFile 创建目录并原子写出 CSV 文件。
//
// 原子写在 M1 不是硬要求（旧脚本直接覆盖），但仍使用临时文件 + rename，
// 避免导出中断留下半截文件。
func WriteCSVFile(path string, header []string, rows [][]string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return runtimef("导出 CSV", "创建目录 %s: %v", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".export-*.tmp")
	if err != nil {
		return runtimef("导出 CSV", "创建临时文件: %v", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := WriteCSV(tmp, header, rows); err != nil {
		tmp.Close()
		return runtimef("导出 CSV", "%v", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return runtimef("导出 CSV", "fsync: %v", err)
	}
	if err := tmp.Close(); err != nil {
		return runtimef("导出 CSV", "close: %v", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return runtimef("导出 CSV", "chmod: %v", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return runtimef("导出 CSV", "rename: %v", err)
	}
	return nil
}

// csvStatus 返回导出时使用的中文状态。
func csvStatus(kind ExportKind) string {
	if kind == ExportRevoked {
		return "已吊销"
	}
	return "有效"
}
