package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"ai-subscription-keeper/internal/app"
	"ai-subscription-keeper/internal/provider"
	"ai-subscription-keeper/internal/provider/registry"
	"ai-subscription-keeper/internal/store"
)

// dashboardService is one card on the status board.
type dashboardService struct {
	ID               string                `json:"id"`
	Name             string                `json:"name"`
	ProviderType     string                `json:"provider_type"`
	ProviderName     string                `json:"provider_name"`
	ProviderLogo     string                `json:"provider_logo,omitempty"`
	Enabled          bool                  `json:"enabled"`
	Status           string                `json:"status"` // ok | error | unknown
	Quota            *provider.QuotaInfo   `json:"quota,omitempty"`
	Raw              *provider.RawResponse `json:"raw,omitempty"`
	UnsupportedQuota bool                  `json:"unsupported_quota"`
	LastQuotaAt      *time.Time            `json:"last_quota_at,omitempty"`
	LastQuotaError   string                `json:"last_quota_error,omitempty"`
	DefaultTestModel string                `json:"default_test_model"`
	SortOrder        int                   `json:"sort_order"`
}

func handleProviderTypes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"types": registry.Types()})
}

// buildDashboardCard assembles a card DTO from a store row, applying the
// per-service quota label overrides on top of provider defaults.
func buildDashboardCard(ms *store.ModelService) *dashboardService {
	pt, _ := registry.TypeByCode(ms.ProviderType)
	card := &dashboardService{
		ID:               ms.ID,
		Name:             ms.Name,
		ProviderType:     ms.ProviderType,
		ProviderName:     pt.Name,
		ProviderLogo:     pt.Logo,
		Enabled:          ms.Enabled,
		DefaultTestModel: ms.DefaultTestModel,
		SortOrder:        ms.SortOrder,
		UnsupportedQuota: !pt.SupportsQuota,
		LastQuotaAt:      ms.LastQuotaAt,
		LastQuotaError:   ms.LastQuotaError,
	}
	if ms.LastQuota != "" {
		var payload app.QuotaPayload
		if err := json.Unmarshal([]byte(ms.LastQuota), &payload); err == nil {
			card.Quota = payload.Quota
			card.Raw = payload.Raw
			if payload.Unsupported {
				card.UnsupportedQuota = true
			}
		}
	}
	if card.Quota != nil && ms.QuotaLabels != "" {
		var labels map[string]string
		if err := json.Unmarshal([]byte(ms.QuotaLabels), &labels); err == nil {
			for i := range card.Quota.Dimensions {
				if label, ok := labels[card.Quota.Dimensions[i].Code]; ok && label != "" {
					card.Quota.Dimensions[i].DisplayName = label
				}
			}
		}
	}
	switch {
	case ms.LastQuotaError != "":
		card.Status = "error"
	case ms.LastQuotaAt != nil:
		card.Status = "ok"
	default:
		card.Status = "unknown"
	}
	return card
}

func dashboardResponse(a *app.App) (map[string]any, error) {
	services, err := a.Store.ListModelServices()
	if err != nil {
		return nil, err
	}
	cards := make([]*dashboardService, 0, len(services))
	var lastUpdate *time.Time
	for _, ms := range services {
		cards = append(cards, buildDashboardCard(ms))
		if ms.LastQuotaAt != nil && (lastUpdate == nil || ms.LastQuotaAt.After(*lastUpdate)) {
			t := *ms.LastQuotaAt
			lastUpdate = &t
		}
	}
	return map[string]any{"services": cards, "updated_at": lastUpdate}, nil
}

func handleDashboard(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := dashboardResponse(a)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// refreshConcurrency caps parallel vendor queries during 全部刷新.
const refreshConcurrency = 4

// refreshAll concurrently refreshes every enabled service.
func refreshAll(ctx context.Context, a *app.App) {
	services, err := a.Store.ListModelServices()
	if err != nil {
		return
	}
	sem := make(chan struct{}, refreshConcurrency)
	var wg sync.WaitGroup
	for _, ms := range services {
		if !ms.Enabled {
			continue
		}
		wg.Add(1)
		go func(ms *store.ModelService) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			a.RefreshQuota(ctx, ms)
		}(ms)
	}
	wg.Wait()
}

func handleDashboardRefresh(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
		defer cancel()
		refreshAll(ctx, a)
		resp, err := dashboardResponse(a)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}
