package authoringv1alpha1

import (
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec/readback"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
)

// readBackToProto is the engine's read-back on the wire, field for field
// (design R25), with its folded form beside it (rpg-project#978). It adds,
// drops and rewords nothing: the sentences are the toolkit's, and a
// consumer keys on Kind. folded is computed by the registry, inside the
// read-back's containment; this only maps it.
func readBackToProto(rb *readback.ReadBack, folded []readback.FoldedSection) *authoringpb.ReadBack {
	out := &authoringpb.ReadBack{
		Key:        rb.Key,
		Name:       rb.Name,
		Sections:   make([]*authoringpb.ReadBackSection, 0, len(rb.Sections)),
		NotYetSaid: make([]*authoringpb.ReadBackUnsaid, 0, len(rb.NotYetSaid)),
		Folded:     make([]*authoringpb.ReadBackFoldedSection, 0, len(folded)),
	}
	for _, sec := range folded {
		out.Folded = append(out.Folded, &authoringpb.ReadBackFoldedSection{
			Title: sec.Title,
			Items: foldedItemsToProto(sec.Items),
		})
	}
	for _, sec := range rb.Sections {
		out.Sections = append(out.Sections, readBackSectionToProto(sec))
	}
	for _, u := range rb.NotYetSaid {
		out.NotYetSaid = append(out.NotYetSaid, &authoringpb.ReadBackUnsaid{Path: u.Path, Kind: u.Kind, Why: u.Why})
	}
	return out
}

func readBackSectionToProto(sec readback.Section) *authoringpb.ReadBackSection {
	out := &authoringpb.ReadBackSection{
		Title:  sec.Title,
		Groups: make([]*authoringpb.ReadBackGroup, 0, len(sec.Groups)),
	}
	for _, g := range sec.Groups {
		group := &authoringpb.ReadBackGroup{
			Title:      g.Title,
			About:      readBackSubjectToProto(g.About),
			Statements: make([]*authoringpb.ReadBackStatement, 0, len(g.Statements)),
		}
		for _, st := range g.Statements {
			group.Statements = append(group.Statements, readBackStatementToProto(st))
		}
		out.Groups = append(out.Groups, group)
	}
	return out
}

func readBackStatementToProto(st readback.Statement) *authoringpb.ReadBackStatement {
	out := &authoringpb.ReadBackStatement{
		Kind:     string(st.Kind),
		About:    readBackSubjectToProto(st.About),
		Sentence: st.Sentence,
		Path:     st.Path,
		Connects: make([]*authoringpb.ReadBackConnection, 0, len(st.Connects)),
	}
	for _, c := range st.Connects {
		out.Connects = append(out.Connects, &authoringpb.ReadBackConnection{Kind: c.Kind, Id: c.ID, Role: c.Role})
	}
	return out
}

func readBackSubjectToProto(s readback.Subject) *authoringpb.ReadBackSubject {
	return &authoringpb.ReadBackSubject{Kind: s.Kind, Id: s.ID}
}

// foldedItemsToProto maps folded items at every depth, order kept.
func foldedItemsToProto(items []readback.Item) []*authoringpb.ReadBackFoldedItem {
	out := make([]*authoringpb.ReadBackFoldedItem, 0, len(items))
	for _, it := range items {
		out = append(out, &authoringpb.ReadBackFoldedItem{
			Text:  it.Text,
			Paths: append([]string(nil), it.Paths...),
			Base:  it.Base,
			Items: foldedItemsToProto(it.Items),
		})
	}
	return out
}
