package authoringv1alpha1

import (
	"context"
	"maps"
	"slices"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// GetMachineWords reports every word a behavior machine's step may use, as
// the engine this server runs publishes them (encounter.MachineWords(),
// rpg-project#997, #1001), and the engine release the server was built with.
//
// # Verbatim, field for field
//
// The table is the engine's; this file copies it and words nothing. Four
// fields cross under a different name, because a wire name must survive
// every target language: BeatWord.Names is `names_someone`, Written.In is
// `part`, Arg.Default is `default_value` (and a read-back connection's Not is
// `negated`, read_back.go). A reflection test fails the day the engine adds
// a field this file does not copy.
//
// UNGATED. Reading a table mutates nothing -- ListWeapons' precedent -- so it
// makes no authoring-enabled refusal of its own. It is still a signed-in
// verb, like every RPC on this service.
func (h *Handler) GetMachineWords(
	ctx context.Context, _ *authoringpb.GetMachineWordsRequest,
) (*authoringpb.GetMachineWordsResponse, error) {
	if err := requireAuthenticated(ctx); err != nil {
		return nil, err
	}

	out, err := h.orch.GetMachineWords(ctx, &authoringorch.GetMachineWordsInput{})
	if err != nil {
		return nil, statusError(err)
	}

	return machineWordsToProto(out.Words, out.EncounterVersion), nil
}

// machineWordsToProto is the published table on the wire.
func machineWordsToProto(w encounter.MachineWordsTable, version string) *authoringpb.GetMachineWordsResponse {
	out := &authoringpb.GetMachineWordsResponse{
		EncounterVersion: version,
		Beats:            make([]*authoringpb.MachineBeatWord, 0, len(w.Beats)),
		Conditions:       make([]*authoringpb.MachineConditionWord, 0, len(w.Conditions)),
		Acts:             make([]*authoringpb.MachineActWord, 0, len(w.Acts)),
		Ops:              make([]*authoringpb.MachineOpWord, 0, len(w.Ops)),
	}
	for _, b := range w.Beats {
		out.Beats = append(out.Beats, &authoringpb.MachineBeatWord{
			Word:         b.Word,
			Open:         string(b.Open),
			Timed:        b.Timed,
			Scoped:       b.Scoped,
			NamesSomeone: b.Names,
			About:        b.About,
			Picks:        b.Picks,
			Label:        b.Label,
			LabelWith:    b.LabelWith,
			Short:        b.Short,
			Long:         b.Long,
			Written:      machineWrittenToProto(b.Written),
			Args:         machineArgsToProto(b.Args),
		})
	}
	for _, c := range w.Conditions {
		out.Conditions = append(out.Conditions, &authoringpb.MachineConditionWord{
			Word:        c.Word,
			Open:        string(c.Open),
			Fact:        c.Fact,
			Kind:        c.Kind,
			Values:      slices.Clone(c.Values),
			On:          slices.Clone(c.On),
			StatesForm:  c.StatesForm,
			Join:        c.Join,
			Label:       c.Label,
			LabelNot:    c.LabelNot,
			ValueLabels: maps.Clone(c.ValueLabels),
			Long:        c.Long,
			Written:     machineWrittenToProto(c.Written),
			Args:        machineArgsToProto(c.Args),
		})
	}
	for _, a := range w.Acts {
		out.Acts = append(out.Acts, &authoringpb.MachineActWord{
			Word:       a.Word,
			On:         slices.Clone(a.On),
			StatesForm: a.StatesForm,
			Label:      a.Label,
			Short:      a.Short,
			Long:       a.Long,
			Written:    machineWrittenToProto(a.Written),
			Args:       machineArgsToProto(a.Args),
		})
	}
	for _, o := range w.Ops {
		out.Ops = append(out.Ops, &authoringpb.MachineOpWord{Op: o.Op, Words: o.Words})
	}
	return out
}

func machineWrittenToProto(w encounter.Written) *authoringpb.MachineWritten {
	return &authoringpb.MachineWritten{Part: w.In, Key: w.Key, Field: w.Field}
}

func machineArgsToProto(args []encounter.Arg) []*authoringpb.MachineArg {
	out := make([]*authoringpb.MachineArg, 0, len(args))
	for _, a := range args {
		out = append(out, &authoringpb.MachineArg{
			Name:         a.Name,
			Goes:         string(a.Goes),
			Field:        a.Field,
			Names:        string(a.Names),
			Kind:         a.Kind,
			KindOf:       a.KindOf,
			Kinds:        slices.Clone(a.Kinds),
			Values:       slices.Clone(a.Values),
			Many:         a.Many,
			Optional:     a.Optional,
			DefaultValue: a.Default,
			Fixed:        a.Fixed,
		})
	}
	return out
}
