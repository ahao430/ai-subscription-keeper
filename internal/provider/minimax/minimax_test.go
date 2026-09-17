package minimax

import (
	"testing"
	"time"
)

func TestParseMiniMaxRemains(t *testing.T) {
	body := []byte(`{
		"model_remains": [
			{"model_name": "video", "current_interval_remaining_percent": 99},
			{
				"model_name": "general",
				"current_interval_remaining_percent": 82.5,
				"end_time": 1780000000000,
				"current_weekly_status": 1,
				"current_weekly_remaining_percent": 65,
				"weekly_end_time": 1780604800000
			}
		]
	}`)
	info, err := parseMiniMaxRemains(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(info.Dimensions) != 2 {
		t.Fatalf("want 2 dims, got %+v", info.Dimensions)
	}
	d := info.Dimensions[0]
	if d.Code != "5h" || d.UsedPercentage == nil || *d.UsedPercentage != 17.5 {
		t.Errorf("5h dim = %+v, want used 17.5", d)
	}
	if d.ResetAt == nil || !d.ResetAt.Equal(time.UnixMilli(1780000000000)) {
		t.Errorf("5h reset = %v", d.ResetAt)
	}
	w := info.Dimensions[1]
	if w.Code != "weekly" || w.UsedPercentage == nil || *w.UsedPercentage != 35 {
		t.Errorf("weekly dim = %+v, want used 35", w)
	}
}

func TestParseMiniMaxRemainsNoWeekly(t *testing.T) {
	// current_weekly_status=3 → 无周限额，仅 5h 桶。
	body := []byte(`{"model_remains": [
		{"model_name": "general", "current_interval_remaining_percent": 10, "end_time": 1780000000000, "current_weekly_status": 3}
	]}`)
	info, err := parseMiniMaxRemains(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(info.Dimensions) != 1 || info.Dimensions[0].Code != "5h" {
		t.Fatalf("want single 5h dim, got %+v", info.Dimensions)
	}
	if *info.Dimensions[0].UsedPercentage != 90 {
		t.Errorf("used = %v, want 90", *info.Dimensions[0].UsedPercentage)
	}
}

func TestParseMiniMaxRemainsWrapped(t *testing.T) {
	body := []byte(`{"data": {"model_remains": [
		{"model_name": "general", "current_interval_remaining_percent": 50}
	]}}`)
	info, err := parseMiniMaxRemains(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(info.Dimensions) != 1 || *info.Dimensions[0].UsedPercentage != 50 {
		t.Fatalf("wrapped payload should parse, got %+v", info.Dimensions)
	}
}

func TestParseMiniMaxRemainsBaseRespError(t *testing.T) {
	body := []byte(`{"base_resp": {"status_code": 1004, "status_msg": "cookie is missing, log in again"}}`)
	_, err := parseMiniMaxRemains(body)
	if err == nil || err.Error() != "cookie is missing, log in again" {
		t.Fatalf("want base_resp error, got %v", err)
	}
}
