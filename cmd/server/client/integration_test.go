//go:build integration

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

// D&D API response structures
type DnDAPIClass struct {
	Index                    string                    `json:"index"`
	Name                     string                    `json:"name"`
	HitDie                   int                       `json:"hit_die"`
	ProficiencyChoices       []DnDAPIProficiencyChoice `json:"proficiency_choices"`
	StartingEquipmentOptions []interface{}             `json:"starting_equipment_options"`
}

type DnDAPIProficiencyChoice struct {
	Choose int    `json:"choose"`
	Type   string `json:"type"`
	From   struct {
		Options []interface{} `json:"options"`
	} `json:"from"`
}

func TestFighterChoicesIntegration(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Connect to our gRPC server
	grpcServerAddress := os.Getenv("GRPC_SERVER_ADDRESS")
	if grpcServerAddress == "" {
		grpcServerAddress = "localhost:50051"
	}
	conn, err := grpc.NewClient(grpcServerAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("Failed to close connection: %v", err)
		}
	}()

	client := dnd5ev1alpha1.NewCharacterServiceClient(conn)

	// Get fighter data from our API
	resp, err := client.GetClassDetails(context.Background(), &dnd5ev1alpha1.GetClassDetailsRequest{
		ClassId: "fighter",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Class)

	// Get fighter data from D&D API
	baseURL := os.Getenv("DND_API_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:3002"
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		fmt.Sprintf("%s/api/2014/classes/fighter", baseURL), nil)
	require.NoError(t, err)
	dndResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() {
		if err := dndResp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	var dndClass DnDAPIClass
	err = json.NewDecoder(dndResp.Body).Decode(&dndClass)
	require.NoError(t, err)

	// Verify basic class info matches
	assert.Equal(t, "fighter", resp.Class.Id)
	assert.Equal(t, "Fighter", resp.Class.Name)
	assert.Equal(t, fmt.Sprintf("1d%d", dndClass.HitDie), resp.Class.HitDie)

	// Verify we have the expected number of choices
	// Fighter should have:
	// - 4 equipment choices
	// - 1 skill proficiency choice
	// - 1 fighting style feature choice
	assert.Len(t, resp.Class.Choices, 6, "Fighter should have 6 total choices")

	// Count choice types
	choiceTypeCounts := make(map[string]int)
	for _, choice := range resp.Class.Choices {
		choiceTypeCounts[choice.ChoiceType.String()]++
	}

	assert.Equal(t, 4, choiceTypeCounts["CHOICE_TYPE_EQUIPMENT"], "Should have 4 equipment choices")
	assert.Equal(t, 1, choiceTypeCounts["CHOICE_TYPE_SKILL"], "Should have 1 skill choice")
	assert.Equal(t, 1, choiceTypeCounts["CHOICE_TYPE_FEAT"], "Should have 1 feature/feat choice")

	// Verify Fighting Style choice
	var fightingStyleChoice *dnd5ev1alpha1.Choice
	for _, choice := range resp.Class.Choices {
		if choice.Description == "Fighting Style: Choose 1 feature" {
			fightingStyleChoice = choice
			break
		}
	}
	require.NotNil(t, fightingStyleChoice, "Should have Fighting Style choice")
	assert.Equal(t, dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_FEAT, fightingStyleChoice.ChoiceType)
	assert.Equal(t, int32(1), fightingStyleChoice.ChooseCount)

	// Verify Fighting Style has options
	explicitOpts, ok := fightingStyleChoice.OptionSet.(*dnd5ev1alpha1.Choice_ExplicitOptions)
	require.True(t, ok, "Fighting Style should have explicit options")
	assert.Len(t, explicitOpts.ExplicitOptions.Options, 6, "Fighting Style should have 6 options")

	// Verify martial weapon choice
	var martialWeaponChoice *dnd5ev1alpha1.Choice
	for _, choice := range resp.Class.Choices {
		if choice.Description == "(a) a martial weapon and a shield or (b) two martial weapons" {
			martialWeaponChoice = choice
			break
		}
	}
	require.NotNil(t, martialWeaponChoice, "Should have martial weapon choice")

	// Verify it has the bundle option and nested choice option
	martialOpts, ok := martialWeaponChoice.OptionSet.(*dnd5ev1alpha1.Choice_ExplicitOptions)
	require.True(t, ok, "Martial weapon choice should have explicit options")
	assert.Len(t, martialOpts.ExplicitOptions.Options, 2, "Should have 2 options (bundle and nested choice)")
}

func TestClassChoiceTypes(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	testCases := []struct {
		classID         string
		expectedSkill   int
		expectedEquip   int
		expectedFeature int
	}{
		{"fighter", 1, 4, 1},
		{"wizard", 1, 3, 0},
		{"rogue", 1, 3, 0},
	}

	// Connect to our gRPC server
	grpcServerAddress := os.Getenv("GRPC_SERVER_ADDRESS")
	if grpcServerAddress == "" {
		grpcServerAddress = "localhost:50051"
	}
	conn, err := grpc.NewClient(grpcServerAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("Failed to close connection: %v", err)
		}
	}()

	client := dnd5ev1alpha1.NewCharacterServiceClient(conn)

	for _, tc := range testCases {
		t.Run(tc.classID, func(t *testing.T) {
			resp, err := client.GetClassDetails(context.Background(), &dnd5ev1alpha1.GetClassDetailsRequest{
				ClassId: tc.classID,
			})
			require.NoError(t, err)

			// Count choice types
			choiceTypeCounts := make(map[string]int)
			for _, choice := range resp.Class.Choices {
				choiceTypeCounts[choice.ChoiceType.String()]++
			}

			assert.Equal(t, tc.expectedSkill, choiceTypeCounts["CHOICE_TYPE_SKILL"],
				"%s should have %d skill choice(s)", tc.classID, tc.expectedSkill)
			assert.Equal(t, tc.expectedEquip, choiceTypeCounts["CHOICE_TYPE_EQUIPMENT"],
				"%s should have %d equipment choice(s)", tc.classID, tc.expectedEquip)
			assert.Equal(t, tc.expectedFeature, choiceTypeCounts["CHOICE_TYPE_FEAT"],
				"%s should have %d feature choice(s)", tc.classID, tc.expectedFeature)
		})
	}
}
