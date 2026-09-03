package sessionaccess

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

func TestCallerMemberSeated_VerifiesOwnershipAndExactPlayerRow(t *testing.T) {
	cases := []struct {
		name       string
		ctx        context.Context
		member     string
		characters *characterFixture
		roster     *fakeRosterReader
		wantCode   codes.Code
		wantCalls  int
	}{
		{
			name:       "unauthenticated",
			ctx:        context.Background(),
			member:     "char-1",
			characters: newCharacterFixture(map[string]string{"char-1": "alice"}),
			roster:     &fakeRosterReader{output: playerRoster("char-1")},
			wantCode:   codes.Unauthenticated,
		},
		{
			name:       "missing character",
			ctx:        auth.WithPlayerID(context.Background(), "alice"),
			member:     "char-1",
			characters: newCharacterFixture(nil).withGetError("char-1", apierr.NotFound("missing")),
			roster:     &fakeRosterReader{output: playerRoster("char-1")},
			wantCode:   codes.NotFound,
		},
		{
			name:       "foreign owner",
			ctx:        auth.WithPlayerID(context.Background(), "alice"),
			member:     "char-1",
			characters: newCharacterFixture(map[string]string{"char-1": "bob"}),
			roster:     &fakeRosterReader{output: playerRoster("char-1")},
			wantCode:   codes.PermissionDenied,
		},
		{
			name:       "member absent",
			ctx:        auth.WithPlayerID(context.Background(), "alice"),
			member:     "char-1",
			characters: newCharacterFixture(map[string]string{"char-1": "alice"}),
			roster:     &fakeRosterReader{output: playerRoster("char-2")},
			wantCode:   codes.PermissionDenied,
			wantCalls:  1,
		},
		{
			name:       "matching monster row is not a seat",
			ctx:        auth.WithPlayerID(context.Background(), "alice"),
			member:     "char-1",
			characters: newCharacterFixture(map[string]string{"char-1": "alice"}),
			roster:     &fakeRosterReader{output: &sdk.RosterOutput{Members: []sdk.PublicMember{{ID: "char-1", Kind: sdk.KindMonster}}}},
			wantCode:   codes.PermissionDenied,
			wantCalls:  1,
		},
		{
			name:       "matching world row is not a seat",
			ctx:        auth.WithPlayerID(context.Background(), "alice"),
			member:     "char-1",
			characters: newCharacterFixture(map[string]string{"char-1": "alice"}),
			roster:     &fakeRosterReader{output: &sdk.RosterOutput{Members: []sdk.PublicMember{{ID: "char-1", Kind: sdk.KindWorld}}}},
			wantCode:   codes.PermissionDenied,
			wantCalls:  1,
		},
		{
			name:       "owned player seat",
			ctx:        auth.WithPlayerID(context.Background(), "alice"),
			member:     "char-1",
			characters: newCharacterFixture(map[string]string{"char-1": "alice"}),
			roster:     &fakeRosterReader{output: playerRoster("char-1")},
			wantCode:   codes.OK,
			wantCalls:  1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			access, err := New(tc.characters, tc.roster)
			require.NoError(t, err)

			err = access.CallerMemberSeated(tc.ctx, "session-1", tc.member)
			if tc.wantCode == codes.OK {
				require.NoError(t, err)
			} else {
				requireCode(t, err, tc.wantCode)
			}
			require.Len(t, tc.roster.calls, tc.wantCalls)
		})
	}
}

func TestCallerSeated_DelegatesAuthenticationToRosterReader(t *testing.T) {
	reader := &fakeRosterReader{output: playerRoster("char-1")}
	access, err := New(newCharacterFixture(nil), reader)
	require.NoError(t, err)

	err = access.CallerSeated(auth.WithPlayerID(context.Background(), "alice"), "session-1")

	require.NoError(t, err)
	require.Equal(t, []sdk.RosterInput{{Session: "session-1", Player: "alice"}}, reader.calls)
}

func TestCallerSeated_TranslatesRosterErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code codes.Code
	}{
		{name: "no session", err: sdk.ErrNoSession, code: codes.NotFound},
		{name: "no encounter", err: sdk.ErrNoEncounter, code: codes.NotFound},
		{name: "no character", err: sdk.ErrNoCharacter, code: codes.NotFound},
		{name: "bad character", err: sdk.ErrBadCharacter, code: codes.Internal},
		{name: "not seated", err: sdk.ErrNotSeated, code: codes.PermissionDenied},
		{name: "repository failure", err: errors.New("redis unavailable"), code: codes.Internal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			access, err := New(newCharacterFixture(nil), &fakeRosterReader{err: tc.err})
			require.NoError(t, err)

			err = access.CallerSeated(auth.WithPlayerID(context.Background(), "alice"), "session-1")

			requireCode(t, err, tc.code)
		})
	}
}

func TestNew_RequiresCharacterRepository(t *testing.T) {
	access, err := New(nil, &fakeRosterReader{})
	require.Nil(t, access)
	require.EqualError(t, err, "session access: characters repository is required")
}

func TestCallerSeated_WithoutRosterReader_IsInternal(t *testing.T) {
	access, err := New(newCharacterFixture(nil), nil)
	require.NoError(t, err)

	err = access.CallerSeated(auth.WithPlayerID(context.Background(), "alice"), "session-1")

	requireCode(t, err, codes.Internal)
}

func playerRoster(id string) *sdk.RosterOutput {
	return &sdk.RosterOutput{Members: []sdk.PublicMember{{ID: id, Kind: sdk.KindPlayer}}}
}

func requireCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, want, st.Code())
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

type fakeRosterReader struct {
	calls  []sdk.RosterInput
	output *sdk.RosterOutput
	err    error
}

func (r *fakeRosterReader) Roster(_ context.Context, input *sdk.RosterInput) (*sdk.RosterOutput, error) {
	r.calls = append(r.calls, *input)
	return r.output, r.err
}
