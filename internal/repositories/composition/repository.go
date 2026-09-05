// Package composition persists guild-owned immutable composition revisions.
package composition

//go:generate mockgen -destination=mock/mock_repository.go -package=compositionmock github.com/KirkDiggler/rpg-api/internal/repositories/composition Repository

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities"
)

// Repository owns composition identities, timestamps, immutable revisions, and guild indexing.
type Repository interface {
	CreateDefinition(context.Context, CreateDefinitionInput) (*CreateDefinitionOutput, error)
	AppendRevision(context.Context, AppendRevisionInput) (*AppendRevisionOutput, error)
	GetRevision(context.Context, GetRevisionInput) (*GetRevisionOutput, error)
	ListDefinitions(context.Context, ListDefinitionsInput) (*ListDefinitionsOutput, error)
}

// CreateDefinitionInput creates a stable definition with its first immutable revision.
type CreateDefinitionInput struct {
	GuildID           string
	CreatedByPlayerID string
	Source            entities.CompositionSource
}

// CreateDefinitionOutput contains the created definition and first revision.
type CreateDefinitionOutput struct {
	Definition *entities.CompositionDefinition
	Revision   *entities.CompositionRevision
}

// AppendRevisionInput appends only when ExpectedHeadRevisionID is still current.
type AppendRevisionInput struct {
	GuildID                string
	DefinitionID           string
	ExpectedHeadRevisionID string
	CreatedByPlayerID      string
	Source                 entities.CompositionSource
}

// AppendRevisionOutput contains the advanced definition and new immutable revision.
type AppendRevisionOutput struct {
	Definition *entities.CompositionDefinition
	Revision   *entities.CompositionRevision
}

// GetRevisionInput addresses one exact immutable revision within a guild and definition.
type GetRevisionInput struct {
	GuildID      string
	DefinitionID string
	RevisionID   string
}

// GetRevisionOutput contains one exact immutable revision.
type GetRevisionOutput struct {
	Revision *entities.CompositionRevision
}

// ListDefinitionsInput scopes the current guild library.
type ListDefinitionsInput struct {
	GuildID string
}

// CurrentDefinition pairs stable metadata with its current immutable head.
type CurrentDefinition struct {
	Definition *entities.CompositionDefinition
	Revision   *entities.CompositionRevision
}

// ListDefinitionsOutput is sorted lexicographically by definition ID.
type ListDefinitionsOutput struct {
	Definitions []*CurrentDefinition
}
