package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/service"
)

type mockLibraryRepo struct {
	items map[string]*domain.UserLibrary
}

func newMockLibraryRepo() *mockLibraryRepo {
	return &mockLibraryRepo{items: make(map[string]*domain.UserLibrary)}
}

func (m *mockLibraryRepo) Create(ctx context.Context, item *domain.UserLibrary) error {
	m.items[item.ID] = item
	return nil
}

func (m *mockLibraryRepo) GetByID(ctx context.Context, userID, id string) (*domain.UserLibrary, error) {
	it, ok := m.items[id]
	if !ok || it.UserID != userID {
		return nil, repository.ErrLibraryItemNotFound
	}
	return it, nil
}

func (m *mockLibraryRepo) GetByUserAndManhuaID(ctx context.Context, userID string, manhuaID int64) (*domain.UserLibrary, error) {
	for _, it := range m.items {
		if it.UserID == userID && it.CanonicalManhuaID == manhuaID {
			return it, nil
		}
	}
	return nil, repository.ErrLibraryItemNotFound
}

func (m *mockLibraryRepo) ListByUser(ctx context.Context, userID string) ([]*domain.UserLibrary, error) {
	var list []*domain.UserLibrary
	for _, it := range m.items {
		if it.UserID == userID {
			list = append(list, it)
		}
	}

	// Sort canonical order: (isFavorite DESC, customOrder ASC, updatedAt DESC)
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if shouldSwap(list[i], list[j]) {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	return list, nil
}

func shouldSwap(a, b *domain.UserLibrary) bool {
	if a.IsFavorite != b.IsFavorite {
		return !a.IsFavorite && b.IsFavorite
	}
	if a.CustomOrder != b.CustomOrder {
		return a.CustomOrder > b.CustomOrder
	}
	return a.UpdatedAt < b.UpdatedAt
}

func (m *mockLibraryRepo) Update(ctx context.Context, item *domain.UserLibrary) error {
	if _, ok := m.items[item.ID]; !ok {
		return repository.ErrLibraryItemNotFound
	}
	m.items[item.ID] = item
	return nil
}

func (m *mockLibraryRepo) UpdateOrderBatch(ctx context.Context, userID string, items []*domain.UserLibrary) error {
	for _, it := range items {
		if existing, ok := m.items[it.ID]; ok && existing.UserID == userID {
			existing.CustomOrder = it.CustomOrder
			existing.IsFavorite = it.IsFavorite
		}
	}
	return nil
}

func (m *mockLibraryRepo) Delete(ctx context.Context, userID, id string) error {
	it, ok := m.items[id]
	if !ok || it.UserID != userID {
		return repository.ErrLibraryItemNotFound
	}
	delete(m.items, id)
	return nil
}

func (m *mockLibraryRepo) GetMinCustomOrder(ctx context.Context, userID string, isFavorite bool) (int, error) {
	minOrder := 0
	found := false
	for _, it := range m.items {
		if it.UserID == userID && it.IsFavorite == isFavorite {
			if !found || it.CustomOrder < minOrder {
				minOrder = it.CustomOrder
				found = true
			}
		}
	}
	if !found {
		return 0, nil
	}
	return minOrder, nil
}

type mockCanonicalLookup struct {
	manhuas map[int64]*domain.CanonicalManhua
}

func (m *mockCanonicalLookup) GetByID(ctx context.Context, id int64) (*domain.CanonicalManhua, error) {
	if item, ok := m.manhuas[id]; ok {
		return item, nil
	}
	return nil, repository.ErrCanonicalNotFound
}

func TestLibraryService_GoldenRuleOfFavorites(t *testing.T) {
	repo := newMockLibraryRepo()
	lastCh := 100
	canonicalLookup := &mockCanonicalLookup{
		manhuas: map[int64]*domain.CanonicalManhua{
			1: {ID: 1, Title: "Work 1", LastChapter: &lastCh},
			2: {ID: 2, Title: "Work 2", LastChapter: &lastCh},
			3: {ID: 3, Title: "Work 3", LastChapter: &lastCh},
		},
	}
	svc := service.NewLibraryService(repo, canonicalLookup)
	ctx := context.Background()
	userID := "user-123"

	// Add Work 1 (not favorite)
	item1, err := svc.AddToLibrary(ctx, userID, domain.AddToLibraryDTO{CanonicalManhuaID: 1, IsFavorite: false})
	require.NoError(t, err)

	// Add Work 2 (favorite)
	item2, err := svc.AddToLibrary(ctx, userID, domain.AddToLibraryDTO{CanonicalManhuaID: 2, IsFavorite: true})
	require.NoError(t, err)

	// Add Work 3 (not favorite)
	item3, err := svc.AddToLibrary(ctx, userID, domain.AddToLibraryDTO{CanonicalManhuaID: 3, IsFavorite: false})
	require.NoError(t, err)

	list, err := svc.GetUserLibrary(ctx, userID)
	require.NoError(t, err)
	require.Len(t, list, 3)

	// RN-04.1: Favorite must be listed first, even if added after non-favorite!
	assert.True(t, list[0].IsFavorite)
	assert.Equal(t, item2.ID, list[0].ID)
	assert.False(t, list[1].IsFavorite)
	assert.False(t, list[2].IsFavorite)
	assert.ElementsMatch(t, []string{item1.ID, item3.ID}, []string{list[1].ID, list[2].ID})
}

func TestLibraryService_AutomaticPromotionOnProgress(t *testing.T) {
	repo := newMockLibraryRepo()
	lastCh := 100
	canonicalLookup := &mockCanonicalLookup{
		manhuas: map[int64]*domain.CanonicalManhua{
			1: {ID: 1, Title: "Work 1", LastChapter: &lastCh},
			2: {ID: 2, Title: "Work 2", LastChapter: &lastCh},
		},
	}
	svc := service.NewLibraryService(repo, canonicalLookup)
	ctx := context.Background()
	userID := "user-456"

	item1, err := svc.AddToLibrary(ctx, userID, domain.AddToLibraryDTO{CanonicalManhuaID: 1, IsFavorite: false})
	require.NoError(t, err)

	item2, err := svc.AddToLibrary(ctx, userID, domain.AddToLibraryDTO{CanonicalManhuaID: 2, IsFavorite: false})
	require.NoError(t, err)

	// Initially item2 is promoted above item1 because it was added last (customOrder is lower or updatedAt is newer)
	list, err := svc.GetUserLibrary(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, item2.ID, list[0].ID)

	// Now user reads chapter 5 of item1 -> RN-03.3 automatic promotion!
	updated, err := svc.UpdateProgress(ctx, userID, item1.ID, 5)
	require.NoError(t, err)
	assert.Equal(t, 5, updated.CurrentChapter)

	// item1 must now be at the TOP of the library!
	listAfter, err := svc.GetUserLibrary(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, item1.ID, listAfter[0].ID)
	assert.Equal(t, item2.ID, listAfter[1].ID)
}

func TestLibraryService_CeilingByLastChapter(t *testing.T) {
	repo := newMockLibraryRepo()
	lastCh := 50
	canonicalLookup := &mockCanonicalLookup{
		manhuas: map[int64]*domain.CanonicalManhua{
			1: {ID: 1, Title: "Work 1", LastChapter: &lastCh},
		},
	}
	svc := service.NewLibraryService(repo, canonicalLookup)
	ctx := context.Background()
	userID := "user-789"

	item, err := svc.AddToLibrary(ctx, userID, domain.AddToLibraryDTO{CanonicalManhuaID: 1, CurrentChapter: 10})
	require.NoError(t, err)

	// Valid progress <= 50
	_, err = svc.UpdateProgress(ctx, userID, item.ID, 50)
	assert.NoError(t, err)

	// RN-03.2: Invalid progress > 50 -> must return error!
	_, err = svc.UpdateProgress(ctx, userID, item.ID, 51)
	assert.Error(t, err)
	assert.Equal(t, service.ErrChapterExceedsLastChapter, err)
}

func TestLibraryService_ReorderingAutoCorrection(t *testing.T) {
	repo := newMockLibraryRepo()
	lastCh := 100
	canonicalLookup := &mockCanonicalLookup{
		manhuas: map[int64]*domain.CanonicalManhua{
			1: {ID: 1, Title: "Fav 1", LastChapter: &lastCh},
			2: {ID: 2, Title: "Fav 2", LastChapter: &lastCh},
			3: {ID: 3, Title: "NonFav 1", LastChapter: &lastCh},
		},
	}
	svc := service.NewLibraryService(repo, canonicalLookup)
	ctx := context.Background()
	userID := "user-reorder"

	fav1, _ := svc.AddToLibrary(ctx, userID, domain.AddToLibraryDTO{CanonicalManhuaID: 1, IsFavorite: true})
	fav2, _ := svc.AddToLibrary(ctx, userID, domain.AddToLibraryDTO{CanonicalManhuaID: 2, IsFavorite: true})
	nonFav1, _ := svc.AddToLibrary(ctx, userID, domain.AddToLibraryDTO{CanonicalManhuaID: 3, IsFavorite: false})

	// Client sends an invalid order trying to put nonFav1 BEFORE fav1: [nonFav1, fav2, fav1]
	reorderDTO := domain.ReorderLibraryDTO{
		ItemIDs: []string{nonFav1.ID, fav2.ID, fav1.ID},
	}
	err := svc.ReorderLibrary(ctx, userID, reorderDTO)
	require.NoError(t, err)

	// RN-04.3: Backend must auto-correct so all favorites stay on top: [fav2, fav1, nonFav1]!
	list, err := svc.GetUserLibrary(ctx, userID)
	require.NoError(t, err)
	require.Len(t, list, 3)

	assert.Equal(t, fav2.ID, list[0].ID)
	assert.Equal(t, fav1.ID, list[1].ID)
	assert.Equal(t, nonFav1.ID, list[2].ID)
}

func TestLibraryService_ToggleFavorite(t *testing.T) {
	repo := newMockLibraryRepo()
	lastCh := 100
	canonicalLookup := &mockCanonicalLookup{
		manhuas: map[int64]*domain.CanonicalManhua{
			1: {ID: 1, Title: "Work 1", LastChapter: &lastCh},
			2: {ID: 2, Title: "Work 2", LastChapter: &lastCh},
		},
	}
	svc := service.NewLibraryService(repo, canonicalLookup)
	ctx := context.Background()
	userID := "user-fav-toggle"

	item1, _ := svc.AddToLibrary(ctx, userID, domain.AddToLibraryDTO{CanonicalManhuaID: 1, IsFavorite: false})
	item2, _ := svc.AddToLibrary(ctx, userID, domain.AddToLibraryDTO{CanonicalManhuaID: 2, IsFavorite: false})

	// Toggle item1 to favorite -> RN-04.2: moves to top of favorites!
	updated, err := svc.ToggleFavorite(ctx, userID, item1.ID, true)
	require.NoError(t, err)
	assert.True(t, updated.IsFavorite)

	list, err := svc.GetUserLibrary(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, item1.ID, list[0].ID)
	assert.True(t, list[0].IsFavorite)
	assert.Equal(t, item2.ID, list[1].ID)
	assert.False(t, list[1].IsFavorite)
}
