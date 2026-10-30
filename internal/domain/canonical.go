package domain

import (
	"context"
	"time"
)

// CanonicalManhua represents a public, canonical manga/manhua entry.
type CanonicalManhua struct {
	ID                int64     `json:"id"`
	Slug              string    `json:"slug"`
	Title             string    `json:"title"`
	AlternativeTitles *string   `json:"alternativeTitles,omitempty"`
	Synopsis          *string   `json:"synopsis,omitempty"`
	CoverURL          *string   `json:"coverUrl,omitempty"`
	PrimaryDomain     string    `json:"primaryDomain"`
	URLTemplate       string    `json:"urlTemplate"`
	LastChapter       *int      `json:"lastChapter,omitempty"`
	UpdateDay         *int16    `json:"updateDay,omitempty"` // 0 = Monday ... 6 = Sunday
	IsHiatus          bool      `json:"isHiatus"`
	BayesianRating    float64   `json:"bayesianRating"`
	TotalVotes        int       `json:"totalVotes"`
	TotalViews        int64     `json:"totalViews"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// CatalogFilter defines query parameters for searching and filtering the canonical catalog.
type CatalogFilter struct {
	Query     string `json:"query"`
	Domain    string `json:"domain"`
	UpdateDay *int16 `json:"updateDay"`
	IsHiatus  *bool  `json:"isHiatus"`
	SortBy    string `json:"sortBy"` // "views", "rating", "title", "updated"
	Page      int    `json:"page"`
	PageSize  int    `json:"pageSize"`
}

// CatalogSearchResult represents a paginated catalog result.
type CatalogSearchResult struct {
	Items      []*CanonicalManhua `json:"items"`
	Total      int                `json:"total"`
	Page       int                `json:"page"`
	PageSize   int                `json:"pageSize"`
	TotalPages int                `json:"totalPages"`
}

// ScrapedItem represents an item extracted from a provider.
type ScrapedItem struct {
	CoverState    string `json:"coverState,omitempty"`
	Title         string `json:"title"`
	Slug          string `json:"slug"`
	URLTemplate   string `json:"urlTemplate"`
	CoverURL      string `json:"coverUrl"`
	LastChapter   *int   `json:"lastChapter,omitempty"`
	PrimaryDomain string `json:"primaryDomain"`
}

// ScrapeResult represents the result of a scrape search or browse operation.
type ScrapeResult struct {
	Items      []ScrapedItem `json:"items"`
	Page       int           `json:"page"`
	TotalPages int           `json:"totalPages"`
	Warning    string        `json:"warning,omitempty"`
}

// CatalogScraper defines the contract for domain-specific fast scrapers.
type CatalogScraper interface {
	Domain() string
	Name() string
	SearchOrBrowse(ctx context.Context, query string, page int) (*ScrapeResult, error)
}
