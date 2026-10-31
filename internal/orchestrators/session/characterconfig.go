package session

import (
	"context"
	"errors"
	"fmt"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/repositories/characterconfig"
)

// characterConfigReader is the session SDK's CharacterConfigReader over the
// character configuration store. The SDK reads it at every write verb and
// never writes it (rpg-project#753 R2). A character who has not chosen
// answers sdk.ErrNotFound, which the SDK reads as not chosen.
type characterConfigReader struct {
	repo characterconfig.Repository
}

var _ sdk.CharacterConfigReader = (*characterConfigReader)(nil)

func (r *characterConfigReader) DiscoverySharing(ctx context.Context, character string) (*bool, error) {
	out, err := r.repo.Get(ctx, &characterconfig.GetInput{CharacterID: character})
	if err != nil {
		return nil, err
	}
	if out.Data.DiscoverySharing == nil {
		return nil, sdk.ErrNotFound
	}
	sharing := *out.Data.DiscoverySharing
	return &sharing, nil
}

// ErrCharacterRequired is returned when a preference input names no character.
var ErrCharacterRequired = errors.New("preferences: character id is required")

// Preferences is the character configuration's one writer, for the
// character verb SetPreference and the deprecated session SetDiscoverySharing.
// It takes the character's own guard on the same locker the session SDK uses,
// so a preference write serializes with every other character verb and never
// with a session.
type Preferences struct {
	locker sdk.SessionLocker
	repo   characterconfig.Repository
}

// SetPreferenceInput changes one or more of a character's preferences. A nil
// preference is left as it is.
type SetPreferenceInput struct {
	CharacterID      string
	DiscoverySharing *bool
}

// SetPreferenceOutput is the character's configuration after the change. A
// nil preference has never been chosen.
type SetPreferenceOutput struct {
	DiscoverySharing *bool
}

// GetPreferenceInput names the character whose configuration is read.
type GetPreferenceInput struct {
	CharacterID string
}

// GetPreferenceOutput is the character's configuration. A nil preference has
// never been chosen.
type GetPreferenceOutput struct {
	DiscoverySharing *bool
}

// SetPreference applies the input under the character's guard: read, change,
// write. A session sees the change at that character's next write verb.
func (p *Preferences) SetPreference(ctx context.Context, in *SetPreferenceInput) (*SetPreferenceOutput, error) {
	if in == nil {
		return nil, sdk.ErrNilInput
	}
	if in.CharacterID == "" {
		return nil, ErrCharacterRequired
	}
	guard, err := p.locker.LockCharacter(ctx, &sdk.LockCharacterInput{Character: in.CharacterID})
	if err != nil {
		return nil, fmt.Errorf("lock character %q: %w", in.CharacterID, err)
	}
	defer guard.Release()
	current, err := p.repo.Get(ctx, &characterconfig.GetInput{CharacterID: in.CharacterID})
	if err != nil {
		return nil, err
	}
	next := &characterconfig.Data{DiscoverySharing: current.Data.DiscoverySharing}
	if in.DiscoverySharing != nil {
		sharing := *in.DiscoverySharing
		next.DiscoverySharing = &sharing
	}
	stored, err := p.repo.Put(ctx, &characterconfig.PutInput{CharacterID: in.CharacterID, Data: next})
	if err != nil {
		return nil, err
	}
	return &SetPreferenceOutput{DiscoverySharing: stored.Data.DiscoverySharing}, nil
}

// GetPreference reads the character's configuration. It takes no guard: a
// read sees the last completed write.
func (p *Preferences) GetPreference(ctx context.Context, in *GetPreferenceInput) (*GetPreferenceOutput, error) {
	if in == nil {
		return nil, sdk.ErrNilInput
	}
	if in.CharacterID == "" {
		return nil, ErrCharacterRequired
	}
	out, err := p.repo.Get(ctx, &characterconfig.GetInput{CharacterID: in.CharacterID})
	if err != nil {
		return nil, err
	}
	return &GetPreferenceOutput{DiscoverySharing: out.Data.DiscoverySharing}, nil
}
