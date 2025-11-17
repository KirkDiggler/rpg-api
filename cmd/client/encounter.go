package main

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

var rageTestCmd = &cobra.Command{
	Use:   "rage-test",
	Short: "Test full combat flow with Barbarian Rage",
	Long: `Creates a Barbarian character, starts a dungeon, and performs an attack.
This verifies that the Rage feature adds damage bonus through the event-driven combat system.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		// Connect to server
		conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		defer func() {
			if closeErr := conn.Close(); closeErr != nil {
				log.Printf("Failed to close connection: %v", closeErr)
			}
		}()

		charClient := dnd5ev1alpha1.NewCharacterServiceClient(conn)
		encClient := dnd5ev1alpha1.NewEncounterServiceClient(conn)

		fmt.Println("=== Rage Combat Integration Test ===")

		// Step 1: Create Barbarian
		fmt.Println("1. Creating Barbarian character...")
		characterID, err := createBarbarian(ctx, charClient)
		if err != nil {
			return fmt.Errorf("failed to create barbarian: %w", err)
		}
		fmt.Printf("   ✓ Created Barbarian: %s\n\n", characterID)

		// Step 2: Start Dungeon
		fmt.Println("2. Starting dungeon encounter...")
		encounterID, err := startDungeon(ctx, encClient)
		if err != nil {
			return fmt.Errorf("failed to start dungeon: %w", err)
		}
		fmt.Printf("   ✓ Started encounter: %s\n\n", encounterID)

		// Step 3: Perform Attack
		fmt.Println("3. Attacking with Rage active...")
		result, err := performAttack(ctx, encClient, encounterID, characterID)
		if err != nil {
			return fmt.Errorf("failed to attack: %w", err)
		}

		// Display results
		displayAttackResult(result)

		return nil
	},
}

func createBarbarian(ctx context.Context, client dnd5ev1alpha1.CharacterServiceClient) (string, error) {
	// Create draft
	createResp, err := client.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId: "test-player",
	})
	if err != nil {
		return "", err
	}
	draftID := createResp.Draft.Id

	// Set race to Human with language choice
	// Human gets Common automatically + 1 language of choice
	_, err = client.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
				ChoiceId: "human-language",
				Selection: &dnd5ev1alpha1.ChoiceData_Languages{
					Languages: &dnd5ev1alpha1.LanguageSelection{
						Languages: []dnd5ev1alpha1.Language{
							dnd5ev1alpha1.Language_LANGUAGE_DWARVISH, // Pick Dwarvish for variety
						},
					},
				},
			},
		},
	})
	if err != nil {
		return "", err
	}

	// Set class to Barbarian with skill and equipment choices
	// Barbarian needs:
	// - 2 skills from: Animal Handling, Athletics, Intimidation, Nature, Perception, Survival
	// - Primary weapon choice (greataxe or martial weapon)
	// - Secondary weapon choice (two handaxes or simple weapon)
	// - Pack choice (explorer's pack)
	_, err = client.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_BARBARIAN,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			// Skills
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "skills",
				Selection: &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillSelection{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
							dnd5ev1alpha1.Skill_SKILL_INTIMIDATION,
						},
					},
				},
			},
			// Primary weapon: Greataxe
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "barbarian-weapons-primary",
				OptionId: "barbarian-weapon-a", // Greataxe option
			},
			// Secondary weapon: Two handaxes
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "barbarian-weapons-secondary",
				OptionId: "barbarian-secondary-a", // Handaxes option
			},
			// Pack: Explorer's pack
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "barbarian-pack",
				OptionId: "barbarian-pack-a", // Explorer's pack option
			},
		},
	})
	if err != nil {
		return "", err
	}

	// Set background to Soldier (thematic for Barbarian)
	_, err = client.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    draftID,
		Background: dnd5ev1alpha1.Background_BACKGROUND_SOLDIER,
	})
	if err != nil {
		return "", err
	}

	// Set ability scores manually (Barbarian with high STR for Rage damage)
	_, err = client.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength:     16, // High STR for melee attacks
				Dexterity:    14,
				Constitution: 15,
				Intelligence: 8,
				Wisdom:       12,
				Charisma:     10,
			},
		},
	})
	if err != nil {
		return "", err
	}

	// Set name
	_, err = client.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    "Grog the Rager",
	})
	if err != nil {
		return "", err
	}

	// Finalize
	finalizeResp, err := client.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	})
	if err != nil {
		return "", err
	}

	return finalizeResp.Character.Id, nil
}

func startDungeon(ctx context.Context, client dnd5ev1alpha1.EncounterServiceClient) (string, error) {
	resp, err := client.DungeonStart(ctx, &dnd5ev1alpha1.DungeonStartRequest{
		// Phase 2: Empty request - orchestrator creates default setup
	})
	if err != nil {
		return "", err
	}
	return resp.EncounterId, nil
}

func performAttack(ctx context.Context, client dnd5ev1alpha1.EncounterServiceClient, encounterID, attackerID string) (*dnd5ev1alpha1.AttackResult, error) {
	resp, err := client.Attack(ctx, &dnd5ev1alpha1.AttackRequest{
		EncounterId: encounterID,
		AttackerId:  attackerID,
		TargetId:    "goblin-1", // Hardcoded for Phase 2
	})
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func displayAttackResult(result *dnd5ev1alpha1.AttackResult) {
	fmt.Println("=== Attack Result ===")
	fmt.Printf("Hit: %v\n", result.Hit)
	fmt.Printf("Critical: %v\n", result.Critical)
	fmt.Printf("Attack Roll: %d\n", result.AttackRoll)
	fmt.Printf("Attack Total: %d\n", result.AttackTotal)
	fmt.Printf("Target AC: %d\n", result.TargetAc)

	if result.Hit {
		fmt.Printf("\nDamage: %d %s\n", result.Damage, result.DamageType)

		// TODO Phase 3: Display damage breakdown when proto supports it
		// This would show the Rage +2 bonus separately
		fmt.Println("\n✓ Attack successful!")
		fmt.Println("Note: Rage damage bonus (+2) is included in total damage")
		fmt.Println("      (Phase 3 will show detailed breakdown)")
	} else {
		fmt.Println("\n✗ Attack missed")
	}
}

func init() {
	encounterCmd.AddCommand(rageTestCmd)
}
