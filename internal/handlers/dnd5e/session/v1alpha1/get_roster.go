package sessionv1alpha1

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"

	customizationconverter "github.com/KirkDiggler/rpg-api/internal/converters/customization"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	rosterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/roster"
)

// GetRoster reports the public half of every member (rpg-project#264,
// ideas/characters/presentation): identity — never position, never the sheet.
//
// The row the launch wrote holds identity FACTS (ids, kinds, authored monster
// ref/name); everything a character record owns is read fresh HERE, at serve
// time, so a rename or reclass between encounters is never served stale.
// PublicMemberInfo is the projection's whole contract: name, class/race refs
// and public hair for players, the authored ref for monsters, and an always-set
// Customization shelf. Monsters and players without customization keep it empty.
//
// Authorization: the caller must be SEATED — no member parameter to gate on
// (callerActingAs guards verbs that name one), so the gate is membership
// itself: some player row of this roster must belong to the authenticated
// caller. The check rides the same character loads the response needs anyway.
func (h *Handler) GetRoster(ctx context.Context, req *sessionpb.GetRosterRequest) (*sessionpb.GetRosterResponse, error) {
	playerID, err := authenticatedPlayerID(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetSession() == "" {
		return nil, status.Error(codes.InvalidArgument, "session is required")
	}

	row, err := h.roster.Get(ctx, req.GetSession())
	if err != nil {
		if errors.Is(err, rosterrepo.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "session %q has no roster", req.GetSession())
		}
		return nil, status.Errorf(codes.Internal, "load roster for session %q: %v", req.GetSession(), err)
	}

	members := make([]*sessionpb.PublicMemberInfo, 0, len(row.Members))
	seated := false
	for _, m := range row.Members {
		switch m.Kind {
		case rosterrepo.KindPlayer:
			got, err := h.characters.Get(ctx, characterrepo.GetInput{ID: m.ID})
			if err != nil || got == nil || got.Character == nil || got.Character.Data == nil {
				// A roster row references a character the store no longer
				// serves — a data invariant broken somewhere else, reported
				// as such rather than silently thinning the roster.
				return nil, status.Errorf(codes.Internal, "roster member %q has no character record", m.ID)
			}
			data := got.Character.Data
			if data.PlayerID == playerID {
				seated = true
			}
			customization := &sessionpb.Customization{}
			if got.Character.Appearance != nil && got.Character.Appearance.Hair != nil {
				customization.Hair = customizationconverter.EntityToProto(got.Character.Appearance.Hair)
			}
			members = append(members, &sessionpb.PublicMemberInfo{
				Id:   m.ID,
				Kind: sessionpb.MemberKind_MEMBER_KIND_PLAYER,
				Name: data.Name,
				// The same words the client's own render path already maps
				// to models for the local player.
				ClassRef:      data.ClassID,
				RaceRef:       string(data.RaceID),
				Customization: customization,
			})
		case rosterrepo.KindMonster:
			members = append(members, &sessionpb.PublicMemberInfo{
				Id:            m.ID,
				Kind:          sessionpb.MemberKind_MEMBER_KIND_MONSTER,
				Name:          m.Name,
				MonsterRef:    m.Ref,
				Customization: &sessionpb.Customization{},
			})
		default:
			return nil, status.Errorf(codes.Internal, "roster member %q has unspecified kind", m.ID)
		}
	}

	if !seated {
		return nil, status.Error(codes.PermissionDenied, "caller is not seated in this session")
	}

	return &sessionpb.GetRosterResponse{Members: members}, nil
}
