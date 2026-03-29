// Package entities defines the core data structures for the RPG API
package entities

// This file provides custom JSON marshal/unmarshal implementations for event
// structs that contain proto message fields.
//
// Background: encoding/json does not handle protobuf messages correctly.
// The fields were originally tagged json:"-" to avoid broken output, but that
// caused them to be silently dropped when events traveled through Redis
// pub/sub (serialize → publish → receive → deserialize).
//
// Fix strategy: encode each proto field as a base64 string (standard encoding)
// containing the proto wire-format bytes.  This is:
//   - Lossless: proto wire format preserves every field
//   - Compact: wire format is smaller than JSON
//   - Simple: proto.Marshal / proto.Unmarshal do the work
//
// Each affected event type gets a MarshalJSON / UnmarshalJSON pair.
// The alias trick (type alias = OriginalType) lets us call json.Marshal on
// the non-proto fields without triggering infinite recursion.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/proto"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

// ---- helpers ----------------------------------------------------------------

// marshalProto encodes a proto.Message to a base64 string suitable for JSON.
// Returns an empty string if msg is nil.
func marshalProto(msg proto.Message) (string, error) {
	if msg == nil {
		return "", nil
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("proto.Marshal: %w", err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// unmarshalProto decodes a base64 string back into dst (a proto.Message).
// A blank s is treated as "no data" and dst is left unchanged.
func unmarshalProto(s string, dst proto.Message) error {
	if s == "" {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return fmt.Errorf("base64.Decode: %w", err)
	}
	if err := proto.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("proto.Unmarshal: %w", err)
	}
	return nil
}

// marshalProtoSlice encodes a slice of proto.Message values.
func marshalProtoSlice[T proto.Message](msgs []T) ([]string, error) {
	if len(msgs) == 0 {
		return nil, nil
	}
	out := make([]string, len(msgs))
	for i, m := range msgs {
		s, err := marshalProto(m)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}
		out[i] = s
	}
	return out, nil
}

// unmarshalProtoSlice decodes a slice of base64 strings into proto messages.
func unmarshalProtoSlice[T interface {
	proto.Message
	*E
}, E any](strs []string) ([]T, error) {
	if len(strs) == 0 {
		return nil, nil
	}
	out := make([]T, len(strs))
	for i, s := range strs {
		msg := T(new(E))
		if err := unmarshalProto(s, msg); err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}
		out[i] = msg
	}
	return out, nil
}

// ---- PlayerReconnectedEvent -------------------------------------------------

type playerReconnectedEventJSON struct {
	PlayerID           string `json:"player_id"`
	CharacterID        string `json:"character_id"`
	EncounterStateData string `json:"encounter_state_data,omitempty"`
}

// MarshalJSON implements json.Marshaler for PlayerReconnectedEvent.
func (e PlayerReconnectedEvent) MarshalJSON() ([]byte, error) {
	enc, err := marshalProto(e.EncounterStateData)
	if err != nil {
		return nil, fmt.Errorf("PlayerReconnectedEvent.MarshalJSON: %w", err)
	}
	return json.Marshal(playerReconnectedEventJSON{
		PlayerID:           e.PlayerID,
		CharacterID:        e.CharacterID,
		EncounterStateData: enc,
	})
}

// UnmarshalJSON implements json.Unmarshaler for PlayerReconnectedEvent.
func (e *PlayerReconnectedEvent) UnmarshalJSON(data []byte) error {
	var raw playerReconnectedEventJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("PlayerReconnectedEvent.UnmarshalJSON: %w", err)
	}
	e.PlayerID = raw.PlayerID
	e.CharacterID = raw.CharacterID
	if raw.EncounterStateData != "" {
		e.EncounterStateData = &dnd5ev1alpha1.EncounterStateData{}
		if err := unmarshalProto(raw.EncounterStateData, e.EncounterStateData); err != nil {
			return fmt.Errorf("PlayerReconnectedEvent.UnmarshalJSON EncounterStateData: %w", err)
		}
	}
	return nil
}

// ---- CombatStartedEvent -----------------------------------------------------

// combatStartedEventJSON is the on-wire representation.
// We embed the alias so regular fields serialize automatically; proto fields
// are stored separately as base64 strings.
type combatStartedEventAlias CombatStartedEvent

type combatStartedEventJSON struct {
	combatStartedEventAlias
	EncounterStateData string `json:"encounter_state_data,omitempty"`
}

// MarshalJSON implements json.Marshaler for CombatStartedEvent.
func (e CombatStartedEvent) MarshalJSON() ([]byte, error) {
	enc, err := marshalProto(e.EncounterStateData)
	if err != nil {
		return nil, fmt.Errorf("CombatStartedEvent.MarshalJSON: %w", err)
	}
	return json.Marshal(combatStartedEventJSON{
		combatStartedEventAlias: combatStartedEventAlias(e),
		EncounterStateData:      enc,
	})
}

// UnmarshalJSON implements json.Unmarshaler for CombatStartedEvent.
func (e *CombatStartedEvent) UnmarshalJSON(data []byte) error {
	var raw combatStartedEventJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("CombatStartedEvent.UnmarshalJSON: %w", err)
	}
	*e = CombatStartedEvent(raw.combatStartedEventAlias)
	if raw.EncounterStateData != "" {
		e.EncounterStateData = &dnd5ev1alpha1.EncounterStateData{}
		if err := unmarshalProto(raw.EncounterStateData, e.EncounterStateData); err != nil {
			return fmt.Errorf("CombatStartedEvent.UnmarshalJSON EncounterStateData: %w", err)
		}
	}
	return nil
}

// ---- CombatEndedEvent -------------------------------------------------------

type combatEndedEventAlias CombatEndedEvent

type combatEndedEventJSON struct {
	combatEndedEventAlias
	EncounterStateData string `json:"encounter_state_data,omitempty"`
}

// MarshalJSON implements json.Marshaler for CombatEndedEvent.
func (e CombatEndedEvent) MarshalJSON() ([]byte, error) {
	enc, err := marshalProto(e.EncounterStateData)
	if err != nil {
		return nil, fmt.Errorf("CombatEndedEvent.MarshalJSON: %w", err)
	}
	return json.Marshal(combatEndedEventJSON{
		combatEndedEventAlias: combatEndedEventAlias(e),
		EncounterStateData:    enc,
	})
}

// UnmarshalJSON implements json.Unmarshaler for CombatEndedEvent.
func (e *CombatEndedEvent) UnmarshalJSON(data []byte) error {
	var raw combatEndedEventJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("CombatEndedEvent.UnmarshalJSON: %w", err)
	}
	*e = CombatEndedEvent(raw.combatEndedEventAlias)
	if raw.EncounterStateData != "" {
		e.EncounterStateData = &dnd5ev1alpha1.EncounterStateData{}
		if err := unmarshalProto(raw.EncounterStateData, e.EncounterStateData); err != nil {
			return fmt.Errorf("CombatEndedEvent.UnmarshalJSON EncounterStateData: %w", err)
		}
	}
	return nil
}

// ---- CombatResumedEvent -----------------------------------------------------

type combatResumedEventAlias CombatResumedEvent

type combatResumedEventJSON struct {
	combatResumedEventAlias
	EncounterStateData string `json:"encounter_state_data,omitempty"`
}

// MarshalJSON implements json.Marshaler for CombatResumedEvent.
func (e CombatResumedEvent) MarshalJSON() ([]byte, error) {
	enc, err := marshalProto(e.EncounterStateData)
	if err != nil {
		return nil, fmt.Errorf("CombatResumedEvent.MarshalJSON: %w", err)
	}
	return json.Marshal(combatResumedEventJSON{
		combatResumedEventAlias: combatResumedEventAlias(e),
		EncounterStateData:      enc,
	})
}

// UnmarshalJSON implements json.Unmarshaler for CombatResumedEvent.
func (e *CombatResumedEvent) UnmarshalJSON(data []byte) error {
	var raw combatResumedEventJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("CombatResumedEvent.UnmarshalJSON: %w", err)
	}
	*e = CombatResumedEvent(raw.combatResumedEventAlias)
	if raw.EncounterStateData != "" {
		e.EncounterStateData = &dnd5ev1alpha1.EncounterStateData{}
		if err := unmarshalProto(raw.EncounterStateData, e.EncounterStateData); err != nil {
			return fmt.Errorf("CombatResumedEvent.UnmarshalJSON EncounterStateData: %w", err)
		}
	}
	return nil
}

// ---- MovementCompletedEvent -------------------------------------------------

type movementCompletedEventAlias MovementCompletedEvent

type movementCompletedEventJSON struct {
	movementCompletedEventAlias
	UpdatedEntity    string `json:"updated_entity,omitempty"`
	CombatStateProto string `json:"combat_state_proto,omitempty"`
}

// MarshalJSON implements json.Marshaler for MovementCompletedEvent.
func (e MovementCompletedEvent) MarshalJSON() ([]byte, error) {
	updatedEntity, err := marshalProto(e.UpdatedEntity)
	if err != nil {
		return nil, fmt.Errorf("MovementCompletedEvent.MarshalJSON UpdatedEntity: %w", err)
	}
	combatState, err := marshalProto(e.CombatStateProto)
	if err != nil {
		return nil, fmt.Errorf("MovementCompletedEvent.MarshalJSON CombatStateProto: %w", err)
	}
	return json.Marshal(movementCompletedEventJSON{
		movementCompletedEventAlias: movementCompletedEventAlias(e),
		UpdatedEntity:               updatedEntity,
		CombatStateProto:            combatState,
	})
}

// UnmarshalJSON implements json.Unmarshaler for MovementCompletedEvent.
func (e *MovementCompletedEvent) UnmarshalJSON(data []byte) error {
	var raw movementCompletedEventJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("MovementCompletedEvent.UnmarshalJSON: %w", err)
	}
	*e = MovementCompletedEvent(raw.movementCompletedEventAlias)
	if raw.UpdatedEntity != "" {
		e.UpdatedEntity = &dnd5ev1alpha1.EntityState{}
		if err := unmarshalProto(raw.UpdatedEntity, e.UpdatedEntity); err != nil {
			return fmt.Errorf("MovementCompletedEvent.UnmarshalJSON UpdatedEntity: %w", err)
		}
	}
	if raw.CombatStateProto != "" {
		e.CombatStateProto = &dnd5ev1alpha1.CombatState{}
		if err := unmarshalProto(raw.CombatStateProto, e.CombatStateProto); err != nil {
			return fmt.Errorf("MovementCompletedEvent.UnmarshalJSON CombatStateProto: %w", err)
		}
	}
	return nil
}

// ---- AttackResolvedEvent ----------------------------------------------------

type attackResolvedEventAlias AttackResolvedEvent

type attackResolvedEventJSON struct {
	attackResolvedEventAlias
	AttackerState string `json:"attacker_state,omitempty"`
	TargetState   string `json:"target_state,omitempty"`
}

// MarshalJSON implements json.Marshaler for AttackResolvedEvent.
func (e AttackResolvedEvent) MarshalJSON() ([]byte, error) {
	attacker, err := marshalProto(e.AttackerState)
	if err != nil {
		return nil, fmt.Errorf("AttackResolvedEvent.MarshalJSON AttackerState: %w", err)
	}
	target, err := marshalProto(e.TargetState)
	if err != nil {
		return nil, fmt.Errorf("AttackResolvedEvent.MarshalJSON TargetState: %w", err)
	}
	return json.Marshal(attackResolvedEventJSON{
		attackResolvedEventAlias: attackResolvedEventAlias(e),
		AttackerState:            attacker,
		TargetState:              target,
	})
}

// UnmarshalJSON implements json.Unmarshaler for AttackResolvedEvent.
func (e *AttackResolvedEvent) UnmarshalJSON(data []byte) error {
	var raw attackResolvedEventJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("AttackResolvedEvent.UnmarshalJSON: %w", err)
	}
	*e = AttackResolvedEvent(raw.attackResolvedEventAlias)
	if raw.AttackerState != "" {
		e.AttackerState = &dnd5ev1alpha1.EntityState{}
		if err := unmarshalProto(raw.AttackerState, e.AttackerState); err != nil {
			return fmt.Errorf("AttackResolvedEvent.UnmarshalJSON AttackerState: %w", err)
		}
	}
	if raw.TargetState != "" {
		e.TargetState = &dnd5ev1alpha1.EntityState{}
		if err := unmarshalProto(raw.TargetState, e.TargetState); err != nil {
			return fmt.Errorf("AttackResolvedEvent.UnmarshalJSON TargetState: %w", err)
		}
	}
	return nil
}

// ---- TurnEndedEvent ---------------------------------------------------------

type turnEndedEventAlias TurnEndedEvent

type turnEndedEventJSON struct {
	turnEndedEventAlias
	UpdatedEntities  []string `json:"updated_entities,omitempty"`
	CombatStateProto string   `json:"combat_state_proto,omitempty"`
}

// MarshalJSON implements json.Marshaler for TurnEndedEvent.
func (e TurnEndedEvent) MarshalJSON() ([]byte, error) {
	updatedEntities, err := marshalProtoSlice(e.UpdatedEntities)
	if err != nil {
		return nil, fmt.Errorf("TurnEndedEvent.MarshalJSON UpdatedEntities: %w", err)
	}
	combatState, err := marshalProto(e.CombatStateProto)
	if err != nil {
		return nil, fmt.Errorf("TurnEndedEvent.MarshalJSON CombatStateProto: %w", err)
	}
	return json.Marshal(turnEndedEventJSON{
		turnEndedEventAlias: turnEndedEventAlias(e),
		UpdatedEntities:     updatedEntities,
		CombatStateProto:    combatState,
	})
}

// UnmarshalJSON implements json.Unmarshaler for TurnEndedEvent.
func (e *TurnEndedEvent) UnmarshalJSON(data []byte) error {
	var raw turnEndedEventJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("TurnEndedEvent.UnmarshalJSON: %w", err)
	}
	*e = TurnEndedEvent(raw.turnEndedEventAlias)
	if len(raw.UpdatedEntities) > 0 {
		entities, err := unmarshalProtoSlice[*dnd5ev1alpha1.EntityState, dnd5ev1alpha1.EntityState](raw.UpdatedEntities)
		if err != nil {
			return fmt.Errorf("TurnEndedEvent.UnmarshalJSON UpdatedEntities: %w", err)
		}
		e.UpdatedEntities = entities
	}
	if raw.CombatStateProto != "" {
		e.CombatStateProto = &dnd5ev1alpha1.CombatState{}
		if err := unmarshalProto(raw.CombatStateProto, e.CombatStateProto); err != nil {
			return fmt.Errorf("TurnEndedEvent.UnmarshalJSON CombatStateProto: %w", err)
		}
	}
	return nil
}

// ---- MonsterTurnCompletedEvent ----------------------------------------------

type monsterTurnCompletedEventAlias MonsterTurnCompletedEvent

type monsterTurnCompletedEventJSON struct {
	monsterTurnCompletedEventAlias
	UpdatedEntities  []string `json:"updated_entities,omitempty"`
	CombatStateProto string   `json:"combat_state_proto,omitempty"`
}

// MarshalJSON implements json.Marshaler for MonsterTurnCompletedEvent.
func (e MonsterTurnCompletedEvent) MarshalJSON() ([]byte, error) {
	updatedEntities, err := marshalProtoSlice(e.UpdatedEntities)
	if err != nil {
		return nil, fmt.Errorf("MonsterTurnCompletedEvent.MarshalJSON UpdatedEntities: %w", err)
	}
	combatState, err := marshalProto(e.CombatStateProto)
	if err != nil {
		return nil, fmt.Errorf("MonsterTurnCompletedEvent.MarshalJSON CombatStateProto: %w", err)
	}
	return json.Marshal(monsterTurnCompletedEventJSON{
		monsterTurnCompletedEventAlias: monsterTurnCompletedEventAlias(e),
		UpdatedEntities:                updatedEntities,
		CombatStateProto:               combatState,
	})
}

// UnmarshalJSON implements json.Unmarshaler for MonsterTurnCompletedEvent.
func (e *MonsterTurnCompletedEvent) UnmarshalJSON(data []byte) error {
	var raw monsterTurnCompletedEventJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("MonsterTurnCompletedEvent.UnmarshalJSON: %w", err)
	}
	*e = MonsterTurnCompletedEvent(raw.monsterTurnCompletedEventAlias)
	if len(raw.UpdatedEntities) > 0 {
		entities, err := unmarshalProtoSlice[*dnd5ev1alpha1.EntityState, dnd5ev1alpha1.EntityState](raw.UpdatedEntities)
		if err != nil {
			return fmt.Errorf("MonsterTurnCompletedEvent.UnmarshalJSON UpdatedEntities: %w", err)
		}
		e.UpdatedEntities = entities
	}
	if raw.CombatStateProto != "" {
		e.CombatStateProto = &dnd5ev1alpha1.CombatState{}
		if err := unmarshalProto(raw.CombatStateProto, e.CombatStateProto); err != nil {
			return fmt.Errorf("MonsterTurnCompletedEvent.UnmarshalJSON CombatStateProto: %w", err)
		}
	}
	return nil
}

// ---- DungeonVictoryEvent ----------------------------------------------------

type dungeonVictoryEventAlias DungeonVictoryEvent

type dungeonVictoryEventJSON struct {
	dungeonVictoryEventAlias
	EncounterStateData string `json:"encounter_state_data,omitempty"`
}

// MarshalJSON implements json.Marshaler for DungeonVictoryEvent.
func (e DungeonVictoryEvent) MarshalJSON() ([]byte, error) {
	enc, err := marshalProto(e.EncounterStateData)
	if err != nil {
		return nil, fmt.Errorf("DungeonVictoryEvent.MarshalJSON: %w", err)
	}
	return json.Marshal(dungeonVictoryEventJSON{
		dungeonVictoryEventAlias: dungeonVictoryEventAlias(e),
		EncounterStateData:       enc,
	})
}

// UnmarshalJSON implements json.Unmarshaler for DungeonVictoryEvent.
func (e *DungeonVictoryEvent) UnmarshalJSON(data []byte) error {
	var raw dungeonVictoryEventJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("DungeonVictoryEvent.UnmarshalJSON: %w", err)
	}
	*e = DungeonVictoryEvent(raw.dungeonVictoryEventAlias)
	if raw.EncounterStateData != "" {
		e.EncounterStateData = &dnd5ev1alpha1.EncounterStateData{}
		if err := unmarshalProto(raw.EncounterStateData, e.EncounterStateData); err != nil {
			return fmt.Errorf("DungeonVictoryEvent.UnmarshalJSON EncounterStateData: %w", err)
		}
	}
	return nil
}

// ---- DungeonFailureEvent ----------------------------------------------------

type dungeonFailureEventAlias DungeonFailureEvent

type dungeonFailureEventJSON struct {
	dungeonFailureEventAlias
	EncounterStateData string `json:"encounter_state_data,omitempty"`
}

// MarshalJSON implements json.Marshaler for DungeonFailureEvent.
func (e DungeonFailureEvent) MarshalJSON() ([]byte, error) {
	enc, err := marshalProto(e.EncounterStateData)
	if err != nil {
		return nil, fmt.Errorf("DungeonFailureEvent.MarshalJSON: %w", err)
	}
	return json.Marshal(dungeonFailureEventJSON{
		dungeonFailureEventAlias: dungeonFailureEventAlias(e),
		EncounterStateData:       enc,
	})
}

// UnmarshalJSON implements json.Unmarshaler for DungeonFailureEvent.
func (e *DungeonFailureEvent) UnmarshalJSON(data []byte) error {
	var raw dungeonFailureEventJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("DungeonFailureEvent.UnmarshalJSON: %w", err)
	}
	*e = DungeonFailureEvent(raw.dungeonFailureEventAlias)
	if raw.EncounterStateData != "" {
		e.EncounterStateData = &dnd5ev1alpha1.EncounterStateData{}
		if err := unmarshalProto(raw.EncounterStateData, e.EncounterStateData); err != nil {
			return fmt.Errorf("DungeonFailureEvent.UnmarshalJSON EncounterStateData: %w", err)
		}
	}
	return nil
}

// ---- RoomRevealedEvent ------------------------------------------------------

type roomRevealedEventAlias RoomRevealedEvent

type roomRevealedEventJSON struct {
	roomRevealedEventAlias
	EncounterStateData string `json:"encounter_state_data,omitempty"`
}

// MarshalJSON implements json.Marshaler for RoomRevealedEvent.
func (e RoomRevealedEvent) MarshalJSON() ([]byte, error) {
	enc, err := marshalProto(e.EncounterStateData)
	if err != nil {
		return nil, fmt.Errorf("RoomRevealedEvent.MarshalJSON: %w", err)
	}
	return json.Marshal(roomRevealedEventJSON{
		roomRevealedEventAlias: roomRevealedEventAlias(e),
		EncounterStateData:     enc,
	})
}

// UnmarshalJSON implements json.Unmarshaler for RoomRevealedEvent.
func (e *RoomRevealedEvent) UnmarshalJSON(data []byte) error {
	var raw roomRevealedEventJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("RoomRevealedEvent.UnmarshalJSON: %w", err)
	}
	*e = RoomRevealedEvent(raw.roomRevealedEventAlias)
	if raw.EncounterStateData != "" {
		e.EncounterStateData = &dnd5ev1alpha1.EncounterStateData{}
		if err := unmarshalProto(raw.EncounterStateData, e.EncounterStateData); err != nil {
			return fmt.Errorf("RoomRevealedEvent.UnmarshalJSON EncounterStateData: %w", err)
		}
	}
	return nil
}
