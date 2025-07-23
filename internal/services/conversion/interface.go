package conversion

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
)

// DraftConverter handles conversions between storage and domain models for character drafts.
// It provides a centralized location for all conversion logic, making it easier to
// maintain and test.
//
//go:generate mockgen -destination=mock/mock_converter.go -package=conversionmock github.com/KirkDiggler/rpg-api/internal/services/conversion DraftConverter
type DraftConverter interface {
	// ToCharacterDraft converts a storage model (CharacterDraftData) to a domain model (CharacterDraft).
	// This conversion does NOT hydrate the Race, Subrace, Class, or Background info objects.
	// Use HydrateDraft to populate those fields with data from external sources.
	ToCharacterDraft(data *dnd5e.CharacterDraftData) *dnd5e.CharacterDraft

	// FromCharacterDraft converts a domain model (CharacterDraft) to a storage model (CharacterDraftData).
	// This strips out any hydrated info objects, keeping only the ID references.
	FromCharacterDraft(draft *dnd5e.CharacterDraft) *dnd5e.CharacterDraftData

	// HydrateDraft populates the Race, Subrace, Class, and Background info objects
	// on a CharacterDraft using data from external sources.
	// It returns a new CharacterDraft instance with the info objects populated.
	// Any errors fetching external data will be returned.
	HydrateDraft(ctx context.Context, draft *dnd5e.CharacterDraft) (*dnd5e.CharacterDraft, error)
}
