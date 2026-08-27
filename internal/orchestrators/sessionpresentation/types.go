// Package sessionpresentation defines the proto-free shared dice presentation domain.
package sessionpresentation

import "errors"

const (
	maxBodies               = 20
	maxContacts             = 128
	maxCheckpointStates     = 256
	maxAttempt              = 32
	maxStep                 = 480
	fingerprintLength       = 32
	maxEncodedPayloadBytes  = 64 * 1024
	maxPositionComponent    = 4096
	maxLinearSpeed          = 64
	maxAngularSpeed         = 128
	maxQuaternionNormError  = 0.0001
	maxPresentationIDLength = 128
	maxColliderIDLength     = 256
)

var (
	ErrInvalidPlan = errors.New("session presentation: invalid plan")
	ErrConflict    = errors.New("session presentation: conflict")
	ErrClosed      = errors.New("session presentation: closed")
)

type PhysicsSchema uint32

const (
	PhysicsSchemaUnspecified         PhysicsSchema = 0
	PhysicsSchemaRapierDungeonD20V1 PhysicsSchema = 1
)

type Shape uint32

const (
	ShapeUnspecified Shape = 0
	ShapeD4          Shape = 1
	ShapeD6          Shape = 2
	ShapeD8          Shape = 3
	ShapeD10         Shape = 4
	ShapeD12         Shape = 5
	ShapeD20         Shape = 6
)

type StaticContactKind uint32

const (
	StaticContactKindUnspecified StaticContactKind = 0
	StaticContactKindWall        StaticContactKind = 1
	StaticContactKindDoor        StaticContactKind = 2
)

type TerminalKind uint32

const (
	TerminalKindUnspecified TerminalKind = 0
	TerminalKindSettled     TerminalKind = 1
	TerminalKindOffTable    TerminalKind = 2
)

type Vector3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type Quaternion struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
	W float64 `json:"w"`
}

type RigidBodyState struct {
	Position        *Vector3    `json:"position"`
	Rotation        *Quaternion `json:"rotation"`
	LinearVelocity  *Vector3    `json:"linear_velocity"`
	AngularVelocity *Vector3    `json:"angular_velocity"`
}

type BodyInitial struct {
	DieID string          `json:"die_id"`
	Shape Shape           `json:"shape"`
	State *RigidBodyState `json:"state"`
}

type StaticColliderContact struct {
	Kind       StaticContactKind `json:"kind"`
	ColliderID string            `json:"collider_id"`
}

type BodyCheckpoint struct {
	DieID string          `json:"die_id"`
	State *RigidBodyState `json:"state"`
}

type ContactCheckpoint struct {
	Step           uint32                 `json:"step"`
	PrimaryDieID   string                 `json:"primary_die_id"`
	StaticCollider *StaticColliderContact `json:"static_collider,omitempty"`
	OtherDieID     string                 `json:"other_die_id,omitempty"`
	After          []BodyCheckpoint       `json:"after"`
}

type BodyTerminal struct {
	DieID string          `json:"die_id"`
	Step  uint32          `json:"step"`
	Kind  TerminalKind    `json:"kind"`
	State *RigidBodyState `json:"state"`
}

type Draft struct {
	SchemaVersion       uint32              `json:"schema_version"`
	PresentationID      string              `json:"presentation_id"`
	AuthoritySeq        uint64              `json:"authority_seq"`
	Attempt             uint32              `json:"attempt"`
	PhysicsSchema       PhysicsSchema       `json:"physics_schema"`
	ColliderFingerprint []byte              `json:"collider_fingerprint"`
	Bodies              []BodyInitial       `json:"bodies"`
	Contacts            []ContactCheckpoint `json:"contacts"`
	Terminal            []BodyTerminal      `json:"terminal"`
}

type Plan struct {
	SchemaVersion       uint32              `json:"schema_version"`
	Session             string              `json:"session"`
	PresentationID      string              `json:"presentation_id"`
	AuthoritySeq        uint64              `json:"authority_seq"`
	Roller              string              `json:"roller"`
	Attempt             uint32              `json:"attempt"`
	PhysicsSchema       PhysicsSchema       `json:"physics_schema"`
	ColliderFingerprint []byte              `json:"collider_fingerprint"`
	Bodies              []BodyInitial       `json:"bodies"`
	Contacts            []ContactCheckpoint `json:"contacts"`
	Terminal            []BodyTerminal      `json:"terminal"`
}
