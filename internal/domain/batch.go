package domain

// BatchChapterImage represents an individual image panel in a batch chapter download.
type BatchChapterImage struct {
	Index    int    `json:"index"`
	URL      string `json:"url"`
	ProxyURL string `json:"proxyUrl"`
}

// BatchChapterResult contains the extraction outcome for a single chapter within a batch.
type BatchChapterResult struct {
	Chapter int                 `json:"chapter"`
	Success bool                `json:"success"`
	Images  []BatchChapterImage `json:"images"`
	Error   string              `json:"error,omitempty"`
}

// BatchChaptersResponse represents the payload returned by batch download endpoints.
type BatchChaptersResponse struct {
	CanonicalManhuaID int64                `json:"canonicalManhuaId"`
	Slug              string               `json:"slug"`
	Title             string               `json:"title"`
	Start             int                  `json:"start"`
	End               int                  `json:"end"`
	TotalChapters     int                  `json:"totalChapters"`
	Chapters          []BatchChapterResult `json:"chapters"`
}
