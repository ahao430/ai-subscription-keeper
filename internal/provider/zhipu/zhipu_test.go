package zhipu

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseZhipuQuota(t *testing.T) {
	body := []byte(`{
		"code": 200, "success": true,
		"data": {
			"level": "MAX",
			"limits": [
				{"type": "TOKENS_LIMIT", "unit": 3, "number": 5, "percentage": 18.5, "nextResetTime": 1780000000000},
				{"type": "CREDIT_LIMIT", "unit": 6, "percentage": 35, "nextResetTime": 1780086400000},
				{"type": "TIME_LIMIT", "percentage": 60}
			]
		}
	}`)
	info, err := parseZhipuQuota(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(info.Dimensions) != 3 {
		t.Fatalf("want 3 dimensions, got %d: %+v", len(info.Dimensions), info.Dimensions)
	}
	d := info.Dimensions[0]
	if d.Code != "5h" || d.DisplayName != "5小时限额" {
		t.Errorf("dim0 = %s/%s, want 5h/5小时限额", d.Code, d.DisplayName)
	}
	if d.UsedPercentage == nil || *d.UsedPercentage != 18.5 {
		t.Errorf("dim0 used = %v, want 18.5", d.UsedPercentage)
	}
	if d.ResetAt == nil || !d.ResetAt.Equal(time.UnixMilli(1780000000000)) {
		t.Errorf("dim0 reset = %v, want epoch ms 1780000000000", d.ResetAt)
	}
	w := info.Dimensions[1]
	if w.Code != "weekly" || w.DisplayName != "周限额" {
		t.Errorf("dim1 = %s/%s, want weekly/周限额", w.Code, w.DisplayName)
	}
	if w.UsedPercentage == nil || *w.UsedPercentage != 35 {
		t.Errorf("dim1 used = %v, want 35", w.UsedPercentage)
	}
	m := info.Dimensions[2]
	if m.Code != "mcp_monthly" {
		t.Errorf("dim2 = %s, want mcp_monthly", m.Code)
	}
	if len(info.Extra) != 1 || info.Extra[0].Value != "MAX" {
		t.Errorf("level extra = %+v, want MAX", info.Extra)
	}
}

func TestParseZhipuQuotaLegacySingleLimit(t *testing.T) {
	// 老套餐只返回一条无 unit 的限制 → 降级为 5 小时窗。
	body := []byte(`{"success": true, "code": 200, "data": {"limits": [
		{"type": "TOKENS_LIMIT", "percentage": 40, "nextResetTime": 1780000000000}
	]}}`)
	info, err := parseZhipuQuota(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(info.Dimensions) != 1 || info.Dimensions[0].Code != "5h" {
		t.Fatalf("want single 5h dim, got %+v", info.Dimensions)
	}
}

func TestParseZhipuQuotaBusinessError(t *testing.T) {
	body := []byte(`{"code": 401, "success": false, "msg": "令牌已过期或验证不正确"}`)
	if _, err := parseZhipuQuota(body); err == nil || err.Error() != "令牌已过期或验证不正确" {
		t.Fatalf("want business error, got %v", err)
	}
}

func TestEpochToTime(t *testing.T) {
	n := json.Number("1780000000000")
	if _, ok := epochToTime(n); !ok {
		t.Error("ms epoch should parse")
	}
	s := json.Number("1780000000")
	tm, ok := epochToTime(s)
	if !ok || !tm.Equal(time.Unix(1780000000, 0)) {
		t.Error("second epoch should parse")
	}
	if _, ok := epochToTime(json.Number("0")); ok {
		t.Error("zero epoch should not parse")
	}
}
