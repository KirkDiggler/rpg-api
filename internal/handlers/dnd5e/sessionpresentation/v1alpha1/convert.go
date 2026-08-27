// Package sessionpresentationv1alpha1 implements the session presentation gRPC handler.
package sessionpresentationv1alpha1

import (
	presentationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/v1alpha1"
	orchsessionpresentation "github.com/KirkDiggler/rpg-api/internal/orchestrators/sessionpresentation"
)

func draftFromProto(in *presentationpb.DiceThrowDraft) orchsessionpresentation.Draft {
	if in == nil {
		return orchsessionpresentation.Draft{}
	}
	return orchsessionpresentation.Draft{
		SchemaVersion:       in.GetSchemaVersion(),
		PresentationID:      in.GetPresentationId(),
		AuthoritySeq:        in.GetAuthoritySeq(),
		Attempt:             in.GetAttempt(),
		PhysicsSchema:       physicsSchemaFromProto(in.GetPhysicsSchema()),
		ColliderFingerprint: append([]byte(nil), in.GetColliderFingerprint()...),
		Bodies:              bodyInitialsFromProto(in.GetBodies()),
		Contacts:            contactCheckpointsFromProto(in.GetContacts()),
		Terminal:            bodyTerminalsFromProto(in.GetTerminal()),
	}
}

func planToProto(in orchsessionpresentation.Plan) *presentationpb.DiceThrowPlan {
	return &presentationpb.DiceThrowPlan{
		SchemaVersion:       in.SchemaVersion,
		Session:             in.Session,
		PresentationId:      in.PresentationID,
		AuthoritySeq:        in.AuthoritySeq,
		Roller:              in.Roller,
		Attempt:             in.Attempt,
		PhysicsSchema:       physicsSchemaToProto(in.PhysicsSchema),
		ColliderFingerprint: append([]byte(nil), in.ColliderFingerprint...),
		Bodies:              bodyInitialsToProto(in.Bodies),
		Contacts:            contactCheckpointsToProto(in.Contacts),
		Terminal:            throwTerminalToProto(in.Terminal),
	}
}

func physicsSchemaFromProto(in presentationpb.DicePhysicsSchema) orchsessionpresentation.PhysicsSchema {
	return orchsessionpresentation.PhysicsSchema(in)
}

func physicsSchemaToProto(in orchsessionpresentation.PhysicsSchema) presentationpb.DicePhysicsSchema {
	return presentationpb.DicePhysicsSchema(in)
}

func shapeFromProto(in presentationpb.DiceShape) orchsessionpresentation.Shape {
	return orchsessionpresentation.Shape(in)
}

func shapeToProto(in orchsessionpresentation.Shape) presentationpb.DiceShape {
	return presentationpb.DiceShape(in)
}

func staticContactKindFromProto(in presentationpb.DiceStaticContactKind) orchsessionpresentation.StaticContactKind {
	return orchsessionpresentation.StaticContactKind(in)
}

func staticContactKindToProto(in orchsessionpresentation.StaticContactKind) presentationpb.DiceStaticContactKind {
	return presentationpb.DiceStaticContactKind(in)
}

func terminalKindFromProto(in presentationpb.DiceTerminalKind) orchsessionpresentation.TerminalKind {
	return orchsessionpresentation.TerminalKind(in)
}

func terminalKindToProto(in orchsessionpresentation.TerminalKind) presentationpb.DiceTerminalKind {
	return presentationpb.DiceTerminalKind(in)
}

func vector3FromProto(in *presentationpb.Vector3) *orchsessionpresentation.Vector3 {
	if in == nil {
		return nil
	}
	return &orchsessionpresentation.Vector3{X: float64(in.GetX()), Y: float64(in.GetY()), Z: float64(in.GetZ())}
}

func vector3ToProto(in *orchsessionpresentation.Vector3) *presentationpb.Vector3 {
	if in == nil {
		return nil
	}
	return &presentationpb.Vector3{X: float32(in.X), Y: float32(in.Y), Z: float32(in.Z)}
}

func quaternionFromProto(in *presentationpb.Quaternion) *orchsessionpresentation.Quaternion {
	if in == nil {
		return nil
	}
	return &orchsessionpresentation.Quaternion{X: float64(in.GetX()), Y: float64(in.GetY()), Z: float64(in.GetZ()), W: float64(in.GetW())}
}

func quaternionToProto(in *orchsessionpresentation.Quaternion) *presentationpb.Quaternion {
	if in == nil {
		return nil
	}
	return &presentationpb.Quaternion{X: float32(in.X), Y: float32(in.Y), Z: float32(in.Z), W: float32(in.W)}
}

func rigidBodyStateFromProto(in *presentationpb.RigidBodyState) *orchsessionpresentation.RigidBodyState {
	if in == nil {
		return nil
	}
	return &orchsessionpresentation.RigidBodyState{
		Position:        vector3FromProto(in.GetPosition()),
		Rotation:        quaternionFromProto(in.GetRotation()),
		LinearVelocity:  vector3FromProto(in.GetLinearVelocity()),
		AngularVelocity: vector3FromProto(in.GetAngularVelocity()),
	}
}

func rigidBodyStateToProto(in *orchsessionpresentation.RigidBodyState) *presentationpb.RigidBodyState {
	if in == nil {
		return nil
	}
	return &presentationpb.RigidBodyState{
		Position:        vector3ToProto(in.Position),
		Rotation:        quaternionToProto(in.Rotation),
		LinearVelocity:  vector3ToProto(in.LinearVelocity),
		AngularVelocity: vector3ToProto(in.AngularVelocity),
	}
}

func bodyInitialsFromProto(in []*presentationpb.DiceBodyInitial) []orchsessionpresentation.BodyInitial {
	if in == nil {
		return nil
	}
	out := make([]orchsessionpresentation.BodyInitial, len(in))
	for i, body := range in {
		out[i] = orchsessionpresentation.BodyInitial{
			DieID: body.GetDieId(),
			Shape: shapeFromProto(body.GetShape()),
			State: rigidBodyStateFromProto(body.GetState()),
		}
	}
	return out
}

func bodyInitialsToProto(in []orchsessionpresentation.BodyInitial) []*presentationpb.DiceBodyInitial {
	if in == nil {
		return nil
	}
	out := make([]*presentationpb.DiceBodyInitial, len(in))
	for i, body := range in {
		out[i] = &presentationpb.DiceBodyInitial{DieId: body.DieID, Shape: shapeToProto(body.Shape), State: rigidBodyStateToProto(body.State)}
	}
	return out
}

func staticColliderContactFromProto(in *presentationpb.StaticColliderContact) *orchsessionpresentation.StaticColliderContact {
	if in == nil {
		return nil
	}
	return &orchsessionpresentation.StaticColliderContact{Kind: staticContactKindFromProto(in.GetKind()), ColliderID: in.GetColliderId()}
}

func staticColliderContactToProto(in *orchsessionpresentation.StaticColliderContact) *presentationpb.StaticColliderContact {
	if in == nil {
		return nil
	}
	return &presentationpb.StaticColliderContact{Kind: staticContactKindToProto(in.Kind), ColliderId: in.ColliderID}
}

func bodyCheckpointsFromProto(in []*presentationpb.DiceBodyCheckpoint) []orchsessionpresentation.BodyCheckpoint {
	if in == nil {
		return nil
	}
	out := make([]orchsessionpresentation.BodyCheckpoint, len(in))
	for i, checkpoint := range in {
		out[i] = orchsessionpresentation.BodyCheckpoint{DieID: checkpoint.GetDieId(), State: rigidBodyStateFromProto(checkpoint.GetState())}
	}
	return out
}

func bodyCheckpointsToProto(in []orchsessionpresentation.BodyCheckpoint) []*presentationpb.DiceBodyCheckpoint {
	if in == nil {
		return nil
	}
	out := make([]*presentationpb.DiceBodyCheckpoint, len(in))
	for i, checkpoint := range in {
		out[i] = &presentationpb.DiceBodyCheckpoint{DieId: checkpoint.DieID, State: rigidBodyStateToProto(checkpoint.State)}
	}
	return out
}

func contactCheckpointsFromProto(in []*presentationpb.ContactCheckpoint) []orchsessionpresentation.ContactCheckpoint {
	if in == nil {
		return nil
	}
	out := make([]orchsessionpresentation.ContactCheckpoint, len(in))
	for i, contact := range in {
		out[i] = orchsessionpresentation.ContactCheckpoint{
			Step:           contact.GetStep(),
			PrimaryDieID:   contact.GetPrimaryDieId(),
			StaticCollider: staticColliderContactFromProto(contact.GetStaticCollider()),
			OtherDieID:     contact.GetOtherDieId(),
			After:          bodyCheckpointsFromProto(contact.GetAfter()),
		}
	}
	return out
}

func contactCheckpointsToProto(in []orchsessionpresentation.ContactCheckpoint) []*presentationpb.ContactCheckpoint {
	if in == nil {
		return nil
	}
	out := make([]*presentationpb.ContactCheckpoint, len(in))
	for i, contact := range in {
		item := &presentationpb.ContactCheckpoint{
			Step:         contact.Step,
			PrimaryDieId: contact.PrimaryDieID,
			After:        bodyCheckpointsToProto(contact.After),
		}
		if contact.StaticCollider != nil {
			item.Target = &presentationpb.ContactCheckpoint_StaticCollider{StaticCollider: staticColliderContactToProto(contact.StaticCollider)}
		} else if contact.OtherDieID != "" {
			item.Target = &presentationpb.ContactCheckpoint_OtherDieId{OtherDieId: contact.OtherDieID}
		}
		out[i] = item
	}
	return out
}

func bodyTerminalsFromProto(in *presentationpb.ThrowTerminal) []orchsessionpresentation.BodyTerminal {
	if in == nil {
		return nil
	}
	out := make([]orchsessionpresentation.BodyTerminal, len(in.GetDice()))
	for i, terminal := range in.GetDice() {
		out[i] = orchsessionpresentation.BodyTerminal{
			DieID: terminal.GetDieId(),
			Step:  terminal.GetStep(),
			Kind:  terminalKindFromProto(terminal.GetKind()),
			State: rigidBodyStateFromProto(terminal.GetState()),
		}
	}
	return out
}

func throwTerminalToProto(in []orchsessionpresentation.BodyTerminal) *presentationpb.ThrowTerminal {
	if in == nil {
		return nil
	}
	out := make([]*presentationpb.DiceBodyTerminal, len(in))
	for i, terminal := range in {
		out[i] = &presentationpb.DiceBodyTerminal{
			DieId: terminal.DieID,
			Step:  terminal.Step,
			Kind:  terminalKindToProto(terminal.Kind),
			State: rigidBodyStateToProto(terminal.State),
		}
	}
	return &presentationpb.ThrowTerminal{Dice: out}
}
