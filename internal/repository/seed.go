package repository

import (
	"context"
	"log"

	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
)

// SeedInitialCanonicalManhuas populates canonical_manhuas if it is currently empty.
func SeedInitialCanonicalManhuas(ctx context.Context, repo CanonicalRepository) error {
	res, err := repo.List(ctx, domain.CatalogFilter{Page: 1, PageSize: 1})
	if err != nil {
		return err
	}
	if res.Total > 0 {
		return nil // already seeded
	}

	log.Println("[INFO] Seeding initial canonical manhuas into catalog...")

	overgearedAlt := "Overgeared, Tempal"
	overgearedSyn := "Shin Youngwoo has had an unfortunate life and is now stuck carrying bricks on construction sites. But when he discovers a mythical quest in the VR game Satisfy, he becomes the legendary blacksmith Grid!"
	overgearedCover := "https://cdn.asurascans.com/covers/overgeared.webp"
	overgearedLast := 344
	var overgearedDay int16 = 4

	extrasAlt := "The Extra's Academy Survival Guide"
	extrasSyn := "Ed Rothstaylor was known as a fallen noble extra in the fantasy academy game. Reincarnated into Ed's body, he decides to survive and live a peaceful life away from the protagonist's trouble."
	extrasCover := "https://cdn.asurascans.com/covers/extras.webp"
	extrasLast := 128
	var extrasDay int16 = 3

	playerAlt := "Player Who Returned 10,000 Years Later"
	playerSyn := "After 10,000 years trapped in hell devouring demon kings, Kang Woo finally returns to Earth. But the world is no longer what it used to be."
	playerCover := "https://cdn.asurascans.com/covers/player-10000.webp"
	playerLast := 163
	var playerDay int16 = 2

	barbarianAlt := "Surviving The Game as a Barbarian"
	barbarianSyn := "Lee Hansoo beat Dungeon and Stone after 9 years. But when he woke up, he was inside the game as a barbarian named Bjorn Jandel."
	barbarianCover := "https://cdn.asurascans.com/covers/barbarian.webp"
	barbarianLast := 166
	var barbarianDay int16 = 3

	pigAlt := "Almighty Pig Slaughtering Knife, Pig Butcher Knife"
	pigSyn := "In a world of martial arts and cultivation, an ordinary pig butcher discovers his daily knife technique matches the ancient heavenly blade arts."
	pigCover := "https://ravenscans.org/wp-content/uploads/knife.jpg"
	pigLast := 66
	var pigDay int16 = 2

	mountHuaAlt := "Return of the Mount Hua Sect, Blossom Sect"
	mountHuaSyn := "Chung Myung, the 13th disciple of the Mount Hua Sect, woke up a hundred years later to find his beloved sect in ruins. He vows to restore it to glory."
	mountHuaCover := "https://ravenscans.org/wp-content/uploads/hua.jpg"
	mountHuaLast := 142
	var mountHuaDay int16 = 1

	initialItems := []*domain.CanonicalManhua{
		{
			ID:                1,
			Slug:              "overgeared",
			Title:             "Overgeared",
			AlternativeTitles: &overgearedAlt,
			Synopsis:          &overgearedSyn,
			CoverURL:          &overgearedCover,
			PrimaryDomain:     "asurascans.com",
			URLTemplate:       "https://asurascans.com/comics/overgeared-11fe7da3/chapter/{chapter}",
			LastChapter:       &overgearedLast,
			UpdateDay:         &overgearedDay,
			IsHiatus:          false,
			BayesianRating:    4.76,
			TotalVotes:        1450,
			TotalViews:        280000,
		},
		{
			ID:                1792485389283,
			Slug:              "the-extras-academy-survival-guide",
			Title:             "The Extra's Academy Survival Guide",
			AlternativeTitles: &extrasAlt,
			Synopsis:          &extrasSyn,
			CoverURL:          &extrasCover,
			PrimaryDomain:     "asurascans.com",
			URLTemplate:       "https://asurascans.com/comics/the-extras-academy-survival-guide-11fe7da3/chapter/{chapter}",
			LastChapter:       &extrasLast,
			UpdateDay:         &extrasDay,
			IsHiatus:          false,
			BayesianRating:    4.69,
			TotalVotes:        890,
			TotalViews:        195000,
		},
		{
			ID:                1792485866905,
			Slug:              "player-who-returned-10000-years-later",
			Title:             "Player Who Returned 10,000 Years Later",
			AlternativeTitles: &playerAlt,
			Synopsis:          &playerSyn,
			CoverURL:          &playerCover,
			PrimaryDomain:     "asurascans.com",
			URLTemplate:       "https://asurascans.com/comics/player-who-returned-10000-years-later-11fe7da3/chapter/{chapter}",
			LastChapter:       &playerLast,
			UpdateDay:         &playerDay,
			IsHiatus:          false,
			BayesianRating:    4.48,
			TotalVotes:        620,
			TotalViews:        142000,
		},
		{
			ID:                1792485941434,
			Slug:              "surviving-the-game-as-a-barbarian",
			Title:             "Surviving the Game as a Barbarian",
			AlternativeTitles: &barbarianAlt,
			Synopsis:          &barbarianSyn,
			CoverURL:          &barbarianCover,
			PrimaryDomain:     "asurascans.com",
			URLTemplate:       "https://asurascans.com/comics/surviving-the-game-as-a-barbarian-11fe7da3/chapter/{chapter}",
			LastChapter:       &barbarianLast,
			UpdateDay:         &barbarianDay,
			IsHiatus:          false,
			BayesianRating:    4.81,
			TotalVotes:        1820,
			TotalViews:        320000,
		},
		{
			ID:                1792486063617,
			Slug:              "almighty-pig-slaughtering-knife",
			Title:             "Almighty Pig Slaughtering Knife",
			AlternativeTitles: &pigAlt,
			Synopsis:          &pigSyn,
			CoverURL:          &pigCover,
			PrimaryDomain:     "ravenscans.org",
			URLTemplate:       "https://ravenscans.org/almighty-pig-slaughtering-knife-chapter-{chapter}/",
			LastChapter:       &pigLast,
			UpdateDay:         &pigDay,
			IsHiatus:          false,
			BayesianRating:    4.35,
			TotalVotes:        340,
			TotalViews:        78000,
		},
		{
			ID:                1792486063618,
			Slug:              "return-of-the-mount-hua-sect",
			Title:             "Return of the Mount Hua Sect",
			AlternativeTitles: &mountHuaAlt,
			Synopsis:          &mountHuaSyn,
			CoverURL:          &mountHuaCover,
			PrimaryDomain:     "ravenscans.org",
			URLTemplate:       "https://ravenscans.org/return-of-the-mount-hua-sect-chapter-{chapter}/",
			LastChapter:       &mountHuaLast,
			UpdateDay:         &mountHuaDay,
			IsHiatus:          false,
			BayesianRating:    4.90,
			TotalVotes:        2900,
			TotalViews:        510000,
		},
	}

	return repo.UpsertBatch(ctx, initialItems)
}
