package sessionv1alpha1

import (
	"context"
	"errors"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	customizationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/customization/v1alpha1"
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// GetRoster reports the Session SDK's public identity projection. The SDK
// owns roster membership, character loading, and customization semantics;
// this handler only authenticates, binds the transport fields, and converts
// the result to proto.
func (h *Handler) GetRoster(ctx context.Context, req *sessionpb.GetRosterRequest) (*sessionpb.GetRosterResponse, error) {
	playerID, err := authenticatedPlayerID(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetSession() == "" {
		return nil, statusError(sdk.ErrNoSessionID)
	}

	out, err := h.manager.Roster(ctx, &sdk.RosterInput{
		Session: req.GetSession(),
		Player:  playerID,
	})
	if err != nil {
		return nil, statusError(err)
	}
	if out == nil {
		return nil, statusError(errRosterOutputRequired)
	}
	return rosterToProto(out), nil
}

var errRosterOutputRequired = errors.New("roster returned no output")

func rosterToProto(in *sdk.RosterOutput) *sessionpb.GetRosterResponse {
	members := make([]*sessionpb.PublicMemberInfo, len(in.Members))
	for i, member := range in.Members {
		members[i] = publicMemberToProto(member)
	}
	return &sessionpb.GetRosterResponse{Members: members}
}

func publicMemberToProto(member sdk.PublicMember) *sessionpb.PublicMemberInfo {
	return &sessionpb.PublicMemberInfo{
		Id:            member.ID,
		Kind:          memberKindToProto(member.Kind),
		Name:          member.Name,
		ClassRef:      member.ClassRef,
		RaceRef:       member.RaceRef,
		MonsterRef:    member.MonsterRef,
		Customization: customizationToProto(member.Customization),
		// The side this member is on (rpg-project#375, design §6): the
		// dungeon file's own faction id, or the reserved `party` /
		// `monsters`, as the session resolved it. Free-form on both sides
		// -- factions are content, never an enum -- and always written by
		// the session for a player or a monster, so an empty value here
		// means a world NPC, which is in no faction.
		Faction: member.Faction,
	}
}

func customizationToProto(in sdk.Customization) *sessionpb.Customization {
	return &sessionpb.Customization{
		Hair:   hairCustomizationToProto(in.Hair),
		Outfit: outfitCustomizationToProto(in.Outfit),
	}
}

func hairCustomizationToProto(in *sdk.HairCustomization) *customizationpb.HairCustomization {
	if in == nil {
		return nil
	}
	out := &customizationpb.HairCustomization{
		Scalp:      styleSelectionToProto(in.Scalp),
		FacialHair: styleSelectionToProto(in.FacialHair),
	}
	if in.ColorSRGB != nil {
		out.ColorSrgb = proto.Uint32(*in.ColorSRGB)
	}
	if in.Roughness != nil {
		out.Roughness = proto.Float32(*in.Roughness)
	}
	return out
}

func outfitCustomizationToProto(in *sdk.OutfitCustomization) *customizationpb.OutfitCustomization {
	if in == nil {
		return nil
	}
	out := &customizationpb.OutfitCustomization{}
	if in.PrimaryColorSRGB != nil {
		out.PrimaryColorSrgb = proto.Uint32(*in.PrimaryColorSRGB)
	}
	if in.SecondaryColorSRGB != nil {
		out.SecondaryColorSrgb = proto.Uint32(*in.SecondaryColorSRGB)
	}
	return out
}

func styleSelectionToProto(in *sdk.StyleSelection) *customizationpb.StyleSelection {
	if in == nil {
		return nil
	}
	out := &customizationpb.StyleSelection{}
	switch in.Kind {
	case sdk.StyleSelectionStyle:
		out.Selection = &customizationpb.StyleSelection_StyleRef{StyleRef: in.StyleRef}
	case sdk.StyleSelectionNone:
		out.Selection = &customizationpb.StyleSelection_None{None: &emptypb.Empty{}}
	}
	return out
}
