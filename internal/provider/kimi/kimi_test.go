package kimi

import (
	"testing"
	"time"
)

func TestParseKimiUsages(t *testing.T) {
	// 数值兼容字符串与数字两种形态；resetTime 为毫秒。
	body := []byte(`{
		"limits": [
			{"detail": {"limit": "100", "remaining": "80", "resetTime": 1780000000000}}
		],
		"usage": {"limit": 500, "remaining": 250, "resetTime": "1780604800000"}
	}`)
	info, err := parseKimiUsages(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(info.Dimensions) != 2 {
		t.Fatalf("want 2 dims, got %+v", info.Dimensions)
	}
	d := info.Dimensions[0]
	if d.Code != "5h" || d.UsedPercentage == nil || *d.UsedPercentage != 20 {
		t.Errorf("5h dim = %+v, want used 20", d)
	}
	if d.ResetAt == nil || !d.ResetAt.Equal(time.UnixMilli(1780000000000)) {
		t.Errorf("5h reset = %v", d.ResetAt)
	}
	w := info.Dimensions[1]
	if w.Code != "weekly" || w.UsedPercentage == nil || *w.UsedPercentage != 50 {
		t.Errorf("weekly dim = %+v, want used 50", w)
	}
}

func TestParseKimiUsagesOnlyLimits(t *testing.T) {
	body := []byte(`{"limits": [{"detail": {"limit": 200, "remaining": 200}}]}`)
	info, err := parseKimiUsages(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(info.Dimensions) != 1 || *info.Dimensions[0].UsedPercentage != 0 {
		t.Fatalf("want single 5h dim with used 0, got %+v", info.Dimensions)
	}
}

func TestParseKimiUsagesEmpty(t *testing.T) {
	if _, err := parseKimiUsages([]byte(`{"limits": []}`)); err == nil {
		t.Fatal("empty payload should error")
	}
}
