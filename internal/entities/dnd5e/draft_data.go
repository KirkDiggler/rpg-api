package dnd5e

// CharacterDraftData represents the raw data for a character draft (storage/transfer layer)
// This is used by repositories for persistence and by handlers for data transfer.
// The orchestrator converts this to a full CharacterDraft with hydrated info objects.
type CharacterDraftData struct {
	ID               string
	PlayerID         string
	SessionID        string
	Name             string
	RaceID           string
	SubraceID        string
	ClassID          string
	BackgroundID     string
	AbilityScores    *AbilityScores
	Alignment        string
	ChoiceSelections []ChoiceSelection
	Progress         CreationProgress
	ExpiresAt        int64
	CreatedAt        int64
	UpdatedAt        int64
}

// ToCharacterDraft converts CharacterDraftData to CharacterDraft
// Note: This does NOT hydrate the Race, Subrace, Class, or Background info objects
// That should be done by the orchestrator using external data sources
func (d *CharacterDraftData) ToCharacterDraft() *CharacterDraft {
	if d == nil {
		return nil
	}

	return &CharacterDraft{
		ID:               d.ID,
		PlayerID:         d.PlayerID,
		SessionID:        d.SessionID,
		Name:             d.Name,
		RaceID:           d.RaceID,
		SubraceID:        d.SubraceID,
		ClassID:          d.ClassID,
		BackgroundID:     d.BackgroundID,
		AbilityScores:    d.AbilityScores,
		Alignment:        d.Alignment,
		ChoiceSelections: d.ChoiceSelections,
		Progress:         d.Progress,
		ExpiresAt:        d.ExpiresAt,
		CreatedAt:        d.CreatedAt,
		UpdatedAt:        d.UpdatedAt,
		// Info objects (Race, Subrace, Class, Background) are left nil
		// They should be populated by the orchestrator
	}
}

// FromCharacterDraft creates CharacterDraftData from a CharacterDraft
// This strips out the hydrated info objects, keeping only the IDs
func FromCharacterDraft(draft *CharacterDraft) *CharacterDraftData {
	if draft == nil {
		return nil
	}

	return &CharacterDraftData{
		ID:               draft.ID,
		PlayerID:         draft.PlayerID,
		SessionID:        draft.SessionID,
		Name:             draft.Name,
		RaceID:           draft.RaceID,
		SubraceID:        draft.SubraceID,
		ClassID:          draft.ClassID,
		BackgroundID:     draft.BackgroundID,
		AbilityScores:    draft.AbilityScores,
		Alignment:        draft.Alignment,
		ChoiceSelections: draft.ChoiceSelections,
		Progress:         draft.Progress,
		ExpiresAt:        draft.ExpiresAt,
		CreatedAt:        draft.CreatedAt,
		UpdatedAt:        draft.UpdatedAt,
	}
}