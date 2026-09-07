package authoringv1alpha1

import (
	"context"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/scenarios"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// ListScenarios reports every scenario a dungeon may be bound to, and the
// form each one needs filled in (rpg-project#368, design §3.2).
//
// # Verbatim, and that is the whole contract
//
// A descriptor is CONTENT. Each scenario package in the rulebook owns its
// field keys, its labels and -- the part that matters -- its guidance, which
// is the constructor's own refusal text, so the sentence an author reads
// while a blank is empty is the sentence they get back when it is wrong.
// Nothing on this path learns what a captain is, what an artifact is or what
// winning means: this file copies five strings and maps one enum, and there
// is no second copy of any of those words in rpg-api or in the web.
//
// UNGATED. Reading a registry mutates nothing -- GetDungeon's precedent,
// carried from wave 0 -- so unlike PutDungeon it makes no authoring-enabled
// refusal of its own. It is still a signed-in verb, like every RPC on this
// service.
func (h *Handler) ListScenarios(
	ctx context.Context, _ *authoringpb.ListScenariosRequest,
) (*authoringpb.ListScenariosResponse, error) {
	if err := requireAuthenticated(ctx); err != nil {
		return nil, err
	}

	out, err := h.orch.ListScenarios(ctx, &authoringorch.ListScenariosInput{})
	if err != nil {
		return nil, statusError(err)
	}

	descriptors := make([]*authoringpb.ScenarioDescriptor, len(out.Scenarios))
	for i, s := range out.Scenarios {
		descriptors[i] = scenarioDescriptorToProto(s)
	}

	return &authoringpb.ListScenariosResponse{Scenarios: descriptors}, nil
}

// scenarioDescriptorToProto mirrors one scenario onto the wire, field for
// field.
func scenarioDescriptorToProto(s scenarios.Scenario) *authoringpb.ScenarioDescriptor {
	fields := s.Fields()
	out := &authoringpb.ScenarioDescriptor{
		Id:     s.ID(),
		Name:   s.Name(),
		Fields: make([]*authoringpb.ScenarioField, len(fields)),
	}
	for i, f := range fields {
		out.Fields[i] = &authoringpb.ScenarioField{
			Key:      f.Key,
			Label:    f.Label,
			Type:     scenarioFieldTypeToProto(f.Type),
			Kind:     f.Kind,
			Guidance: f.Guidance,
		}
	}

	return out
}

// scenarioFieldTypeToProto maps the toolkit's closed set of widget shapes
// onto the wire's enum.
//
// Kind beside it is NOT mapped and must not be: it is an open string on both
// sides on purpose -- what a picker may point AT is content and grows with
// the rulebook, while the set of widget SHAPES is the builder's own
// vocabulary and grows only when the builder learns to render something new.
// A client that meets a kind it has no picker for shows a raw id field; a
// client cannot render a shape it has never heard of, which is why one is an
// enum and the other is not.
//
// An unrecognized type maps to UNSPECIFIED rather than being guessed at. That
// is the honest answer and it is a PRODUCER defect by construction: the
// toolkit grew a widget shape this build's protos have no value for, so a
// picker cannot be drawn and saying so is better than drawing the wrong one.
func scenarioFieldTypeToProto(t scenarios.FieldType) authoringpb.FieldType {
	switch t {
	case scenarios.FieldEntityRef:
		return authoringpb.FieldType_FIELD_TYPE_ENTITY_REF
	case scenarios.FieldCheck:
		return authoringpb.FieldType_FIELD_TYPE_CHECK
	default:
		return authoringpb.FieldType_FIELD_TYPE_UNSPECIFIED
	}
}
