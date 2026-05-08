// Package encounter is the v2 encounter handler.
// project.go translates a toolkit encounter.Data into the v1alpha2
// proto Encounter message for a specific viewer.
package encounter

import (
	"sort"
	"time"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// ProjectFor builds a proto *encounterv2pb.Encounter for the given viewer from
// the encounter's persisted data. It is called by CreateEncounter,
// GetEncounter, and StreamEncounter (SnapshotDelivered.encounter) so the
// projection logic lives in exactly one place.
//
// The broker is required to rehydrate a live Encounter via LoadFromData
// (needed for SnapshotFor).
//
// now is passed explicitly so callers in tests can inject a fixed clock.
func ProjectFor(
	data *tkenc.Data,
	viewer core.PlayerID,
	broker *tkenc.Broker,
	now time.Time,
) (*encounterv2pb.Encounter, error) {
	enc, err := tkenc.LoadFromData(data, broker)
	if err != nil {
		return nil, err
	}

	snap := enc.SnapshotFor(viewer)

	// Build the Space from the viewer's revealed hexes. Hexes are sorted by
	// (Q,R,S) so the wire output is deterministic across Go map iterations.
	keys := make([]core.Hex, 0, len(snap.RevealedHexes))
	for h := range snap.RevealedHexes {
		keys = append(keys, h)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Q != keys[j].Q {
			return keys[i].Q < keys[j].Q
		}
		if keys[i].R != keys[j].R {
			return keys[i].R < keys[j].R
		}
		return keys[i].S < keys[j].S
	})
	hexes := make([]*encounterv2pb.Hex, 0, len(keys))
	for _, h := range keys {
		hexes = append(hexes, &encounterv2pb.Hex{Position: HexToPosition(h)})
	}

	// The viewer's own entity is visible at their position.
	var entities []*encounterv2pb.Entity
	if snap.PlayerID != "" {
		entities = append(entities, &encounterv2pb.Entity{
			Id:       string(snap.PlayerID),
			Position: HexToPosition(snap.Position),
		})
	}

	return &encounterv2pb.Encounter{
		Id:   string(data.ID),
		Mode: encounterv2pb.EncounterMode_ENCOUNTER_MODE_FREE_ROAM,
		Space: &encounterv2pb.Space{
			Hexes:    hexes,
			Entities: entities,
		},
	}, nil
}
