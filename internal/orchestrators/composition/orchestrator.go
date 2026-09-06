// Package composition implements the world composition service over its repository.
package composition

import (
	"context"
	"encoding/json"

	worldcomposition "github.com/KirkDiggler/rpg-toolkit/world/composition"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	compositionrepo "github.com/KirkDiggler/rpg-api/internal/repositories/composition"
	compositionservice "github.com/KirkDiggler/rpg-api/internal/services/composition"
)

// Config contains the orchestrator dependencies.
type Config struct {
	Repository  compositionrepo.Repository
	IDGenerator idgen.Generator
}

type orchestrator struct {
	repository  compositionrepo.Repository
	idGenerator idgen.Generator
}

// New creates a world composition service.
func New(cfg *Config) (compositionservice.Service, error) {
	if cfg == nil {
		return nil, apierr.InvalidArgument("composition orchestrator config is required")
	}
	if cfg.Repository == nil {
		return nil, apierr.InvalidArgument("composition repository is required")
	}
	if cfg.IDGenerator == nil {
		return nil, apierr.InvalidArgument("composition ID generator is required")
	}
	return &orchestrator{repository: cfg.Repository, idGenerator: cfg.IDGenerator}, nil
}

func (o *orchestrator) Create(ctx context.Context, input *compositionservice.CreateInput) (*compositionservice.CreateOutput, error) {
	if err := validateCreateInput(input); err != nil {
		return nil, err
	}

	compositionID := o.idGenerator.Generate()
	if compositionID == "" {
		return nil, apierr.Internal("composition ID generator returned an empty ID")
	}

	created, err := o.repository.Create(ctx, &compositionrepo.CreateInput{Composition: &worldcomposition.Data{
		ID:      compositionID,
		WorldID: input.WorldID,
		JSON:    append(json.RawMessage(nil), input.JSON...),
	}})
	if err != nil {
		return nil, apierr.Wrap(err, "create composition")
	}
	if created == nil || created.Composition == nil {
		return nil, apierr.Internal("composition repository returned no created composition")
	}
	return &compositionservice.CreateOutput{Composition: created.Composition}, nil
}

func (o *orchestrator) Get(ctx context.Context, input *compositionservice.GetInput) (*compositionservice.GetOutput, error) {
	if err := validateGetInput(input); err != nil {
		return nil, err
	}

	got, err := o.repository.Get(ctx, &compositionrepo.GetInput{
		WorldID: input.WorldID,
		ID:      input.CompositionID,
	})
	if err != nil {
		return nil, apierr.Wrap(err, "get composition")
	}
	if got == nil || got.Composition == nil {
		return nil, apierr.Internal("composition repository returned no composition")
	}
	return &compositionservice.GetOutput{Composition: got.Composition}, nil
}

func (o *orchestrator) List(ctx context.Context, input *compositionservice.ListInput) (*compositionservice.ListOutput, error) {
	if err := validateListInput(input); err != nil {
		return nil, err
	}

	listed, err := o.repository.List(ctx, &compositionrepo.ListInput{WorldID: input.WorldID})
	if err != nil {
		return nil, apierr.Wrap(err, "list compositions")
	}
	if listed == nil {
		return nil, apierr.Internal("composition repository returned no list output")
	}
	return &compositionservice.ListOutput{Compositions: listed.Compositions}, nil
}

func validateCreateInput(input *compositionservice.CreateInput) error {
	if input == nil {
		return apierr.InvalidArgument("create composition input is required")
	}
	if err := validateCallerAndWorld(input.PlayerID, input.WorldID); err != nil {
		return err
	}
	if len(input.JSON) == 0 {
		return apierr.InvalidArgument("composition JSON is required")
	}
	if !json.Valid(input.JSON) {
		return apierr.InvalidArgument("composition JSON must be valid JSON")
	}
	return nil
}

func validateGetInput(input *compositionservice.GetInput) error {
	if input == nil {
		return apierr.InvalidArgument("get composition input is required")
	}
	if err := validateCallerAndWorld(input.PlayerID, input.WorldID); err != nil {
		return err
	}
	if input.CompositionID == "" {
		return apierr.InvalidArgument("composition ID is required")
	}
	return nil
}

func validateListInput(input *compositionservice.ListInput) error {
	if input == nil {
		return apierr.InvalidArgument("list compositions input is required")
	}
	return validateCallerAndWorld(input.PlayerID, input.WorldID)
}

func validateCallerAndWorld(playerID, worldID string) error {
	if playerID == "" {
		return apierr.Unauthenticated("player is required")
	}
	if worldID == "" {
		return apierr.InvalidArgument("world ID is required")
	}
	return nil
}
