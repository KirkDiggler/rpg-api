package authoringv1alpha1

import (
	"context"
	"slices"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/status"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// GetStatusWords reports every word an author may write in a status effect,
// as the rules this server runs publish them (status.Words, rpg-project#1036,
// #1097), and the rulebook release the server was built with.
//
// Verbatim, field for field: the table is the engine's; this file copies it
// and words nothing. A reflection test fails the day the engine adds a field
// this file does not copy.
//
// UNGATED beyond sign-in, for GetMachineWords' reason: reading a table
// mutates nothing.
func (h *Handler) GetStatusWords(
	ctx context.Context, _ *authoringpb.GetStatusWordsRequest,
) (*authoringpb.GetStatusWordsResponse, error) {
	if err := requireAuthenticated(ctx); err != nil {
		return nil, err
	}

	out, err := h.orch.GetStatusWords(ctx, &authoringorch.GetStatusWordsInput{})
	if err != nil {
		return nil, statusError(err)
	}

	return statusWordsToProto(out.Words, out.RulebookVersion), nil
}

// statusWordsToProto is the published table on the wire.
func statusWordsToProto(words []status.Word, version string) *authoringpb.GetStatusWordsResponse {
	out := &authoringpb.GetStatusWordsResponse{
		RulebookVersion: version,
		Words:           make([]*authoringpb.StatusWord, 0, len(words)),
	}
	for _, w := range words {
		word := &authoringpb.StatusWord{
			Word:      w.Word,
			Place:     string(w.Place),
			Shape:     string(w.Shape),
			Values:    make([]*authoringpb.StatusWordValue, 0, len(w.Values)),
			Label:     w.Label,
			WrittenIn: string(w.WrittenIn),
			Holds:     string(w.Holds),
			Required:  w.Required,
			Narrows:   w.Narrows,
			When:      slices.Clone(w.When),
		}
		for _, v := range w.Values {
			word.Values = append(word.Values, &authoringpb.StatusWordValue{
				Word: v.Word, Label: v.Label, When: slices.Clone(v.When),
			})
		}
		if w.Bounds != nil {
			word.Bounds = &authoringpb.StatusNumberBounds{
				Min: int32(w.Bounds.Min), Max: int32(w.Bounds.Max), ZeroAllowed: w.Bounds.ZeroAllowed,
			}
		}
		out.Words = append(out.Words, word)
	}
	return out
}
