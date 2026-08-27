package sessionpresentation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

var presentationIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:_-]{0,127}$`)

const (
	contactTargetKindDice = "dice"
	contactTargetKindDoor = "door"
	contactTargetKindWall = "wall"
)

func ValidateDraft(draft *Draft) (Draft, error) {
	if draft == nil {
		return Draft{}, fmt.Errorf("draft is required: %w", ErrInvalidPlan)
	}
	if draft.SchemaVersion != schemaVersionV1 {
		return Draft{}, fmt.Errorf("invalid schema version: %w", ErrInvalidPlan)
	}
	if !presentationIDPattern.MatchString(draft.PresentationID) {
		return Draft{}, fmt.Errorf("invalid presentation id: %w", ErrInvalidPlan)
	}
	if draft.Attempt == 0 || draft.Attempt > maxAttempt {
		return Draft{}, fmt.Errorf("invalid attempt: %w", ErrInvalidPlan)
	}
	if draft.PhysicsSchema != PhysicsSchemaRapierDungeonD20V1 {
		return Draft{}, fmt.Errorf("invalid physics schema: %w", ErrInvalidPlan)
	}
	if len(draft.ColliderFingerprint) != fingerprintLength {
		return Draft{}, fmt.Errorf("invalid fingerprint length: %w", ErrInvalidPlan)
	}
	if len(draft.Bodies) == 0 || len(draft.Bodies) > maxBodies {
		return Draft{}, fmt.Errorf("invalid body count: %w", ErrInvalidPlan)
	}

	normalized := Draft{
		SchemaVersion:       draft.SchemaVersion,
		PresentationID:      draft.PresentationID,
		AuthoritySeq:        draft.AuthoritySeq,
		Attempt:             draft.Attempt,
		PhysicsSchema:       draft.PhysicsSchema,
		ColliderFingerprint: bytes.Clone(draft.ColliderFingerprint),
		Bodies:              make([]BodyInitial, 0, len(draft.Bodies)),
		Contacts:            make([]ContactCheckpoint, 0, len(draft.Contacts)),
		Terminal:            make([]BodyTerminal, 0, len(draft.Terminal)),
	}

	bodyIDs := make(map[string]struct{}, len(draft.Bodies))
	for _, body := range draft.Bodies {
		normalizedBody, err := validateBody(body, draft.PhysicsSchema, bodyIDs)
		if err != nil {
			return Draft{}, err
		}
		normalized.Bodies = append(normalized.Bodies, normalizedBody)
	}

	terminalSteps, normalizedTerminal, err := validateTerminal(draft.Terminal, bodyIDs)
	if err != nil {
		return Draft{}, err
	}
	normalized.Terminal = normalizedTerminal

	totalCheckpointStates := 0
	var previousContact ContactCheckpoint
	for i, contact := range draft.Contacts {
		normalizedContact, contactErr := validateContact(contact, bodyIDs, terminalSteps)
		if contactErr != nil {
			return Draft{}, contactErr
		}
		if i > 0 {
			if normalizedContact.Step < previousContact.Step {
				return Draft{}, fmt.Errorf("decreasing contact step: %w", ErrInvalidPlan)
			}
			if normalizedContact.Step == previousContact.Step && compareEqualStepContacts(previousContact, normalizedContact) >= 0 {
				return Draft{}, fmt.Errorf("non-canonical same-step contact order: %w", ErrInvalidPlan)
			}
		}
		previousContact = normalizedContact
		totalCheckpointStates += len(normalizedContact.After)
		if stateCountErr := validateCheckpointStateCount(totalCheckpointStates); stateCountErr != nil {
			return Draft{}, stateCountErr
		}
		normalized.Contacts = append(normalized.Contacts, normalizedContact)
	}
	if len(draft.Contacts) > maxContacts {
		return Draft{}, fmt.Errorf("invalid contact count: %w", ErrInvalidPlan)
	}

	encoded, err := marshalDeterministicDraft(normalized)
	if err != nil {
		return Draft{}, fmt.Errorf("marshal draft: %w", ErrInvalidPlan)
	}
	if len(encoded) > maxEncodedPayloadBytes {
		return Draft{}, fmt.Errorf("encoded plan too large: %w", ErrInvalidPlan)
	}

	return normalized, nil
}

func validateBody(body BodyInitial, schema PhysicsSchema, seen map[string]struct{}) (BodyInitial, error) {
	if !presentationIDPattern.MatchString(body.DieID) {
		return BodyInitial{}, fmt.Errorf("invalid die id: %w", ErrInvalidPlan)
	}
	if _, ok := seen[body.DieID]; ok {
		return BodyInitial{}, fmt.Errorf("duplicate die id: %w", ErrInvalidPlan)
	}
	if schema == PhysicsSchemaRapierDungeonD20V1 && body.Shape != ShapeD20 {
		return BodyInitial{}, fmt.Errorf("unsupported body shape: %w", ErrInvalidPlan)
	}
	normalizedState, err := validateState(body.State)
	if err != nil {
		return BodyInitial{}, err
	}
	seen[body.DieID] = struct{}{}
	return BodyInitial{
		DieID: body.DieID,
		Shape: body.Shape,
		State: normalizedState,
	}, nil
}

func validateTerminal(terminal []BodyTerminal, bodyIDs map[string]struct{}) (map[string]uint32, []BodyTerminal, error) {
	if len(terminal) != len(bodyIDs) {
		return nil, nil, fmt.Errorf("terminal/body mismatch: %w", ErrInvalidPlan)
	}

	steps := make(map[string]uint32, len(terminal))
	normalized := make([]BodyTerminal, 0, len(terminal))
	for _, item := range terminal {
		if _, ok := bodyIDs[item.DieID]; !ok {
			return nil, nil, fmt.Errorf("unknown terminal body: %w", ErrInvalidPlan)
		}
		if _, duplicate := steps[item.DieID]; duplicate {
			return nil, nil, fmt.Errorf("duplicate terminal body: %w", ErrInvalidPlan)
		}
		if item.Step == 0 || item.Step > maxStep {
			return nil, nil, fmt.Errorf("invalid terminal step: %w", ErrInvalidPlan)
		}
		if item.Kind != TerminalKindSettled && item.Kind != TerminalKindOffTable {
			return nil, nil, fmt.Errorf("invalid terminal kind: %w", ErrInvalidPlan)
		}
		state, err := validateState(item.State)
		if err != nil {
			return nil, nil, err
		}
		steps[item.DieID] = item.Step
		normalized = append(normalized, BodyTerminal{
			DieID: item.DieID,
			Step:  item.Step,
			Kind:  item.Kind,
			State: state,
		})
	}
	return steps, normalized, nil
}

func validateContact(contact ContactCheckpoint, bodyIDs map[string]struct{}, terminalSteps map[string]uint32) (ContactCheckpoint, error) {
	if contact.Step == 0 || contact.Step > maxStep {
		return ContactCheckpoint{}, fmt.Errorf("invalid contact step: %w", ErrInvalidPlan)
	}
	if _, ok := bodyIDs[contact.PrimaryDieID]; !ok {
		return ContactCheckpoint{}, fmt.Errorf("unknown primary body: %w", ErrInvalidPlan)
	}

	normalized := ContactCheckpoint{
		Step:         contact.Step,
		PrimaryDieID: contact.PrimaryDieID,
		After:        make([]BodyCheckpoint, 0, len(contact.After)),
	}

	hasStatic := contact.StaticCollider != nil
	hasOther := contact.OtherDieID != ""
	if hasStatic == hasOther {
		return ContactCheckpoint{}, fmt.Errorf("invalid contact target: %w", ErrInvalidPlan)
	}

	if hasStatic {
		if contact.StaticCollider.Kind != StaticContactKindWall && contact.StaticCollider.Kind != StaticContactKindDoor {
			return ContactCheckpoint{}, fmt.Errorf("invalid static contact kind: %w", ErrInvalidPlan)
		}
		if err := validateColliderID(contact.StaticCollider.Kind, contact.StaticCollider.ColliderID); err != nil {
			return ContactCheckpoint{}, err
		}
		if len(contact.After) != 1 || contact.After[0].DieID != contact.PrimaryDieID {
			return ContactCheckpoint{}, fmt.Errorf("invalid static checkpoint bodies: %w", ErrInvalidPlan)
		}
		if contact.Step >= terminalSteps[contact.PrimaryDieID] {
			return ContactCheckpoint{}, fmt.Errorf("contact after terminal: %w", ErrInvalidPlan)
		}
		normalized.StaticCollider = &StaticColliderContact{
			Kind:       contact.StaticCollider.Kind,
			ColliderID: contact.StaticCollider.ColliderID,
		}
	} else {
		if contact.PrimaryDieID == contact.OtherDieID {
			return ContactCheckpoint{}, fmt.Errorf("die contact requires distinct bodies: %w", ErrInvalidPlan)
		}
		if _, ok := bodyIDs[contact.OtherDieID]; !ok {
			return ContactCheckpoint{}, fmt.Errorf("unknown die contact body: %w", ErrInvalidPlan)
		}
		if len(contact.After) != 2 || !matchesExactDiePair(contact.After, contact.PrimaryDieID, contact.OtherDieID) {
			return ContactCheckpoint{}, fmt.Errorf("invalid die checkpoint bodies: %w", ErrInvalidPlan)
		}
		if contact.Step >= terminalSteps[contact.PrimaryDieID] || contact.Step >= terminalSteps[contact.OtherDieID] {
			return ContactCheckpoint{}, fmt.Errorf("contact after terminal: %w", ErrInvalidPlan)
		}
		normalized.OtherDieID = contact.OtherDieID
	}

	for _, checkpoint := range contact.After {
		state, err := validateState(checkpoint.State)
		if err != nil {
			return ContactCheckpoint{}, err
		}
		normalized.After = append(normalized.After, BodyCheckpoint{
			DieID: checkpoint.DieID,
			State: state,
		})
	}

	return normalized, nil
}

func compareEqualStepContacts(left, right ContactCheckpoint) int {
	if cmp := strings.Compare(left.PrimaryDieID, right.PrimaryDieID); cmp != 0 {
		return cmp
	}

	leftKind, leftTargetID := contactTargetOrder(left)
	rightKind, rightTargetID := contactTargetOrder(right)
	if cmp := strings.Compare(leftKind, rightKind); cmp != 0 {
		return cmp
	}

	return strings.Compare(leftTargetID, rightTargetID)
}

func contactTargetOrder(contact ContactCheckpoint) (string, string) {
	if contact.OtherDieID != "" {
		return contactTargetKindDice, contact.OtherDieID
	}
	if contact.StaticCollider.Kind == StaticContactKindDoor {
		return contactTargetKindDoor, contact.StaticCollider.ColliderID
	}
	return contactTargetKindWall, contact.StaticCollider.ColliderID
}

func validateCheckpointStateCount(total int) error {
	if total > maxCheckpointStates {
		return fmt.Errorf("checkpoint bounds exceeded: %w", ErrInvalidPlan)
	}
	return nil
}

func matchesExactDiePair(after []BodyCheckpoint, primaryID, otherID string) bool {
	seen := map[string]int{}
	for _, checkpoint := range after {
		seen[checkpoint.DieID]++
	}
	return len(seen) == 2 && seen[primaryID] == 1 && seen[otherID] == 1
}

func validateState(state *RigidBodyState) (*RigidBodyState, error) {
	if state == nil {
		return nil, fmt.Errorf("missing body state: %w", ErrInvalidPlan)
	}
	position, err := validateVector(state.Position, maxPositionComponent, false)
	if err != nil {
		return nil, err
	}
	rotation, err := validateQuaternion(state.Rotation)
	if err != nil {
		return nil, err
	}
	linear, err := validateVector(state.LinearVelocity, maxLinearSpeed, true)
	if err != nil {
		return nil, err
	}
	angular, err := validateVector(state.AngularVelocity, maxAngularSpeed, true)
	if err != nil {
		return nil, err
	}
	return &RigidBodyState{
		Position:        position,
		Rotation:        rotation,
		LinearVelocity:  linear,
		AngularVelocity: angular,
	}, nil
}

func validateVector(v *Vector3, limit float64, magnitude bool) (*Vector3, error) {
	if v == nil {
		return nil, fmt.Errorf("missing vector: %w", ErrInvalidPlan)
	}
	if !isFinite(v.X) || !isFinite(v.Y) || !isFinite(v.Z) {
		return nil, fmt.Errorf("non-finite vector: %w", ErrInvalidPlan)
	}
	if magnitude {
		if vectorMagnitude(*v) > limit {
			return nil, fmt.Errorf("vector magnitude out of bounds: %w", ErrInvalidPlan)
		}
		return &Vector3{X: v.X, Y: v.Y, Z: v.Z}, nil
	}
	if math.Abs(v.X) > limit || math.Abs(v.Y) > limit || math.Abs(v.Z) > limit {
		return nil, fmt.Errorf("vector component out of bounds: %w", ErrInvalidPlan)
	}
	return &Vector3{X: v.X, Y: v.Y, Z: v.Z}, nil
}

func validateQuaternion(q *Quaternion) (*Quaternion, error) {
	if q == nil {
		return nil, fmt.Errorf("missing quaternion: %w", ErrInvalidPlan)
	}
	if !isFinite(q.X) || !isFinite(q.Y) || !isFinite(q.Z) || !isFinite(q.W) {
		return nil, fmt.Errorf("non-finite quaternion: %w", ErrInvalidPlan)
	}
	norm := math.Sqrt(q.X*q.X + q.Y*q.Y + q.Z*q.Z + q.W*q.W)
	if !isFinite(norm) || math.Abs(norm-1) > maxQuaternionNormError {
		return nil, fmt.Errorf("quaternion not normalized: %w", ErrInvalidPlan)
	}
	return &Quaternion{X: q.X, Y: q.Y, Z: q.Z, W: q.W}, nil
}

func validateColliderID(kind StaticContactKind, id string) error {
	if id == "" || len(id) > maxColliderIDLength {
		return fmt.Errorf("invalid collider id length: %w", ErrInvalidPlan)
	}
	for i := 0; i < len(id); i++ {
		if id[i] < 33 || id[i] > 126 {
			return fmt.Errorf("invalid collider id characters: %w", ErrInvalidPlan)
		}
	}
	prefix := "wall:"
	if kind == StaticContactKindDoor {
		prefix = "door:"
	}
	if !strings.HasPrefix(id, prefix) {
		return fmt.Errorf("invalid collider id prefix: %w", ErrInvalidPlan)
	}
	return nil
}

func vectorMagnitude(v Vector3) float64 {
	return math.Sqrt(v.X*v.X + v.Y*v.Y + v.Z*v.Z)
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func marshalDeterministicDraft(draft Draft) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(draft); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}), nil
}
