package authoringv1alpha1_test

import (
	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
)

// edge is one connection as a test names it.
type edge struct{ kind, id, role, is, at string }

// edgesAt is every connection of every statement at path (of kind, when
// kind is set), as edges.
func edgesAt(rb *authoringpb.ReadBack, path, kind string) [][]edge {
	var out [][]edge
	for _, st := range statements(rb) {
		if st.GetPath() != path || (kind != "" && st.GetKind() != kind) {
			continue
		}
		var es []edge
		for _, c := range st.GetConnects() {
			es = append(es, edge{c.GetKind(), c.GetId(), c.GetRole(), c.GetIs(), c.GetAt()})
		}
		out = append(out, es)
	}
	return out
}

// anyHolds reports whether one statement's edges hold every edge in want.
func anyHolds(groups [][]edge, want ...edge) bool {
	for _, es := range groups {
		have := map[edge]bool{}
		for _, e := range es {
			have[e] = true
		}
		all := true
		for _, w := range want {
			if !have[w] {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// TestPutDungeon_ValidateOnly_TracesOneNote: who a note is about, who may
// take it and how, and what gave it, on the castle's check (encounter
// v0.229.0, rpg-project#1001), through the wire's existing connection fields.
func (s *ReadBackWireSuite) TestPutDungeon_ValidateOnly_TracesOneNote() {
	resp := s.gradedCastle()
	s.Require().Empty(resp.GetErrors())
	rb := resp.GetReadBack()

	// The vouch line's give, written once in the conversation `past-the-gate`
	// that gate-1 and gate-2 carry (rpg-project#1025): about the party it
	// sees; the guard it is said to takes it by judgement, the other
	// carriers by hearing it. Each carrier's copy said so at its own path;
	// written once, the conversation's path says it for each carrier.
	vouch := edgesAt(rb, "conversations.past-the-gate.lines[0].success[0].give", "line.grant")
	for _, judged := range []struct{ by, heard string }{{"gate-1", "gate-2"}, {"gate-2", "gate-1"}} {
		s.True(anyHolds(vouch,
			edge{"topic", "admitted", "grants", "", ""},
			edge{"who", "party_in_sight", "about", "", "admitted"},
			edge{"who", "spoken_to", "takes", "judgement", "admitted"},
			edge{"who", "carriers_who_witnessed", "takes", "heard", "admitted"},
			edge{"creature", judged.by, "may-note", "judgement", "admitted"},
			edge{"creature", judged.heard, "may-note", "heard", "admitted"},
			edge{"creature", "captain", "may-note", "heard", "admitted"},
			edge{"option", "vouch", "on", "said", ""},
		), "the vouch give said to %s says whom, who may note it and how: %v", judged.by, vouch)
	}

	// The captain's two steps: one reads a note about anyone, one about
	// the one the beat names.
	s.True(anyHolds(edgesAt(rb, "behaviorDefinitions.captain-policy.recipe.states.ready.transitions[2]", "routine.wake"),
		edge{"who", "anyone", "about", "", "admitted"}))
	s.True(anyHolds(edgesAt(rb, "behaviorDefinitions.captain-policy.recipe.states.in-hall.transitions[1]", "routine.wake"),
		edge{"who", "the_one_named", "about", "", "admitted"}))

	// The two new statements: the seizure on sight, and the jail's wanted.
	s.True(anyHolds(edgesAt(rb, "interactions.front-gate.outcomes.arrested[0].arrest", "conversation.outcome"),
		edge{"topic", "arrested", "grants", "", ""},
		edge{"who", "the_taken", "about", "", "arrested"},
		edge{"who", "the_arresting", "takes", "judgement", "arrested"},
		edge{"creature", "captain", "may-note", "judgement", "arrested"},
		edge{"conversation", "front-gate", "enters", "arrested", ""},
	), "the seizure on sight")
	s.True(anyHolds(edgesAt(rb, "interactions.front-gate.outcomes.jailed[0].jail", "conversation.outcome"),
		edge{"topic", "wanted", "grants", "", ""},
		edge{"who", "the_gone", "about", "", "wanted"},
		edge{"who", "the_arresting", "takes", "judgement", "wanted"},
	), "the jail's wanted")
}
