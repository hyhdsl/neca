package core

import (
	"testing"
	"time"
)

func TestBuildExpirySummaryAndFilter(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	cert := func(cn, status string, days int) WebCert {
		return WebCert{
			CN: cn, Status: status,
			ExpiresAt: now.Add(time.Duration(days)*24*time.Hour + time.Hour),
		}
	}
	certs := []WebCert{
		cert("expired", "valid", -2),
		cert("revoked", "revoked", 1),
		cert("unindexed", "unindexed", 1),
		cert("d10", "valid", 10),
		cert("d45", "valid", 45),
		cert("d80", "valid", 80),
		cert("d100", "valid", 100),
		{CN: "no-expiry", Status: "valid"},
	}
	got := BuildExpirySummary(certs, now)
	want := ExpirySummary{Expired: 1, Within30: 1, Within60: 2, Within90: 3}
	if got != want {
		t.Fatalf("summary = %+v, want %+v", got, want)
	}
	within := ExpiringWithin(certs, now, 60)
	if len(within) != 2 || within[0].CN != "d10" || within[1].CN != "d45" {
		t.Fatalf("expiring within 60 = %+v", within)
	}
	if within[0].DaysLeft != 10 || within[1].DaysLeft != 45 {
		t.Fatalf("days left = %d/%d", within[0].DaysLeft, within[1].DaysLeft)
	}
}
