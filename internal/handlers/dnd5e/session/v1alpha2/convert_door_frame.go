package sessionv1alpha2

import (
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

func frameSpanToProto(span *sdk.WallFrameSpan) *sessionpb.WallFrameSpan {
	if span == nil {
		return nil
	}
	return &sessionpb.WallFrameSpan{Before: span.Before, After: span.After}
}

// Copy already-permitted immutable facts, including explicit phase presence.
// This translator neither derives a parent association nor supplies a default.
func doorFrameToProto(frame *sdk.DoorFramePresentation) *sessionpb.DoorFramePresentation {
	if frame == nil {
		return nil
	}
	out := &sessionpb.DoorFramePresentation{Span: frameSpanToProto(frame.Span)}
	for _, face := range frame.FaceFinishes {
		value := &sessionpb.DoorFrameFaceFinish{Side: face.Side, MaterialRef: face.MaterialRef}
		if face.PhaseU != nil {
			phase := *face.PhaseU
			value.PhaseU = &phase
		}
		out.FaceFinishes = append(out.FaceFinishes, value)
	}
	return out
}
