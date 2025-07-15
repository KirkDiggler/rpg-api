// Package external is the location for the dnd5e-api client
package external

//go:generate mockgen -destination=mock/mock_client.go -package=externalmock github.com/KirkDiggler/rpg-api/internal/clients/external Client

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/errors"
)

// Client defines the interface for external API interactions
type Client interface {
	// GetRaceData fetches race information from external source
	GetRaceData(ctx context.Context, raceID string) (*RaceData, error)

	// GetClassData fetches class information from external source
	GetClassData(ctx context.Context, classID string) (*ClassData, error)

	// GetBackgroundData fetches background information from external source
	GetBackgroundData(ctx context.Context, backgroundID string) (*BackgroundData, error)

	// GetSpellData fetches spell information from external source
	GetSpellData(ctx context.Context, spellID string) (*SpellData, error)

	// ListAvailableRaces returns all available races with full details
	// Implementation should handle reference->details conversion internally
	ListAvailableRaces(ctx context.Context) ([]*RaceData, error)

	// ListAvailableClasses returns all available classes with full details
	// Implementation should handle reference->details conversion internally
	ListAvailableClasses(ctx context.Context) ([]*ClassData, error)

	// ListAvailableBackgrounds returns all available backgrounds with full details
	// Implementation should handle reference->details conversion internally
	ListAvailableBackgrounds(ctx context.Context) ([]*BackgroundData, error)
}

type client struct {
}

type Config struct {
}

func (cfg *Config) Validate() error {
	return nil
}

func New(cfg *Config) (Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &client{}, nil
}

func (c *client) GetRaceData(ctx context.Context, raceID string) (*RaceData, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (c *client) GetClassData(ctx context.Context, classID string) (*ClassData, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (c *client) GetBackgroundData(ctx context.Context, backgroundID string) (*BackgroundData, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (c *client) GetSpellData(ctx context.Context, spellID string) (*SpellData, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (c *client) ListAvailableRaces(ctx context.Context) ([]*RaceData, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (c *client) ListAvailableClasses(ctx context.Context) ([]*ClassData, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (c *client) ListAvailableBackgrounds(ctx context.Context) ([]*BackgroundData, error) {
	return nil, errors.Unimplemented("not implemented")
}
