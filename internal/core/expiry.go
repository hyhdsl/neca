// 本文件计算证书到期窗口与汇总。
package core

import (
	"sort"
	"time"
)

// ExpirySummary 是只读的到期概览。计数只覆盖仍需要续期的证书：
// 状态为 valid/expired，且不包含 revoked/unindexed。窗口为累计值：
// Within30 包含 30 天内到期的证书，Within60 也包含 30 天内的证书。
type ExpirySummary struct {
	Expired  int `json:"expired"`
	Within30 int `json:"within30"`
	Within60 int `json:"within60"`
	Within90 int `json:"within90"`
}

// ExpiringCert 是一张即将到期的证书，供 CLI 与 Web 只读展示。
type ExpiringCert struct {
	CN        string    `json:"cn"`
	Status    string    `json:"status"`
	Expires   string    `json:"expires"`
	ExpiresAt time.Time `json:"expiresAt"`
	DaysLeft  int       `json:"daysLeft"`
}

// DaysUntil 返回按自然日向下取整的剩余天数；已过期返回负数。
func DaysUntil(expiresAt, now time.Time) int {
	return int(expiresAt.Sub(now).Hours() / 24)
}

// BuildExpirySummary 统计过期和 30/60/90 天累计窗口。
// 已吊销与未索引证书不进入续期提醒；无到期时间的记录跳过。
func BuildExpirySummary(certs []WebCert, now time.Time) ExpirySummary {
	summary := ExpirySummary{}
	now = now.UTC()
	for _, cert := range certs {
		if cert.Status == "revoked" || cert.Status == "unindexed" || cert.ExpiresAt.IsZero() {
			continue
		}
		if !cert.ExpiresAt.After(now) {
			summary.Expired++
			continue
		}
		days := DaysUntil(cert.ExpiresAt, now)
		if days <= 30 {
			summary.Within30++
		}
		if days <= 60 {
			summary.Within60++
		}
		if days <= 90 {
			summary.Within90++
		}
	}
	return summary
}

// ExpiringWithin 返回未来 N 天内到期的有效证书，按到期时间升序。
// 过期证书不在此列表中；概览中的 Expired 单独展示。
func ExpiringWithin(certs []WebCert, now time.Time, days int) []ExpiringCert {
	if days <= 0 {
		return []ExpiringCert{}
	}
	now = now.UTC()
	out := make([]ExpiringCert, 0)
	for _, cert := range certs {
		if cert.Status != "valid" || cert.ExpiresAt.IsZero() || !cert.ExpiresAt.After(now) {
			continue
		}
		left := DaysUntil(cert.ExpiresAt, now)
		if left > days {
			continue
		}
		out = append(out, ExpiringCert{
			CN: cert.CN, Status: cert.Status, Expires: cert.Expires,
			ExpiresAt: cert.ExpiresAt.UTC(), DaysLeft: left,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ExpiresAt.Equal(out[j].ExpiresAt) {
			return out[i].CN < out[j].CN
		}
		return out[i].ExpiresAt.Before(out[j].ExpiresAt)
	})
	return out
}
