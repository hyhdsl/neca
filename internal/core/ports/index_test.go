package ports

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// validRow 返回一条合法的索引行，减少用例里的重复字段。
func validRow() Row {
	return Row{
		Status:    IndexValid,
		Expires:   "271115000000Z",
		RevokedAt: "",
		Serial:    "7063A61F",
		File:      "TMP01.pem",
		Subject:   "/C=CN/O=NekoBigLazyer/CN=TMP01",
	}
}

// TestParseRowValid 校验 6 字段的正常解析。
func TestParseRowValid(t *testing.T) {
	row, err := ParseRow("V\t271115000000Z\t\t7063A61F\tTMP01.pem\t/C=CN/O=NekoBigLazyer/CN=TMP01")
	if err != nil {
		t.Fatalf("ParseRow() error = %v", err)
	}
	if row.Status != IndexValid || row.Serial != "7063A61F" || row.CN() != "TMP01" {
		t.Fatalf("ParseRow() = %+v, want status V / serial 7063A61F / CN TMP01", row)
	}
}

// TestParseRowInvalid 校验字段数不足与未知状态码都会报错，
// 避免把损坏的索引行当成有效证书继续处理。
func TestParseRowInvalid(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"字段不足", "V\t271115000000Z\t7063A61F"},
		{"字段过多", "V\t1\t2\t3\t4\t5\t6"},
		{"未知状态", "X\t271115000000Z\t\t7063A61F\tTMP01.pem\t/CN=TMP01"},
		{"空状态", "\t271115000000Z\t\t7063A61F\tTMP01.pem\t/CN=TMP01"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseRow(tc.line); err == nil {
				t.Fatalf("ParseRow(%q) 应当报错", tc.line)
			}
		})
	}
}

// TestRowRoundTrip 校验 String/ParseRow 往返一致，且 CRLF 行尾可解析。
func TestRowRoundTrip(t *testing.T) {
	want := validRow()
	got, err := ParseRow(want.String() + "\r\n")
	if err != nil {
		t.Fatalf("ParseRow(String()) error = %v", err)
	}
	if got != want {
		t.Fatalf("往返不一致：got %+v, want %+v", got, want)
	}
}

// TestRowCN 校验从三种 subject 形态提取 CN 的行为。
func TestRowCN(t *testing.T) {
	tests := []struct {
		subject string
		want    string
	}{
		{"/C=CN/O=NekoBigLazyer/CN=TMP01", "TMP01"},
		{"/CN=ONLY", "ONLY"},
		{"/C=CN/O=NekoBigLazyer", ""},
	}
	for _, tc := range tests {
		row := Row{Subject: tc.subject}
		if got := row.CN(); got != tc.want {
			t.Fatalf("CN(%q) = %q, want %q", tc.subject, got, tc.want)
		}
	}
}

// TestIndexFind 校验序列号查找不区分大小写，找不到时返回 -1。
func TestIndexFind(t *testing.T) {
	idx := &Index{Rows: []Row{validRow()}}
	if got := idx.Find("7063a61f"); got != 0 {
		t.Fatalf("Find(小写) = %d, want 0", got)
	}
	if got := idx.Find("DEADBEEF"); got != -1 {
		t.Fatalf("Find(不存在) = %d, want -1", got)
	}
}

// memStore 是 IndexStore 的内存实现，用于验证接口契约与回滚语义。
// 它同时证明接口可被核心之外（测试）的实现满足，M1 的文件后端按同样语义实现。
type memStore struct {
	rows   []Row
	locked bool
}

// 编译期断言：memStore 必须满足 IndexStore。
var _ IndexStore = (*memStore)(nil)

// Load 返回副本，保证调用方修改返回值不会影响存储内部状态。
func (m *memStore) Load(ctx context.Context) ([]Row, error) {
	return append([]Row(nil), m.rows...), nil
}

// Update 在副本上执行 fn，成功才提交；失败时保持原状态，模拟文件后端的
// "临时文件丢弃、索引保持原样"（开发文档 §11）。
func (m *memStore) Update(ctx context.Context, fn func(idx *Index) error) error {
	if m.locked {
		return fmt.Errorf("memStore: %w", ErrIndexLocked)
	}
	idx := &Index{Rows: append([]Row(nil), m.rows...)}
	if err := fn(idx); err != nil {
		return err
	}
	m.rows = idx.Rows
	return nil
}

// TestIndexStoreContract 用假实现验证 IndexStore 的核心语义：
// Update 成功提交、失败回滚、Load 返回副本。
func TestIndexStoreContract(t *testing.T) {
	ctx := context.Background()
	store := &memStore{}

	// 1. 成功提交一行。
	if err := store.Update(ctx, func(idx *Index) error {
		idx.Rows = append(idx.Rows, validRow())
		return nil
	}); err != nil {
		t.Fatalf("Update(commit) error = %v", err)
	}

	rows, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(rows) != 1 || rows[0].Serial != "7063A61F" {
		t.Fatalf("Load() = %+v, want one row 7063A61F", rows)
	}

	// 2. 失败时不得落盘（回滚语义）。
	boom := errors.New("boom")
	if err := store.Update(ctx, func(idx *Index) error {
		idx.Rows = nil
		return boom
	}); !errors.Is(err, boom) {
		t.Fatalf("Update(rollback) error = %v, want boom", err)
	}
	rows, _ = store.Load(ctx)
	if len(rows) != 1 {
		t.Fatalf("失败后 Load() 行数 = %d, want 1（不得落盘）", len(rows))
	}

	// 3. 调用方修改 Load 返回值不影响内部状态。
	rows[0].Serial = "MUTATED"
	rows2, _ := store.Load(ctx)
	if rows2[0].Serial != "7063A61F" {
		t.Fatalf("Load() 未返回副本：%+v", rows2)
	}
}

// TestIndexStoreReportsLock 校验锁冲突使用哨兵错误，CLI 才能映射到退出码 3。
func TestIndexStoreReportsLock(t *testing.T) {
	store := &memStore{locked: true}
	err := store.Update(context.Background(), func(idx *Index) error { return nil })
	if !errors.Is(err, ErrIndexLocked) {
		t.Fatalf("Update(locked) error = %v, want ErrIndexLocked", err)
	}
	if !strings.Contains(err.Error(), "locked") {
		t.Fatalf("错误信息应说明锁冲突：%v", err)
	}
}
