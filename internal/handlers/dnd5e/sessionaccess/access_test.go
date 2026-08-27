package sessionaccess

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	rosterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/roster"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

func TestCallerMemberSeated(t *testing.T) {
	const (
		sessionID = "session-1"
		member    = "fighter"
		caller    = "player-1"
		foreign   = "player-2"
	)

	rosterWith := func(members ...rosterrepo.Member) *rosterrepo.Data {
		return &rosterrepo.Data{EncounterID: sessionID, Members: members}
	}

	cases := []struct {
		name            string
		ctx             context.Context
		session         string
		member          string
		characters      *characterFixture
		roster          *rosterFixture
		wantCode        codes.Code
		wantMessage     string
		wantCharCalls   int
		wantRosterCalls int
	}{
		{
			name:            "unauthenticated",
			ctx:             context.Background(),
			session:         sessionID,
			member:          member,
			characters:      newCharacterFixture(map[string]string{member: caller}),
			roster:          &rosterFixture{row: rosterWith(rosterrepo.Member{ID: member, Kind: rosterrepo.KindPlayer})},
			wantCode:        codes.Unauthenticated,
			wantMessage:     "no player id in context",
			wantCharCalls:   0,
			wantRosterCalls: 0,
		},
		{
			name:            "empty session",
			ctx:             auth.WithPlayerID(context.Background(), caller),
			session:         "",
			member:          member,
			characters:      newCharacterFixture(map[string]string{member: caller}),
			roster:          &rosterFixture{row: rosterWith(rosterrepo.Member{ID: member, Kind: rosterrepo.KindPlayer})},
			wantCode:        codes.InvalidArgument,
			wantMessage:     "session is required",
			wantCharCalls:   0,
			wantRosterCalls: 0,
		},
		{
			name:            "empty member",
			ctx:             auth.WithPlayerID(context.Background(), caller),
			session:         sessionID,
			member:          "",
			characters:      newCharacterFixture(map[string]string{member: caller}),
			roster:          &rosterFixture{row: rosterWith(rosterrepo.Member{ID: member, Kind: rosterrepo.KindPlayer})},
			wantCode:        codes.InvalidArgument,
			wantMessage:     "member is required",
			wantCharCalls:   0,
			wantRosterCalls: 0,
		},
		{
			name:            "missing character",
			ctx:             auth.WithPlayerID(context.Background(), caller),
			session:         sessionID,
			member:          member,
			characters:      newCharacterFixture(nil).withGetError(member, errors.New("missing")),
			roster:          &rosterFixture{row: rosterWith(rosterrepo.Member{ID: member, Kind: rosterrepo.KindPlayer})},
			wantCode:        codes.NotFound,
			wantMessage:     "member \"fighter\" not found",
			wantCharCalls:   1,
			wantRosterCalls: 0,
		},
		{
			name:            "foreign owner",
			ctx:             auth.WithPlayerID(context.Background(), caller),
			session:         sessionID,
			member:          member,
			characters:      newCharacterFixture(map[string]string{member: foreign}),
			roster:          &rosterFixture{row: rosterWith(rosterrepo.Member{ID: member, Kind: rosterrepo.KindPlayer})},
			wantCode:        codes.PermissionDenied,
			wantMessage:     "caller does not control this member",
			wantCharCalls:   1,
			wantRosterCalls: 0,
		},
		{
			name:            "missing roster",
			ctx:             auth.WithPlayerID(context.Background(), caller),
			session:         sessionID,
			member:          member,
			characters:      newCharacterFixture(map[string]string{member: caller}),
			roster:          &rosterFixture{getErr: rosterrepo.ErrNotFound},
			wantCode:        codes.NotFound,
			wantMessage:     "session \"session-1\" has no roster",
			wantCharCalls:   1,
			wantRosterCalls: 1,
		},
		{
			name:            "member absent from roster",
			ctx:             auth.WithPlayerID(context.Background(), caller),
			session:         sessionID,
			member:          member,
			characters:      newCharacterFixture(map[string]string{member: caller}),
			roster:          &rosterFixture{row: rosterWith(rosterrepo.Member{ID: "wizard", Kind: rosterrepo.KindPlayer})},
			wantCode:        codes.PermissionDenied,
			wantMessage:     "caller is not seated in this session",
			wantCharCalls:   1,
			wantRosterCalls: 1,
		},
		{
			name:            "monster row",
			ctx:             auth.WithPlayerID(context.Background(), caller),
			session:         sessionID,
			member:          member,
			characters:      newCharacterFixture(map[string]string{member: caller}),
			roster:          &rosterFixture{row: rosterWith(rosterrepo.Member{ID: member, Kind: rosterrepo.KindMonster, Ref: "dnd5e:monsters:skeleton", Name: "Skeleton"})},
			wantCode:        codes.PermissionDenied,
			wantMessage:     "caller is not seated in this session",
			wantCharCalls:   1,
			wantRosterCalls: 1,
		},
		{
			name:            "owned player seat",
			ctx:             auth.WithPlayerID(context.Background(), caller),
			session:         sessionID,
			member:          member,
			characters:      newCharacterFixture(map[string]string{member: caller}),
			roster:          &rosterFixture{row: rosterWith(rosterrepo.Member{ID: member, Kind: rosterrepo.KindPlayer})},
			wantCode:        codes.OK,
			wantCharCalls:   1,
			wantRosterCalls: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			access, err := New(tc.characters, tc.roster)
			require.NoError(t, err)

			err = access.CallerMemberSeated(tc.ctx, tc.session, tc.member)
			if tc.wantCode == codes.OK {
				require.NoError(t, err)
			} else {
				requireStatus(t, err, tc.wantCode, tc.wantMessage)
			}
			require.Len(t, tc.characters.getCalls, tc.wantCharCalls)
			require.Len(t, tc.roster.getCalls, tc.wantRosterCalls)
		})
	}
}

func TestCallerActingAs_OwnedMemberPasses(t *testing.T) {
	access, err := New(
		newCharacterFixture(map[string]string{"fighter": "player-1"}),
		&rosterFixture{},
	)
	require.NoError(t, err)

	err = access.CallerActingAs(auth.WithPlayerID(context.Background(), "player-1"), "fighter")
	require.NoError(t, err)
}

func TestCallerSeated_OwnedPlayerInRosterPasses(t *testing.T) {
	characters := newCharacterFixture(map[string]string{"fighter": "player-1"})
	roster := &rosterFixture{row: &rosterrepo.Data{
		EncounterID: "session-1",
		Members:     []rosterrepo.Member{{ID: "fighter", Kind: rosterrepo.KindPlayer}},
	}}
	access, err := New(characters, roster)
	require.NoError(t, err)

	err = access.CallerSeated(auth.WithPlayerID(context.Background(), "player-1"), "session-1")
	require.NoError(t, err)
}

func TestNew_NilCharacters_Errors(t *testing.T) {
	access, err := New(nil, &rosterFixture{})
	require.Nil(t, access)
	require.EqualError(t, err, "session access: characters repository is required")
}

func TestNew_NilRoster_Errors(t *testing.T) {
	access, err := New(newCharacterFixture(nil), nil)
	require.Nil(t, access)
	require.EqualError(t, err, "session access: roster repository is required")
}

func requireStatus(t *testing.T, err error, wantCode codes.Code, wantMessage string) {
	t.Helper()
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, wantCode, st.Code())
	require.Equal(t, wantMessage, st.Message())
}

type characterFixture struct {
	getCalls []string
	players  map[string]string
	getErrs  map[string]error
}

func newCharacterFixture(players map[string]string) *characterFixture {
	return &characterFixture{players: players, getErrs: map[string]error{}}
}

func (r *characterFixture) withGetError(id string, err error) *characterFixture {
	r.getErrs[id] = err
	return r
}

func (r *characterFixture) Create(context.Context, characterrepo.CreateInput) (*characterrepo.CreateOutput, error) {
	return nil, errors.New("not implemented")
}

func (r *characterFixture) Get(_ context.Context, input characterrepo.GetInput) (*characterrepo.GetOutput, error) {
	r.getCalls = append(r.getCalls, input.ID)
	if err, ok := r.getErrs[input.ID]; ok {
		return nil, err
	}
	playerID, ok := r.players[input.ID]
	if !ok {
		return nil, errors.New("not found")
	}
	return &characterrepo.GetOutput{Character: &entities.Character{Data: &tkcharacter.Data{ID: input.ID, PlayerID: playerID}}}, nil
}

func (r *characterFixture) Update(context.Context, characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
	return nil, errors.New("not implemented")
}

func (r *characterFixture) PatchEquipment(context.Context, characterrepo.PatchEquipmentInput) (*characterrepo.PatchEquipmentOutput, error) {
	return nil, errors.New("not implemented")
}

func (r *characterFixture) Delete(context.Context, characterrepo.DeleteInput) (*characterrepo.DeleteOutput, error) {
	return nil, errors.New("not implemented")
}

func (r *characterFixture) ListByPlayerID(context.Context, characterrepo.ListByPlayerIDInput) (*characterrepo.ListByPlayerIDOutput, error) {
	return nil, errors.New("not implemented")
}

func (r *characterFixture) ListBySessionID(context.Context, characterrepo.ListBySessionIDInput) (*characterrepo.ListBySessionIDOutput, error) {
	return nil, errors.New("not implemented")
}

type rosterFixture struct {
	getCalls []string
	row      *rosterrepo.Data
	getErr   error
}

func (r *rosterFixture) Get(_ context.Context, encounterID string) (*rosterrepo.Data, error) {
	r.getCalls = append(r.getCalls, encounterID)
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.row, nil
}

func (r *rosterFixture) Save(context.Context, *rosterrepo.Data) error {
	return errors.New("not implemented")
}
