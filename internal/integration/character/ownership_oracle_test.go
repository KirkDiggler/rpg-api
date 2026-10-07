package characterintegration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	characterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
)

// TestOwnershipGates_AForeignUnprojectableSheetAnswersLikeAMissingOne pins
// rpg-api#815 against rpg-project#538's fold. The ownership gates read the
// stored record unfolded, so a foreign sheet the resolution door would refuse
// answers exactly as an id that names nothing: NOT_FOUND, the same canonical
// sentence. Were the gate to fold first, the foreign sheet would answer
// INTERNAL and reveal that it exists.
func TestOwnershipGates_AForeignUnprojectableSheetAnswersLikeAMissingOne(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	release := sharedRedis.Lease()
	defer release()

	server, err := harness.NewWithRedis(ctx, nil, sharedRedis.Addr)
	require.NoError(t, err)
	defer server.Close()
	require.NoError(t, server.FlushRedis(ctx))

	// An Inspired die with no granting bard: it parses, and the resolution
	// door refuses to attach it.
	const foreignID = "char-foreign-unprojectable"
	inspired, err := (&conditions.InspiredCondition{MemberID: foreignID}).ToJSON()
	require.NoError(t, err)
	_, err = server.CharacterRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{
		Data: &tkcharacter.Data{
			ID: foreignID, PlayerID: "oracle-foreign", Name: "Foreign", Level: 1,
			ClassID: classes.Fighter, RaceID: races.Human,
			EquipmentSlots: tkcharacter.EquipmentSlots{},
			Conditions:     []json.RawMessage{inspired},
		},
	}})
	require.NoError(t, err)

	callerCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Dev oracle-caller")
	const missingID = "char-never-created"

	for _, rpc := range []struct {
		name string
		call func(id string) error
	}{
		{"GetNextLevel", func(id string) error {
			_, callErr := server.CharacterClient.GetNextLevel(callerCtx, &dnd5ev1alpha1.GetNextLevelRequest{CharacterId: id})
			return callErr
		}},
		{"GetCharacterData", func(id string) error {
			_, callErr := server.CharacterClientV2.GetCharacterData(callerCtx, &characterv2pb.GetCharacterDataRequest{CharacterId: id})
			return callErr
		}},
	} {
		t.Run(rpc.name, func(t *testing.T) {
			missingErr := rpc.call(missingID)
			foreignErr := rpc.call(foreignID)

			require.Equal(t, codes.NotFound, status.Code(missingErr), "%v", missingErr)
			require.Equal(t, codes.NotFound, status.Code(foreignErr), "%v", foreignErr)
			require.Equal(t, `character "`+missingID+`" not found`, status.Convert(missingErr).Message())
			require.Equal(t, `character "`+foreignID+`" not found`, status.Convert(foreignErr).Message(),
				"the same sentence, naming only the id the caller sent")
		})
	}
}
