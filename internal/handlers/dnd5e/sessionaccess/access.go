// Package sessionaccess centralizes the session handler's caller/member/session authorization gates.
package sessionaccess

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	rosterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/roster"
)

const (
	errNoPlayerID                   = "no player id in context"
	errSessionRequired              = "session is required"
	errMemberRequired               = "member is required"
	errCallerDoesNotControl         = "caller does not control this member"
	errCallerNotSeated              = "caller is not seated in this session"
	errCharactersRepositoryRequired = "session access: characters repository is required"
	errRosterRepositoryRequired     = "session access: roster repository is required"
	rosterMemberNoCharacterFmt      = "roster member %q has no character record"
	sessionHasNoRosterFmt           = "session %q has no roster"
	loadRosterForSessionFmt         = "load roster for session %q: %v"
	memberNotFoundFmt               = "member %q not found"
)

// Access centralizes the session handler's caller/member/session authorization
// checks so multiple presentation RPCs can share the exact same gate.
type Access struct {
	characters characterrepo.Repository
	roster     rosterrepo.Repository
}

// New constructs an Access gate over the character and roster repositories.
func New(characters characterrepo.Repository, roster rosterrepo.Repository) (*Access, error) {
	if characters == nil {
		return nil, errors.New(errCharactersRepositoryRequired)
	}
	if roster == nil {
		return nil, errors.New(errRosterRepositoryRequired)
	}
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

// CallerSeated verifies the authenticated caller owns at least one player seat
// in session's roster.
func (a *Access) CallerSeated(ctx context.Context, session string) error {
	playerID, err := authenticatedPlayerID(ctx)
	if err != nil {
		return err
	}
	if session == "" {
		return status.Error(codes.InvalidArgument, errSessionRequired)
	}
	return a.callerSeated(ctx, session, playerID)
}

// CallerMemberSeated verifies the authenticated caller controls member and
// that the same member is seated in session as a player row.
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

	row, err := a.loadRoster(ctx, session)
	if err != nil {
		return err
	}
	for _, m := range row.Members {
		if m.ID == member && m.Kind == rosterrepo.KindPlayer {
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
		return status.Errorf(codes.NotFound, memberNotFoundFmt, member)
	}
	if out == nil || out.Character == nil || out.Character.Data == nil {
		return status.Errorf(codes.NotFound, memberNotFoundFmt, member)
	}
	if out.Character.Data.PlayerID != playerID {
		return status.Error(codes.PermissionDenied, errCallerDoesNotControl)
	}
	return nil
}

func (a *Access) callerSeated(ctx context.Context, session, playerID string) error {
	row, err := a.loadRoster(ctx, session)
	if err != nil {
		return err
	}

	for _, m := range row.Members {
		if m.Kind != rosterrepo.KindPlayer {
			continue
		}
		got, err := a.characters.Get(ctx, characterrepo.GetInput{ID: m.ID})
		if err != nil || got == nil || got.Character == nil || got.Character.Data == nil {
			return status.Errorf(codes.Internal, rosterMemberNoCharacterFmt, m.ID)
		}
		if got.Character.Data.PlayerID == playerID {
			return nil
		}
	}
	return status.Error(codes.PermissionDenied, errCallerNotSeated)
}

func (a *Access) loadRoster(ctx context.Context, session string) (*rosterrepo.Data, error) {
	row, err := a.roster.Get(ctx, session)
	if err != nil {
		if errors.Is(err, rosterrepo.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, sessionHasNoRosterFmt, session)
		}
		return nil, status.Errorf(codes.Internal, loadRosterForSessionFmt, session, err)
	}
	return row, nil
}
