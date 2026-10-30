package service_test

import (
	"context"
	"testing"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/providers"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/service"
)

type mockScraper struct {
	domain string
	name   string
}

func (m *mockScraper) Domain() string { return m.domain }
func (m *mockScraper) Name() string   { return m.name }
func (m *mockScraper) SearchOrBrowse(ctx context.Context, q string, page int) (*domain.ScrapeResult, error) {
	ch := 50
	return &domain.ScrapeResult{
		Items: []domain.ScrapedItem{
			{
				Title:         "Test Manhua",
				Slug:          "test-manhua",
				URLTemplate:   "https://test.com/chapter/{chapter}",
				CoverURL:      "https://test.com/cover.jpg",
				LastChapter:   &ch,
				PrimaryDomain: "test.com",
			},
		},
		Page:       page,
		TotalPages: 5,
	}, nil
}

func TestImportService_Browse(t *testing.T) {
	reg := providers.NewProviderRegistry()
	reg.Register(&mockScraper{domain: "test.com", name: "Test"})

	svc := service.NewImportService(reg, nil, nil, nil)
	res, err := svc.Browse(context.Background(), "test.com", "", 1, nil)
	if err != nil {
		t.Fatalf("unexpected browse error: %v", err)
	}

	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}
	if res.Items[0].Title != "Test Manhua" {
		t.Errorf("expected 'Test Manhua', got '%s'", res.Items[0].Title)
	}
	if res.Items[0].AlreadyInCatalog {
		t.Errorf("expected AlreadyInCatalog to be false without repo")
	}
}

func TestImportServiceCollectionDoesNotChangeOtherDomains(t *testing.T) {
	reg := providers.NewProviderRegistry()
	reg.Register(&mockScraper{domain: "test.com", name: "Test"})
	svc := service.NewImportService(reg, nil, nil, nil)
	svc.SetCollection(service.NewCatalogCollection(nil, reg, nil, nil))
	res, err := svc.Browse(context.Background(), "test.com", "", 1, nil)
	if err != nil || len(res.Items) != 1 || res.Syncing {
		t.Fatalf("existing provider flow changed: result=%+v error=%v", res, err)
	}
}
