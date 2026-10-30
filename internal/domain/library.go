package domain

import (
	"time"
)

// UserLibrary represents an item in a user's personal reading library.
type UserLibrary struct {
	ID                string           `json:"id"`
	UserID            string           `json:"userId"`
	CanonicalManhuaID int64            `json:"canonicalManhuaId"`
	CurrentChapter    int              `json:"currentChapter"`
	IsFavorite        bool             `json:"isFavorite"`
	CustomOrder       int              `json:"customOrder"`
	CreatedAt         time.Time        `json:"createdAt"`
	UpdatedAt         int64            `json:"updatedAt"` // Unix timestamp in seconds
	CanonicalManhua   *CanonicalManhua `json:"canonicalManhua,omitempty"`
}

// AddToLibraryDTO represents the payload to add a manhua to a user's library.
type AddToLibraryDTO struct {
	CanonicalManhuaID int64 `json:"canonicalManhuaId"`
	CurrentChapter    int   `json:"currentChapter,omitempty"`
	IsFavorite        bool  `json:"isFavorite,omitempty"`
}

// UpdateProgressDTO represents the payload to update reading progress.
type UpdateProgressDTO struct {
	CurrentChapter int `json:"currentChapter"`
}

// ToggleFavoriteDTO represents the payload to toggle favorite status.
type ToggleFavoriteDTO struct {
	IsFavorite bool `json:"isFavorite"`
}

// ReorderLibraryDTO represents the payload to reorder library items.
type ReorderLibraryDTO struct {
	ItemIDs []string `json:"itemIds"`
}
