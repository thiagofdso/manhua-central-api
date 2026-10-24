package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/domain"
	"github.com/thiagofdso/manhua-manager-pro/apps/api-gateway/internal/repository"
)

var (
	ErrChapterExceedsLastChapter = errors.New("current chapter cannot exceed last chapter")
	ErrAlreadyInLibrary          = errors.New("manhua already exists in user library")
	ErrInvalidChapterProgress    = errors.New("chapter number must be greater than zero")
)

type CanonicalLookup interface {
	GetByID(ctx context.Context, id int64) (*domain.CanonicalManhua, error)
}

type LibraryService struct {
	libraryRepo     repository.LibraryRepository
	canonicalLookup CanonicalLookup
}

func NewLibraryService(libraryRepo repository.LibraryRepository, canonicalLookup CanonicalLookup) *LibraryService {
	return &LibraryService{
		libraryRepo:     libraryRepo,
		canonicalLookup: canonicalLookup,
	}
}

func (s *LibraryService) AddToLibrary(ctx context.Context, userID string, dto domain.AddToLibraryDTO) (*domain.UserLibrary, error) {
	if existing, err := s.libraryRepo.GetByUserAndManhuaID(ctx, userID, dto.CanonicalManhuaID); err == nil && existing != nil {
		return nil, ErrAlreadyInLibrary
	}

	canonical, err := s.canonicalLookup.GetByID(ctx, dto.CanonicalManhuaID)
	if err != nil {
		return nil, err
	}

	currentCh := dto.CurrentChapter
	if currentCh <= 0 {
		currentCh = 1
	}

	if canonical.LastChapter != nil && currentCh > *canonical.LastChapter {
		return nil, ErrChapterExceedsLastChapter
	}

	minOrder, err := s.libraryRepo.GetMinCustomOrder(ctx, userID, dto.IsFavorite)
	if err != nil {
		minOrder = 0
	}

	item := &domain.UserLibrary{
		ID:                uuid.New().String(),
		UserID:            userID,
		CanonicalManhuaID: dto.CanonicalManhuaID,
		CurrentChapter:    currentCh,
		IsFavorite:        dto.IsFavorite,
		CustomOrder:       minOrder - 1,
		UpdatedAt:         time.Now().Unix(),
		CanonicalManhua:   canonical,
	}

	if err := s.libraryRepo.Create(ctx, item); err != nil {
		return nil, err
	}

	return item, nil
}

func (s *LibraryService) GetUserLibrary(ctx context.Context, userID string) ([]*domain.UserLibrary, error) {
	return s.libraryRepo.ListByUser(ctx, userID)
}

func (s *LibraryService) UpdateProgress(ctx context.Context, userID, itemID string, newChapter int) (*domain.UserLibrary, error) {
	if newChapter <= 0 {
		return nil, ErrInvalidChapterProgress
	}

	item, err := s.libraryRepo.GetByID(ctx, userID, itemID)
	if err != nil {
		return nil, err
	}

	canonical, err := s.canonicalLookup.GetByID(ctx, item.CanonicalManhuaID)
	if err != nil {
		return nil, err
	}

	// RN-03.2: Teto inviolável por lastChapter
	if canonical.LastChapter != nil && newChapter > *canonical.LastChapter {
		return nil, ErrChapterExceedsLastChapter
	}

	// RN-03.3: Promoção Automática de Leitura
	minOrder, err := s.libraryRepo.GetMinCustomOrder(ctx, userID, item.IsFavorite)
	if err != nil {
		minOrder = 0
	}

	item.CurrentChapter = newChapter
	item.CustomOrder = minOrder - 1
	item.UpdatedAt = time.Now().Unix()

	if err := s.libraryRepo.Update(ctx, item); err != nil {
		return nil, err
	}

	return item, nil
}

func (s *LibraryService) ToggleFavorite(ctx context.Context, userID, itemID string, isFavorite bool) (*domain.UserLibrary, error) {
	item, err := s.libraryRepo.GetByID(ctx, userID, itemID)
	if err != nil {
		return nil, err
	}

	if item.IsFavorite == isFavorite {
		return item, nil
	}

	// RN-04.2: Transição de Favorito com liderança de grupo
	minOrder, err := s.libraryRepo.GetMinCustomOrder(ctx, userID, isFavorite)
	if err != nil {
		minOrder = 0
	}

	item.IsFavorite = isFavorite
	item.CustomOrder = minOrder - 1
	item.UpdatedAt = time.Now().Unix()

	if err := s.libraryRepo.Update(ctx, item); err != nil {
		return nil, err
	}

	return item, nil
}

func (s *LibraryService) ReorderLibrary(ctx context.Context, userID string, dto domain.ReorderLibraryDTO) error {
	allItems, err := s.libraryRepo.ListByUser(ctx, userID)
	if err != nil {
		return err
	}

	itemsMap := make(map[string]*domain.UserLibrary, len(allItems))
	for _, it := range allItems {
		itemsMap[it.ID] = it
	}

	var favorites []*domain.UserLibrary
	var nonFavorites []*domain.UserLibrary
	seen := make(map[string]bool)

	// RN-04.3: Auto-correção de reordenação garantindo que favoritas nunca fiquem abaixo de não-favoritas
	for _, id := range dto.ItemIDs {
		if it, exists := itemsMap[id]; exists {
			seen[id] = true
			if it.IsFavorite {
				favorites = append(favorites, it)
			} else {
				nonFavorites = append(nonFavorites, it)
			}
		}
	}

	// Preservar itens não mencionados
	for _, it := range allItems {
		if !seen[it.ID] {
			if it.IsFavorite {
				favorites = append(favorites, it)
			} else {
				nonFavorites = append(nonFavorites, it)
			}
		}
	}

	// Unir preservando favoritas estritamente no topo
	ordered := append(favorites, nonFavorites...)

	for i, it := range ordered {
		it.CustomOrder = i
	}

	return s.libraryRepo.UpdateOrderBatch(ctx, userID, ordered)
}

func (s *LibraryService) RemoveFromLibrary(ctx context.Context, userID, itemID string) error {
	return s.libraryRepo.Delete(ctx, userID, itemID)
}
