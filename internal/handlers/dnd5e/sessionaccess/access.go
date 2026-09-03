// Package sessionaccess centralizes the session handler's caller/member/session authorization gates.
package sessionaccess

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

const (
	errNoPlayerID                   = "no player id in context"
	errSessionRequired              = "session is required"
	errMemberRequired               = "member is required"
	errCallerDoesNotControl         = "caller does not control this member"
	errCallerNotSeated              = "caller is not seated in this session"
	errCharactersRepositoryRequired = "session access: characters repository is required"
	errRosterReaderRequired         = "session access: roster reader is required"
	memberNotFoundFmt               = "member %q not found"
	loadMemberFmt                   = "load member %q: %v"
)

// RosterReader is the narrow Session SDK read needed by the access gates.
// *session.Manager satisfies it directly; keeping the interface here avoids
// making authorization depend on the concrete manager or on API-owned storage.
type RosterReader interface {
	Roster(context.Context, *sdk.RosterInput) (*sdk.RosterOutput, error)
}

// Access centralizes the session handler's caller/member/session authorization
// checks so multiple presentation RPCs can share the exact same gate.
type Access struct {
	characters characterrepo.Repository
	roster     RosterReader
}

// New constructs an Access gate over the character repository and Session
// SDK roster reader.
func New(characters characterrepo.Repository, roster RosterReader) (*Access, error) {
	if characters == nil {
		return nil, errors.New(errCharactersRepositoryRequired)
	}
	// The roster reader is optional for CallerActingAs, whose existing
	// ownership gate does not require session membership. Production wiring
	// always supplies the Session Manager so CallerSeated and
	// CallerMemberSeated can use it.
	return &Access{characters: characters, roster: roster}, nil
}

// CallerActingAs verifies the authenticated caller controls member.
func (a *Access) CallerActingAs(ctx context.Context, member string) error {
	playerID, err := authenticatedPlayerID(ctx)
	if err != nil {
		return err
	}
	if member == "" {
		return status.Error(codes.InvalidArgument, errMemberRequired)
	}
	return a.verifyMemberOwnership(ctx, playerID, member)
}

// CallerSeated verifies the authenticated caller is seated in session. The
// Session SDK owns membership and caller-seat authorization; this gate does
// not enumerate character records.
func (a *Access) CallerSeated(ctx context.Context, session string) error {
	playerID, err := authenticatedPlayerID(ctx)
	if err != nil {
		return err
	}
	if session == "" {
		return status.Error(codes.InvalidArgument, errSessionRequired)
	}
	_, err = a.readRoster(ctx, session, playerID)
	return err
}

// CallerMemberSeated verifies the authenticated caller controls member and
// that the same member is seated in session as an exact player row.
func (a *Access) CallerMemberSeated(ctx context.Context, session, member string) error {
	playerID, err := authenticatedPlayerID(ctx)
	if err != nil {
		return err
	}
	if session == "" {
		return status.Error(codes.InvalidArgument, errSessionRequired)
	}
	if member == "" {
		return status.Error(codes.InvalidArgument, errMemberRequired)
	}
	ownershipErr := a.verifyMemberOwnership(ctx, playerID, member)
	if ownershipErr != nil {
		return ownershipErr
	}

	roster, err := a.readRoster(ctx, session, playerID)
	if err != nil {
		return err
	}
	for _, seated := range roster.Members {
		if seated.ID == member && seated.Kind == sdk.KindPlayer {
			return nil
		}
	}
	return status.Error(codes.PermissionDenied, errCallerNotSeated)
}

func authenticatedPlayerID(ctx context.Context) (string, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return "", status.Error(codes.Unauthenticated, errNoPlayerID)
	}
	return playerID, nil
}

func (a *Access) verifyMemberOwnership(ctx context.Context, playerID, member string) error {
	out, err := a.characters.Get(ctx, characterrepo.GetInput{ID: member})
	if err != nil {
		if apierr.IsNotFound(err) {
			return status.Errorf(codes.NotFound, memberNotFoundFmt, member)
		}
		return status.Errorf(codes.Internal, loadMemberFmt, member, err)
	}
	if out == nil || out.Character == nil || out.Character.Data == nil {
		return status.Errorf(codes.NotFound, memberNotFoundFmt, member)
	}
	if out.Character.Data.PlayerID != playerID {
		return status.Error(codes.PermissionDenied, errCallerDoesNotControl)
	}
	return nil
}

func (a *Access) readRoster(ctx context.Context, session, playerID string) (*sdk.RosterOutput, error) {
	if a.roster == nil {
		return nil, status.Error(codes.Internal, errRosterReaderRequired)
	}
	out, err := a.roster.Roster(ctx, &sdk.RosterInput{Session: session, Player: playerID})
	if err != nil {
		return nil, rosterError(err)
	}
	if out == nil {
		return nil, status.Error(codes.Internal, "roster returned no output")
	}
	return out, nil
}

func rosterError(err error) error {
	switch {
	case errors.Is(err, sdk.ErrNoSession), errors.Is(err, sdk.ErrNoEncounter), errors.Is(err, sdk.ErrNoCharacter):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, sdk.ErrNotSeated):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
