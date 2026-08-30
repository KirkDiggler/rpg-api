package sessionpresentation

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateDraft_AcceptsValidDraftAndDeepCopies(t *testing.T) {
	draft := validTwoBodyDraft()

	normalized, err := ValidateDraft(&draft)
	require.NoError(t, err)

	require.Equal(t, draft.PresentationID, normalized.PresentationID)
	require.Equal(t, draft.Attempt, normalized.Attempt)
	require.Equal(t, draft.ColliderFingerprint, normalized.ColliderFingerprint)
	require.Equal(t, draft.Bodies, normalized.Bodies)
	require.Equal(t, draft.Contacts, normalized.Contacts)
	require.Equal(t, draft.Terminal, normalized.Terminal)

	draft.ColliderFingerprint[0] = 0xFF
	draft.Bodies[0].DieID = "mutated-body"
	draft.Bodies[0].State.Position.X = 999
	draft.Contacts[0].PrimaryDieID = "mutated-primary"
	draft.Contacts[1].StaticCollider.ColliderID = "wall:mutated"
	draft.Contacts[0].After[0].DieID = "mutated-after"
	draft.Contacts[0].After[0].State.LinearVelocity.Y = 777
	draft.Terminal[0].DieID = "mutated-terminal"
	draft.Terminal[0].State.AngularVelocity.Z = 555

	require.NotEqual(t, draft.ColliderFingerprint[0], normalized.ColliderFingerprint[0])
	require.NotEqual(t, draft.Bodies[0].DieID, normalized.Bodies[0].DieID)
	require.NotEqual(t, draft.Bodies[0].State.Position.X, normalized.Bodies[0].State.Position.X)
	require.NotEqual(t, draft.Contacts[0].PrimaryDieID, normalized.Contacts[0].PrimaryDieID)
	require.NotEqual(t, draft.Contacts[1].StaticCollider.ColliderID, normalized.Contacts[1].StaticCollider.ColliderID)
	require.NotEqual(t, draft.Contacts[0].After[0].DieID, normalized.Contacts[0].After[0].DieID)
	require.NotEqual(t, draft.Contacts[0].After[0].State.LinearVelocity.Y, normalized.Contacts[0].After[0].State.LinearVelocity.Y)
	require.NotEqual(t, draft.Terminal[0].DieID, normalized.Terminal[0].DieID)
	require.NotEqual(t, draft.Terminal[0].State.AngularVelocity.Z, normalized.Terminal[0].State.AngularVelocity.Z)
}

func TestValidateDraft_AcceptsSameStepContactsInCanonicalOrder(t *testing.T) {
	draft := validTwoBodyDraft()
	draft.Contacts = []ContactCheckpoint{
		validDieContact(draft.Bodies[0].DieID, draft.Bodies[1].DieID, 1),
		validStaticContact(draft.Bodies[0].DieID, 1, "door:hall-1"),
		validStaticContact(draft.Bodies[0].DieID, 1, "wall:segment-1"),
		validStaticContact(draft.Bodies[1].DieID, 1, "wall:backstop"),
	}

	normalized, err := ValidateDraft(&draft)
	require.NoError(t, err)
	require.Equal(t, draft.Contacts, normalized.Contacts)
}

func TestValidateDraft_RejectsSameStepContactsOutOfCanonicalOrder(t *testing.T) {
	draft := validTwoBodyDraft()
	draft.Contacts = []ContactCheckpoint{
		validStaticContact(draft.Bodies[0].DieID, 1, "wall:segment-2"),
		validStaticContact(draft.Bodies[0].DieID, 1, "wall:segment-1"),
	}

	_, err := ValidateDraft(&draft)
	require.ErrorIs(t, err, ErrInvalidPlan)
}

func TestValidateDraft_RejectsDuplicateSameStepContact(t *testing.T) {
	draft := validTwoBodyDraft()
	duplicate := validDieContact(draft.Bodies[0].DieID, draft.Bodies[1].DieID, 1)
	draft.Contacts = []ContactCheckpoint{duplicate, duplicate}

	_, err := ValidateDraft(&draft)
	require.ErrorIs(t, err, ErrInvalidPlan)
}

func TestValidateDraft_RejectsInvalidDrafts(t *testing.T) {
	tests := []struct {
		name  string
		build func() *Draft
	}{
		{
			name: "nil draft",
			build: func() *Draft {
				return nil
			},
		},
		{
			name: "missing presentation id",
			build: func() *Draft {
				draft := validDraft()
				draft.PresentationID = ""
				return &draft
			},
		},
		{
			name: "unsafe presentation id",
			build: func() *Draft {
				draft := validDraft()
				draft.PresentationID = " bad"
				return &draft
			},
		},
		{
			name: "schema version zero",
			build: func() *Draft {
				draft := validDraft()
				draft.SchemaVersion = 0
				return &draft
			},
		},
		{
			name: "schema version unknown",
			build: func() *Draft {
				draft := validDraft()
				draft.SchemaVersion = 2
				return &draft
			},
		},
		{
			name: "attempt below minimum",
			build: func() *Draft {
				draft := validDraft()
				draft.Attempt = 0
				return &draft
			},
		},
		{
			name: "attempt above maximum",
			build: func() *Draft {
				draft := validDraft()
				draft.Attempt = 33
				return &draft
			},
		},
		{
			name: "unsupported physics schema",
			build: func() *Draft {
				draft := validDraft()
				draft.PhysicsSchema = PhysicsSchema(99)
				return &draft
			},
		},
		{
			name: "fingerprint wrong length",
			build: func() *Draft {
				draft := validDraft()
				draft.ColliderFingerprint = make([]byte, 31)
				return &draft
			},
		},
		{
			name: "body count below minimum",
			build: func() *Draft {
				draft := validDraft()
				draft.Bodies = nil
				return &draft
			},
		},
		{
			name: "body count above maximum",
			build: func() *Draft {
				draft := validDraft()
				body := draft.Bodies[0]
				draft.Bodies = make([]BodyInitial, 21)
				for i := range draft.Bodies {
					body.DieID = dieIDForIndex(i)
					draft.Bodies[i] = body
				}
				draft.Terminal = make([]BodyTerminal, 21)
				for i := range draft.Terminal {
					terminal := validTerminal(dieIDForIndex(i), 480, TerminalKindSettled)
					draft.Terminal[i] = terminal
				}
				return &draft
			},
		},
		{
			name: "duplicate die ids",
			build: func() *Draft {
				draft := validTwoBodyDraft()
				draft.Bodies[1].DieID = draft.Bodies[0].DieID
				return &draft
			},
		},
		{
			name: "unsafe die id",
			build: func() *Draft {
				draft := validDraft()
				draft.Bodies[0].DieID = "bad id"
				draft.Terminal[0].DieID = "bad id"
				return &draft
			},
		},
		{
			name: "schema rejects non d20 shape",
			build: func() *Draft {
				draft := validDraft()
				draft.Bodies[0].Shape = ShapeD6
				return &draft
			},
		},
		{
			name: "missing body state",
			build: func() *Draft {
				draft := validDraft()
				draft.Bodies[0].State = nil
				return &draft
			},
		},
		{
			name: "missing position vector",
			build: func() *Draft {
				draft := validDraft()
				draft.Bodies[0].State.Position = nil
				return &draft
			},
		},
		{
			name: "non finite position",
			build: func() *Draft {
				draft := validDraft()
				draft.Bodies[0].State.Position.X = math.NaN()
				return &draft
			},
		},
		{
			name: "position out of bounds",
			build: func() *Draft {
				draft := validDraft()
				draft.Bodies[0].State.Position.Z = 4096.01
				return &draft
			},
		},
		{
			name: "linear speed magnitude too large",
			build: func() *Draft {
				draft := validDraft()
				draft.Bodies[0].State.LinearVelocity = &Vector3{X: 64.01}
				return &draft
			},
		},
		{
			name: "angular speed magnitude too large",
			build: func() *Draft {
				draft := validDraft()
				draft.Bodies[0].State.AngularVelocity = &Vector3{Z: 128.01}
				return &draft
			},
		},
		{
			name: "quaternion not normalized",
			build: func() *Draft {
				draft := validDraft()
				draft.Bodies[0].State.Rotation = &Quaternion{X: 0, Y: 0, Z: 0, W: 0.5}
				return &draft
			},
		},
		{
			name: "too many contacts",
			build: func() *Draft {
				draft := validDraft()
				draft.Contacts = make([]ContactCheckpoint, 129)
				for i := range draft.Contacts {
					draft.Contacts[i] = validStaticContact(draft.Bodies[0].DieID, uint32(i+1), "wall:"+strings.Repeat("a", 8))
				}
				return &draft
			},
		},
		{
			name: "contact steps must be non-decreasing",
			build: func() *Draft {
				draft := validDraft()
				draft.Contacts = []ContactCheckpoint{
					validStaticContact(draft.Bodies[0].DieID, 2, "wall:two"),
					validStaticContact(draft.Bodies[0].DieID, 1, "wall:one"),
				}
				return &draft
			},
		},
		{
			name: "contact step below minimum",
			build: func() *Draft {
				draft := validDraft()
				draft.Contacts[0].Step = 0
				return &draft
			},
		},
		{
			name: "contact step above maximum",
			build: func() *Draft {
				draft := validDraft()
				draft.Contacts[0].Step = 481
				return &draft
			},
		},
		{
			name: "static contact kind must be wall or door",
			build: func() *Draft {
				draft := validDraft()
				draft.Contacts[0].StaticCollider.Kind = StaticContactKind(99)
				return &draft
			},
		},
		{
			name: "unsafe collider id",
			build: func() *Draft {
				draft := validDraft()
				draft.Contacts[0].StaticCollider.ColliderID = "wall:bad id"
				return &draft
			},
		},
		{
			name: "terminal must include every body exactly once",
			build: func() *Draft {
				draft := validTwoBodyDraft()
				draft.Terminal = draft.Terminal[:1]
				return &draft
			},
		},
		{
			name: "terminal step below minimum",
			build: func() *Draft {
				draft := validDraft()
				draft.Terminal[0].Step = 0
				return &draft
			},
		},
		{
			name: "terminal step above maximum",
			build: func() *Draft {
				draft := validDraft()
				draft.Terminal[0].Step = 481
				return &draft
			},
		},
		{
			name: "contact cannot occur at terminal step",
			build: func() *Draft {
				draft := validDraft()
				draft.Contacts[0].Step = draft.Terminal[0].Step
				return &draft
			},
		},
		{
			name: "encoded payload too large",
			build: func() *Draft {
				draft := oversizedValidDraft()
				return &draft
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateDraft(tt.build())
			require.ErrorIs(t, err, ErrInvalidPlan)
		})
	}
}

func TestValidateDraft_RejectsStaticCheckpointWithoutPrimaryBodyOnly(t *testing.T) {
	draft := validDraft()
	draft.Contacts[0].After = []BodyCheckpoint{
		validCheckpoint(draft.Bodies[0].DieID),
		validCheckpoint("other-d20"),
	}

	_, err := ValidateDraft(&draft)
	require.ErrorIs(t, err, ErrInvalidPlan)
}

func TestValidateDraft_RejectsDieCheckpointWithoutBothBodies(t *testing.T) {
	draft := validTwoBodyDraft()
	draft.Contacts[0].After = draft.Contacts[0].After[:1]

	_, err := ValidateDraft(&draft)
	require.ErrorIs(t, err, ErrInvalidPlan)
}

func TestValidateCheckpointStateCount_AcceptsMaximum(t *testing.T) {
	require.NoError(t, validateCheckpointStateCount(maxCheckpointStates))
}

func TestValidateCheckpointStateCount_RejectsAboveMaximum(t *testing.T) {
	err := validateCheckpointStateCount(maxCheckpointStates + 1)
	require.ErrorIs(t, err, ErrInvalidPlan)
}

func validDraft() Draft {
	bodyID := "attack-d20"
	return Draft{
		SchemaVersion:       1,
		PresentationID:      "present-1",
		AuthoritySeq:        42,
		Attempt:             1,
		PhysicsSchema:       PhysicsSchemaRapierDungeonD20V1,
		ColliderFingerprint: bytes32(0x11),
		Bodies: []BodyInitial{
			{
				DieID: bodyID,
				Shape: ShapeD20,
				State: validState(1, 2, 3),
			},
		},
		Contacts: []ContactCheckpoint{
			validStaticContact(bodyID, 1, "wall:segment-1"),
		},
		Terminal: []BodyTerminal{
			validTerminal(bodyID, 2, TerminalKindSettled),
		},
	}
}

func validTwoBodyDraft() Draft {
	draft := validDraft()
	draft.Bodies = append(draft.Bodies,
		BodyInitial{DieID: "support-d20", Shape: ShapeD20, State: validState(-1, -2, -3)},
	)
	draft.Contacts = []ContactCheckpoint{
		validDieContact(draft.Bodies[0].DieID, draft.Bodies[1].DieID, 1),
		validStaticContact(draft.Bodies[0].DieID, 2, "door:hall-1"),
	}
	draft.Terminal = []BodyTerminal{
		validTerminal(draft.Bodies[0].DieID, 3, TerminalKindSettled),
		validTerminal(draft.Bodies[1].DieID, 4, TerminalKindOffTable),
	}
	return draft
}

func oversizedValidDraft() Draft {
	draft := validTwoBodyDraft()
	draft.PresentationID = strings.Repeat("p", 128)
	draft.Bodies = make([]BodyInitial, 20)
	draft.Terminal = make([]BodyTerminal, 20)
	for i := 0; i < 20; i++ {
		id := dieIDForIndex(i)
		draft.Bodies[i] = BodyInitial{DieID: id, Shape: ShapeD20, State: validState(float64(i), 0, 0)}
		draft.Terminal[i] = validTerminal(id, 480, TerminalKindSettled)
	}
	draft.Contacts = make([]ContactCheckpoint, 128)
	for i := 0; i < 128; i++ {
		bodyID := draft.Bodies[i%len(draft.Bodies)].DieID
		draft.Contacts[i] = validStaticContact(bodyID, uint32(i+1), colliderIDForIndex(i))
	}
	return draft
}

func validStaticContact(bodyID string, step uint32, colliderID string) ContactCheckpoint {
	return ContactCheckpoint{
		Step:         step,
		PrimaryDieID: bodyID,
		StaticCollider: &StaticColliderContact{
			Kind:       staticKindForCollider(colliderID),
			ColliderID: colliderID,
		},
		After: []BodyCheckpoint{validCheckpoint(bodyID)},
	}
}

func validDieContact(primaryID, otherID string, step uint32) ContactCheckpoint {
	return ContactCheckpoint{
		Step:         step,
		PrimaryDieID: primaryID,
		OtherDieID:   otherID,
		After: []BodyCheckpoint{
			validCheckpoint(primaryID),
			validCheckpoint(otherID),
		},
	}
}

func validCheckpoint(bodyID string) BodyCheckpoint {
	return BodyCheckpoint{
		DieID: bodyID,
		State: validState(0, 0, 0),
	}
}

func validTerminal(bodyID string, step uint32, kind TerminalKind) BodyTerminal {
	return BodyTerminal{
		DieID: bodyID,
		Step:  step,
		Kind:  kind,
		State: validState(0, 0, 0),
	}
}

func validState(x, y, z float64) *RigidBodyState {
	return &RigidBodyState{
		Position:        &Vector3{X: x, Y: y, Z: z},
		Rotation:        &Quaternion{W: 1},
		LinearVelocity:  &Vector3{X: 1, Y: 2, Z: 3},
		AngularVelocity: &Vector3{X: 4, Y: 5, Z: 6},
	}
}

func bytes32(fill byte) []byte {
	out := make([]byte, 32)
	for i := range out {
		out[i] = fill
	}
	return out
}

func dieIDForIndex(i int) string {
	return "die-" + strings.Repeat("x", 120) + string(rune('a'+(i%26)))
}

func colliderIDForIndex(i int) string {
	prefix := "wall:"
	if i%2 == 1 {
		prefix = "door:"
	}
	return prefix + strings.Repeat("c", 250-len(prefix)) + string(rune('a'+(i%26)))
}

func staticKindForCollider(colliderID string) StaticContactKind {
	if strings.HasPrefix(colliderID, "door:") {
		return StaticContactKindDoor
	}
	return StaticContactKindWall
}
