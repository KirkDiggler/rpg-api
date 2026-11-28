package authoringv1alpha1

import (
	"context"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/magicitems"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// GetItemWords reports every word an author may write for an item, as the
// rules this server runs publish them (magicitems.Words, rpg-project#850), and
// the rulebook release the server was built with.
//
// Verbatim, field for field: the table is the rules'; this file copies it and
// words nothing and reorders nothing. A reflection test fails the day the
// rules add a field this file does not copy.
//
// UNGATED beyond sign-in, for GetMachineWords' reason: reading a table
// mutates nothing.
func (h *Handler) GetItemWords(
	ctx context.Context, _ *authoringpb.GetItemWordsRequest,
) (*authoringpb.GetItemWordsResponse, error) {
	if err := requireAuthenticated(ctx); err != nil {
		return nil, err
	}

	out, err := h.orch.GetItemWords(ctx, &authoringorch.GetItemWordsInput{})
	if err != nil {
		return nil, statusError(err)
	}

	return itemWordsToProto(out.Words, out.RulebookVersion), nil
}

// itemWordsToProto is the published table on the wire.
func itemWordsToProto(words []magicitems.Word, version string) *authoringpb.GetItemWordsResponse {
	return &authoringpb.GetItemWordsResponse{
		RulebookVersion: version,
		Words:           itemWordList(words),
	}
}

// itemWordList copies a list of words; a value's With goes through it again,
// so the copy is recursive.
func itemWordList(words []magicitems.Word) []*authoringpb.ItemWord {
	out := make([]*authoringpb.ItemWord, 0, len(words))
	for _, w := range words {
		word := &authoringpb.ItemWord{
			Word:     w.Word,
			Place:    string(w.Place),
			Shape:    string(w.Shape),
			Values:   make([]*authoringpb.ItemWordValue, 0, len(w.Values)),
			Label:    w.Label,
			Holds:    string(w.Holds),
			Required: w.Required,
		}
		for _, v := range w.Values {
			word.Values = append(word.Values, &authoringpb.ItemWordValue{
				Word: v.Word, Label: v.Label, With: itemWordList(v.With), NeedsCount: v.NeedsCount,
			})
		}
		if w.Bounds != nil {
			word.Bounds = &authoringpb.ItemNumberBounds{
				Min: int32(w.Bounds.Min), Max: int32(w.Bounds.Max), NoMax: w.Bounds.NoMax,
			}
		}
		out = append(out, word)
	}
	return out
}
