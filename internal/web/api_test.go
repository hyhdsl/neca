// 本文件测试 api 相关逻辑。
package web

import (
	"testing"
	"time"

	"github.com/hyhdsl/neca/internal/core"
)

func TestFilterCertsByExpiryWindow(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	cert := func(cn string, days int) core.WebCert {
		return core.WebCert{CN: cn, Status: "valid", ExpiresAt: now.Add(time.Duration(days) * 24 * time.Hour)}
	}
	certs := []core.WebCert{cert("d10", 10), cert("d45", 45), cert("d80", 80), cert("d100", 100)}

	if got := filterCertsByExpiry(certs, "30", now); len(got) != 1 || got[0].CN != "d10" {
		t.Fatalf("30-day filter = %+v", got)
	}
	if got := filterCertsByExpiry(certs, "60", now); len(got) != 2 || got[0].CN != "d10" || got[1].CN != "d45" {
		t.Fatalf("60-day filter = %+v", got)
	}
	if got := filterCertsByExpiry(certs, "90", now); len(got) != 3 || got[0].CN != "d10" || got[1].CN != "d45" || got[2].CN != "d80" {
		t.Fatalf("90-day filter = %+v", got)
	}
	if got := filterCertsByExpiry(certs, "invalid", now); len(got) != len(certs) {
		t.Fatalf("invalid window changed result: %+v", got)
	}
}
