package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	apierr "github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	rosterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/roster"
)

// tombRoster stores a two-player-one-skeleton roster row and returns the
// repo, ready to hand to a Handler literal.
func tombRoster(t *testing.T) rosterrepo.Repository {
	t.Helper()
	repo := rosterrepo.NewInMemory()
	require.NoError(t, repo.Save(context.Background(), &rosterrepo.Data{
		EncounterID: "sess-1",
		Members: []rosterrepo.Member{
			{ID: "char-alice", Kind: rosterrepo.KindPlayer},
			{ID: "char-bob", Kind: rosterrepo.KindPlayer},
			{ID: "skeleton-1", Kind: rosterrepo.KindMonster, Ref: "dnd5e:monsters:skeleton", Name: "Skeleton"},
		},
	}))
	return repo
}

type rosterCharacter struct {
	owner      string
	name       string
	class      string
	race       string
	appearance *entities.Appearance
}

// charactersOf serves complete character records while roster assertions stay
// limited to their public projection.
func charactersOf(ctrl *gomock.Controller, rows map[string]rosterCharacter) characterrepo.Repository {
	repo := charactermock.NewMockRepository(ctrl)
	repo.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in characterrepo.GetInput) (*characterrepo.GetOutput, error) {
			row, ok := rows[in.ID]
			if !ok {
				return nil, apierr.NotFound("character not found")
			}
			return &characterrepo.GetOutput{Character: &entities.Character{
				Data: &tkcharacter.Data{
					ID: in.ID, PlayerID: row.owner, Name: row.name,
					ClassID: row.class, RaceID: races.Race(row.race), HitPoints: 99,
				},
				Appearance: row.appearance,
			}}, nil
		},
	).AnyTimes()
	return repo
}

func rosterHairAppearance() *entities.Appearance {
	color := uint32(0x123456)
	roughness := float32(0.33)
	return &entities.Appearance{Hair: &entities.HairCustomization{
		Scalp: &entities.StyleSelection{
			Kind:     entities.StyleSelectionKindStyle,
			StyleRef: "modular-fantasy-hero:hair:38",
		},
		FacialHair: &entities.StyleSelection{Kind: entities.StyleSelectionKindNone},
		ColorSRGB:  &color,
		Roughness:  &roughness,
	}}
}

func TestGetRoster_Unauthenticated_Errors(t *testing.T) {
	h := &Handler{}
	_, err := h.GetRoster(context.Background(), &sessionpb.GetRosterRequest{Session: "sess-1"})
	requireCode(t, err, codes.Unauthenticated)
}

func TestGetRoster_MissingSession_Errors(t *testing.T) {
	h := &Handler{}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetRoster(ctx, &sessionpb.GetRosterRequest{})
	requireCode(t, err, codes.InvalidArgument)
}

func TestGetRoster_UnknownSession_NotFound(t *testing.T) {
	h := &Handler{roster: rosterrepo.NewInMemory()}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetRoster(ctx, &sessionpb.GetRosterRequest{Session: "sess-none"})
	requireCode(t, err, codes.NotFound)
}

// The whole projection, pinned: player rows read the character record FRESH
// (name, class ref, race ref — the same words the client's local-player path
// already maps to models), project public hair, and carry no private sheet
// fields. Default players and monsters still receive an always-present empty
// Customization shelf.
func TestGetRoster_ProjectsThePublicHalf(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{
		roster: tombRoster(t),
		characters: charactersOf(ctrl, map[string]rosterCharacter{
			"char-alice": {owner: "alice", name: "Alice", class: "fighter", race: "human", appearance: rosterHairAppearance()},
			"char-bob":   {owner: "bob", name: "Bob", class: "rogue", race: "elf"},
		}),
	}

	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.GetRoster(ctx, &sessionpb.GetRosterRequest{Session: "sess-1"})
	require.NoError(t, err)
	require.Len(t, resp.GetMembers(), 3)

	alice, bob, skel := resp.GetMembers()[0], resp.GetMembers()[1], resp.GetMembers()[2]

	require.Equal(t, "char-alice", alice.GetId())
	require.Equal(t, sessionpb.MemberKind_MEMBER_KIND_PLAYER, alice.GetKind())
	require.Equal(t, "Alice", alice.GetName())
	require.Equal(t, "fighter", alice.GetClassRef())
	require.Equal(t, "human", alice.GetRaceRef())
	require.Empty(t, alice.GetMonsterRef(), "a player has no monster ref")
	require.NotNil(t, alice.GetCustomization(), "the shelf is always set")
	hair := alice.GetCustomization().GetHair()
	require.NotNil(t, hair)
	require.Equal(t, "modular-fantasy-hero:hair:38", hair.GetScalp().GetStyleRef())
	require.NotNil(t, hair.GetFacialHair().GetNone())
	require.NotNil(t, hair.ColorSrgb)
	require.Equal(t, uint32(0x123456), hair.GetColorSrgb())
	require.NotNil(t, hair.Roughness)
	require.InDelta(t, 0.33, hair.GetRoughness(), 0.000001)

	require.Equal(t, "rogue", bob.GetClassRef())
	require.NotNil(t, bob.GetCustomization())
	require.Nil(t, bob.GetCustomization().GetHair(), "nil appearance keeps an empty public shelf")

	require.Equal(t, "skeleton-1", skel.GetId())
	require.Equal(t, sessionpb.MemberKind_MEMBER_KIND_MONSTER, skel.GetKind())
	require.Equal(t, "Skeleton", skel.GetName())
	require.Equal(t, "dnd5e:monsters:skeleton", skel.GetMonsterRef())
	require.Empty(t, skel.GetClassRef(), "a monster has no class ref")
	require.NotNil(t, skel.GetCustomization())
	require.Nil(t, skel.GetCustomization().GetHair(), "monster customization remains empty")
}

// The seated gate: a roster is readable by exactly the players sitting in it.
// carol is authenticated and real, but owns neither seat — the same
// public/private boundary #814 draws around the sheet, drawn here around the
// session.
func TestGetRoster_NotSeated_PermissionDenied(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{
		roster: tombRoster(t),
		characters: charactersOf(ctrl, map[string]rosterCharacter{
			"char-alice": {owner: "alice", name: "Alice", class: "fighter", race: "human"},
			"char-bob":   {owner: "bob", name: "Bob", class: "rogue", race: "elf"},
		}),
	}

	ctx := auth.WithPlayerID(context.Background(), "carol")
	_, err := h.GetRoster(ctx, &sessionpb.GetRosterRequest{Session: "sess-1"})
	requireCode(t, err, codes.PermissionDenied)
}

// A roster row naming a character the store cannot serve is a broken
// invariant, reported as Internal rather than silently thinning the roster.
func TestGetRoster_DanglingCharacter_Internal(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{
		roster:     tombRoster(t),
		characters: charactersOf(ctrl, map[string]rosterCharacter{}),
	}

	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetRoster(ctx, &sessionpb.GetRosterRequest{Session: "sess-1"})
	requireCode(t, err, codes.Internal)
}
