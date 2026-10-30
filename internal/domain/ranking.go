package domain

import "time"

// UserRating represents a user's 1-5 star review of a canonical manhua.
type UserRating struct {
	ID                string    `json:"id"`
	UserID            string    `json:"userId"`
	CanonicalManhuaID int64     `json:"canonicalManhuaId"`
	Score             int       `json:"score"` // 1 to 5
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// RankedManhua represents an entry in a leaderboard (ranking) with its position and score.
type RankedManhua struct {
	Rank      int              `json:"rank"`
	Score     float64          `json:"score"` // Bayesian WR (e.g., 4.75) or total view count
	Canonical *CanonicalManhua `json:"canonical"`
}

// SubmitRatingRequest payload for POST /api/ratings
type SubmitRatingRequest struct {
	CanonicalManhuaID int64 `json:"canonicalManhuaId"`
	Score             int   `json:"score"`
}

// RecordViewRequest payload for POST /api/reader/view
type RecordViewRequest struct {
	CanonicalManhuaID int64 `json:"canonicalManhuaId"`
}
