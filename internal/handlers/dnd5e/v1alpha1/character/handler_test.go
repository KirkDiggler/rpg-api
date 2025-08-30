package character

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	external "github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	orchestrator "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/languages"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/proficiencies"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/race"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
)

type HandlerTestSuite struct {
	suite.Suite
	ctrl             *gomock.Controller
	mockOrchestrator *charactermock.MockService
	handler          *Handler
}

func TestHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(HandlerTestSuite))
}

func (s *HandlerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockOrchestrator = charactermock.NewMockService(s.ctrl)

	handler, err := NewHandler(&HandlerConfig{
		CharacterService: s.mockOrchestrator,
	})
	s.Require().NoError(err)
	s.handler = handler
}

func (s *HandlerTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// ============================================================================
// CreateDraft Tests
// ============================================================================

func (s *HandlerTestSuite) TestCreateDraft_Success() {
	ctx := context.Background()

	// Setup request
	request := &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId:  "player123",
		SessionId: "session456",
		InitialData: &dnd5ev1alpha1.CharacterDraftData{
			Name: "Gandalf",
		},
	}

	// Expected orchestrator input
	expectedInput := &orchestrator.CreateDraftInput{
		PlayerID:  "player123",
		SessionID: "session456",
		InitialData: &toolkitchar.DraftData{
			Name: "Gandalf",
		},
	}

	// Mock response
	mockOutput := &orchestrator.CreateDraftOutput{
		Draft: &toolkitchar.DraftData{
			ID:   "draft789",
			Name: "Gandalf",
		},
	}

	s.mockOrchestrator.EXPECT().
		CreateDraft(gomock.Any(), expectedInput).
		Return(mockOutput, nil)

	// Execute
	resp, err := s.handler.CreateDraft(ctx, request)

	// Verify
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal("draft789", resp.Draft.Id)
	s.Equal("Gandalf", resp.Draft.Name)
}

func (s *HandlerTestSuite) TestCreateDraft_MissingPlayerID() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId:  "", // Missing
		SessionId: "session456",
	}

	// Execute
	resp, err := s.handler.CreateDraft(ctx, request)

	// Verify
	s.Nil(resp)
	s.Error(err)
	s.Equal(codes.InvalidArgument, status.Code(err))
	s.Contains(err.Error(), "player_id is required")
}

func (s *HandlerTestSuite) TestCreateDraft_OrchestratorError() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId:  "player123",
		SessionId: "session456",
	}

	expectedInput := &orchestrator.CreateDraftInput{
		PlayerID:  "player123",
		SessionID: "session456",
	}

	s.mockOrchestrator.EXPECT().
		CreateDraft(gomock.Any(), expectedInput).
		Return(nil, status.Error(codes.Internal, "database error"))

	// Execute
	resp, err := s.handler.CreateDraft(ctx, request)

	// Verify
	s.Nil(resp)
	s.Error(err)
	s.Equal(codes.Internal, status.Code(err))
}

func (s *HandlerTestSuite) TestCreateDraft_WithoutInitialData() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.CreateDraftRequest{
		PlayerId:  "player123",
		SessionId: "session456",
		// No InitialData
	}

	expectedInput := &orchestrator.CreateDraftInput{
		PlayerID:  "player123",
		SessionID: "session456",
		// No InitialData
	}

	mockOutput := &orchestrator.CreateDraftOutput{
		Draft: &toolkitchar.DraftData{
			ID: "draft789",
		},
	}

	s.mockOrchestrator.EXPECT().
		CreateDraft(gomock.Any(), expectedInput).
		Return(mockOutput, nil)

	// Execute
	resp, err := s.handler.CreateDraft(ctx, request)

	// Verify
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal("draft789", resp.Draft.Id)
}

// ============================================================================
// UpdateClass Tests
// ============================================================================

func (s *HandlerTestSuite) TestUpdateClass_Success() {
	ctx := context.Background()

	// Setup request with skill choices
	request := &dnd5ev1alpha1.UpdateClassRequest{
		DraftId:  "draft123",
		Class:    dnd5ev1alpha1.Class_CLASS_FIGHTER,
		Subclass: dnd5ev1alpha1.Subclass_SUBCLASS_UNSPECIFIED,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				ChoiceId: "fighter_skills",
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillList{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
							dnd5ev1alpha1.Skill_SKILL_INTIMIDATION,
						},
					},
				},
			},
		},
	}

	// Expected orchestrator input
	expectedInput := &orchestrator.UpdateClassInput{
		DraftID:    "draft123",
		ClassID:    classes.Fighter,
		SubclassID: "",
		Choices: []toolkitchar.ChoiceData{
			{
				ChoiceID: "fighter_skills",
				Category: shared.ChoiceSkills,
				Source:   shared.SourceClass,
				SkillSelection: []skills.Skill{
					skills.Athletics,
					skills.Intimidation,
				},
			},
		},
	}

	// Mock response
	mockOutput := &orchestrator.UpdateClassOutput{
		Draft: &toolkitchar.DraftData{
			ID:   "draft123",
			Name: "Aragorn",
			ClassChoice: toolkitchar.ClassChoice{
				ClassID: classes.Fighter,
			},
		},
		Warnings: []orchestrator.ValidationWarning{},
	}

	s.mockOrchestrator.EXPECT().
		UpdateClass(gomock.Any(), expectedInput).
		Return(mockOutput, nil)

	// Execute
	resp, err := s.handler.UpdateClass(ctx, request)

	// Verify
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal("draft123", resp.Draft.Id)
	s.Empty(resp.Warnings)
}

func (s *HandlerTestSuite) TestUpdateClass_WithSubclass() {
	ctx := context.Background()

	// Setup request with Cleric and Life Domain
	request := &dnd5ev1alpha1.UpdateClassRequest{
		DraftId:  "draft123",
		Class:    dnd5ev1alpha1.Class_CLASS_CLERIC,
		Subclass: dnd5ev1alpha1.Subclass_SUBCLASS_UNSPECIFIED, // TODO: Find correct Life Domain enum
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				ChoiceId: "cleric_skills",
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillList{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_MEDICINE,
							dnd5ev1alpha1.Skill_SKILL_RELIGION,
						},
					},
				},
			},
		},
	}

	// Expected input
	expectedInput := &orchestrator.UpdateClassInput{
		DraftID:    "draft123",
		ClassID:    classes.Cleric,
		SubclassID: "", // Would be classes.LifeDomain if proto had the right enum
		Choices: []toolkitchar.ChoiceData{
			{
				ChoiceID: "cleric_skills",
				Category: shared.ChoiceSkills,
				Source:   shared.SourceClass,
				SkillSelection: []skills.Skill{
					skills.Medicine,
					skills.Religion,
				},
			},
		},
	}

	mockOutput := &orchestrator.UpdateClassOutput{
		Draft: &toolkitchar.DraftData{
			ID: "draft123",
			ClassChoice: toolkitchar.ClassChoice{
				ClassID: classes.Cleric,
				// SubclassID would be classes.LifeDomain
			},
		},
		Warnings: []orchestrator.ValidationWarning{},
	}

	s.mockOrchestrator.EXPECT().
		UpdateClass(gomock.Any(), expectedInput).
		Return(mockOutput, nil)

	// Execute
	resp, err := s.handler.UpdateClass(ctx, request)

	// Verify
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
}

func (s *HandlerTestSuite) TestUpdateClass_InvalidClass() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.UpdateClassRequest{
		DraftId:  "draft123",
		Class:    dnd5ev1alpha1.Class_CLASS_UNSPECIFIED,
		Subclass: dnd5ev1alpha1.Subclass_SUBCLASS_UNSPECIFIED,
	}

	// No mock expectation - should fail validation

	// Execute
	resp, err := s.handler.UpdateClass(ctx, request)

	// Verify
	s.Nil(resp)
	s.Error(err)
	s.Equal(codes.InvalidArgument, status.Code(err))
	s.Contains(err.Error(), "invalid class")
}

func (s *HandlerTestSuite) TestUpdateClass_WithWarnings() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.UpdateClassRequest{
		DraftId:      "draft123",
		Class:        dnd5ev1alpha1.Class_CLASS_FIGHTER,
		Subclass:     dnd5ev1alpha1.Subclass_SUBCLASS_UNSPECIFIED,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{},
	}

	expectedInput := &orchestrator.UpdateClassInput{
		DraftID:    "draft123",
		ClassID:    classes.Fighter,
		SubclassID: "",
		Choices:    nil, // Handler sends nil when no choices provided
	}

	mockOutput := &orchestrator.UpdateClassOutput{
		Draft: &toolkitchar.DraftData{
			ID: "draft123",
			ClassChoice: toolkitchar.ClassChoice{
				ClassID: classes.Fighter,
			},
		},
		Warnings: []orchestrator.ValidationWarning{
			{
				Field:   "skills",
				Message: "Some skills were already selected",
				Type:    "duplicate",
			},
		},
	}

	s.mockOrchestrator.EXPECT().
		UpdateClass(gomock.Any(), expectedInput).
		Return(mockOutput, nil)

	// Execute
	resp, err := s.handler.UpdateClass(ctx, request)

	// Verify
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Warnings, 1)
	s.Equal("skills", resp.Warnings[0].Field)
	s.Equal("Some skills were already selected", resp.Warnings[0].Message)
}

// ============================================================================
// ListClasses Tests
// ============================================================================

func (s *HandlerTestSuite) TestListClasses_Success() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.ListClassesRequest{
		PageSize:  10,
		PageToken: "",
	}

	expectedInput := &orchestrator.ListClassesInput{
		PageSize:  10,
		PageToken: "",
	}

	// Setup mock response with test classes that exercise different converter paths
	mockOutput := &orchestrator.ListClassesOutput{
		Classes: []*toolkitchar.StartingClass{
			{
				// Fighter - simple martial class, no subclass at level 1
				ID: classes.Fighter,
				Grants: &classes.AutomaticGrants{
					HitDice:      10,
					SavingThrows: []abilities.Ability{abilities.STR, abilities.CON},
					WeaponProficiencies: []proficiencies.Weapon{
						proficiencies.WeaponSimple,
						proficiencies.WeaponMartial,
					},
					ArmorProficiencies: []proficiencies.Armor{
						proficiencies.ArmorLight,
						proficiencies.ArmorMedium,
						proficiencies.ArmorHeavy,
						proficiencies.ArmorShields,
					},
				},
				Requirements: &choices.Requirements{
					Skills: &choices.SkillRequirement{
						Count:   2,
						Options: []skills.Skill{skills.Athletics, skills.Intimidation},
					},
				},
			},
			{
				// Cleric - has level 1 subclasses
				ID: classes.Cleric,
				Grants: &classes.AutomaticGrants{
					HitDice:      8,
					SavingThrows: []abilities.Ability{abilities.WIS, abilities.CHA},
					WeaponProficiencies: []proficiencies.Weapon{
						proficiencies.WeaponSimple,
					},
					ArmorProficiencies: []proficiencies.Armor{
						proficiencies.ArmorLight,
						proficiencies.ArmorMedium,
						proficiencies.ArmorShields,
					},
				},
				Subclass: []*toolkitchar.SubclassOption{
					{
						ID:    classes.LifeDomain,
						Level: 1,
						Grants: &classes.AutomaticGrants{
							// Life Domain gets bonus proficiency with heavy armor
							ArmorProficiencies: []proficiencies.Armor{
								proficiencies.ArmorHeavy,
							},
						},
					},
					{
						ID:    classes.LightDomain,
						Level: 1,
						// Light domain might have different grants
					},
				},
			},
			{
				// Rogue - unique features, specific weapon list
				ID: classes.Rogue,
				Grants: &classes.AutomaticGrants{
					HitDice:      8,
					SavingThrows: []abilities.Ability{abilities.DEX, abilities.INT},
					WeaponProficiencies: []proficiencies.Weapon{
						proficiencies.WeaponSimple,
						proficiencies.WeaponHandCrossbow,
						proficiencies.WeaponLongsword,
						proficiencies.WeaponRapier,
						proficiencies.WeaponShortsword,
					},
					ArmorProficiencies: []proficiencies.Armor{
						proficiencies.ArmorLight,
					},
					// TODO: Add tool proficiencies when Tool constants are defined
					// ToolProficiencies: []proficiencies.Tool{
					//     proficiencies.ToolThievesTools,
					// },
				},
				Requirements: &choices.Requirements{
					Skills: &choices.SkillRequirement{
						Count: 4, // Rogues get 4 skills
						Options: []skills.Skill{
							skills.Acrobatics, skills.Athletics, skills.Deception,
							skills.Insight, skills.Intimidation, skills.Investigation,
							skills.Perception, skills.Performance, skills.Persuasion,
							skills.SleightOfHand, skills.Stealth,
						},
					},
				},
			},
		},
		TotalSize:     3,
		NextPageToken: "",
	}

	s.mockOrchestrator.EXPECT().
		ListClasses(gomock.Any(), expectedInput).
		Return(mockOutput, nil)

	// Execute
	resp, err := s.handler.ListClasses(ctx, request)

	// Verify
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Classes, 3)

	// Check Fighter - simple martial class
	fighter := resp.Classes[0]
	s.Equal("fighter", fighter.Id)
	s.Equal("Fighter", fighter.Name)
	s.Equal(dnd5ev1alpha1.Class_CLASS_FIGHTER, fighter.Class)
	s.Equal("1d10", fighter.HitDie)
	s.Contains(fighter.SavingThrowProficiencies, "str")
	s.Contains(fighter.SavingThrowProficiencies, "con")
	s.NotEmpty(fighter.WeaponProficiencies)
	s.NotEmpty(fighter.ArmorProficiencies)

	// Check Cleric - has subclasses at level 1
	cleric := resp.Classes[1]
	s.Equal("cleric", cleric.Id)
	s.Equal("Cleric", cleric.Name)
	s.Equal(dnd5ev1alpha1.Class_CLASS_CLERIC, cleric.Class)
	s.Equal("1d8", cleric.HitDie)
	s.NotNil(cleric.Subclasses)
	s.Len(cleric.Subclasses, 2)
	// Check Life Domain subclass
	if len(cleric.Subclasses) > 0 {
		lifeDomain := cleric.Subclasses[0]
		s.Equal("life-domain", lifeDomain.Id)
		s.Equal("Life Domain", lifeDomain.Name)
		s.Equal(int32(1), lifeDomain.Level)
	}

	// Check Rogue - unique features
	rogue := resp.Classes[2]
	s.Equal("rogue", rogue.Id)
	s.Equal("Rogue", rogue.Name)
	s.Equal(dnd5ev1alpha1.Class_CLASS_ROGUE, rogue.Class)
	s.Equal("1d8", rogue.HitDie)
	s.Equal(int32(4), rogue.SkillChoicesCount) // Rogues get 4 skills
}

func (s *HandlerTestSuite) TestListClasses_Empty() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.ListClassesRequest{
		PageSize:  10,
		PageToken: "",
	}

	expectedInput := &orchestrator.ListClassesInput{
		PageSize:  10,
		PageToken: "",
	}

	mockOutput := &orchestrator.ListClassesOutput{
		Classes:       []*toolkitchar.StartingClass{},
		TotalSize:     0,
		NextPageToken: "",
	}

	s.mockOrchestrator.EXPECT().
		ListClasses(gomock.Any(), expectedInput).
		Return(mockOutput, nil)

	// Execute
	resp, err := s.handler.ListClasses(ctx, request)

	// Verify
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Classes)
	s.Equal(int32(0), resp.TotalSize)
}

func (s *HandlerTestSuite) TestListClasses_WithPagination() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.ListClassesRequest{
		PageSize:  5,
		PageToken: "token123",
	}

	expectedInput := &orchestrator.ListClassesInput{
		PageSize:  5,
		PageToken: "token123",
	}

	mockOutput := &orchestrator.ListClassesOutput{
		Classes: []*toolkitchar.StartingClass{
			{
				ID: classes.Wizard,
				Grants: &classes.AutomaticGrants{
					HitDice: 6,
				},
			},
		},
		TotalSize:     10,
		NextPageToken: "token456",
	}

	s.mockOrchestrator.EXPECT().
		ListClasses(gomock.Any(), expectedInput).
		Return(mockOutput, nil)

	// Execute
	resp, err := s.handler.ListClasses(ctx, request)

	// Verify
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Classes, 1)
	s.Equal(int32(10), resp.TotalSize)
	s.Equal("token456", resp.NextPageToken)
}

func (s *HandlerTestSuite) TestListClasses_OrchestratorError() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.ListClassesRequest{
		PageSize:  10,
		PageToken: "",
	}

	expectedInput := &orchestrator.ListClassesInput{
		PageSize:  10,
		PageToken: "",
	}

	s.mockOrchestrator.EXPECT().
		ListClasses(gomock.Any(), expectedInput).
		Return(nil, status.Error(codes.Internal, "database unavailable"))

	// Execute
	resp, err := s.handler.ListClasses(ctx, request)

	// Verify
	s.Nil(resp)
	s.Error(err)
	s.Equal(codes.Internal, status.Code(err))
}

// ============================================================================
// ListRaces and UpdateRace Tests
// ============================================================================

func (s *HandlerTestSuite) TestListRaces_Success() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.ListRacesRequest{
		PageSize:  10,
		PageToken: "",
	}

	expectedInput := &orchestrator.ListRacesInput{
		PageSize:  10,
		PageToken: "",
	}

	// Mock response with races that have different structures
	mockOutput := &orchestrator.ListRacesOutput{
		Races: []orchestrator.RaceListItem{
			{
				RaceData: &race.Data{
					ID:          races.Dwarf,
					Name:        "Dwarf",
					Speed:       25,
					Size:        "Medium",
					Description: "Bold and hardy",
					Subraces: []race.SubraceData{
						{ID: races.HillDwarf, Name: "Hill Dwarf"},
						{ID: races.MountainDwarf, Name: "Mountain Dwarf"},
					},
				},
				UIData: &external.RaceUIData{
					SizeDescription:      "Medium",
					AgeDescription:       "Dwarves mature at 50",
					AlignmentDescription: "Lawful",
				},
			},
			{
				RaceData: &race.Data{
					ID:          races.Human,
					Name:        "Human",
					Speed:       30,
					Size:        "Medium",
					Description: "Versatile",
					// No subraces
				},
				UIData: &external.RaceUIData{
					SizeDescription:      "Medium",
					AgeDescription:       "Humans reach adulthood in their late teens",
					AlignmentDescription: "Any",
				},
			},
			{
				RaceData: &race.Data{
					ID:          races.Elf,
					Name:        "Elf",
					Speed:       30,
					Size:        "Medium",
					Description: "Graceful",
					Subraces: []race.SubraceData{
						{ID: races.HighElf, Name: "High Elf"},
						{ID: races.WoodElf, Name: "Wood Elf"},
						{ID: races.DarkElf, Name: "Dark Elf (Drow)"},
					},
				},
				UIData: &external.RaceUIData{
					SizeDescription:      "Medium",
					AgeDescription:       "Elves reach adulthood at 100",
					AlignmentDescription: "Chaotic",
				},
			},
		},
		TotalSize:     3,
		NextPageToken: "",
	}

	s.mockOrchestrator.EXPECT().
		ListRaces(gomock.Any(), expectedInput).
		Return(mockOutput, nil)

	// Execute
	resp, err := s.handler.ListRaces(ctx, request)

	// Verify
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Races, 3)

	// Check Dwarf with subraces
	dwarf := resp.Races[0]
	s.Equal("dwarf", dwarf.Id)
	s.Equal("Dwarf", dwarf.Name)
	s.Equal(int32(25), dwarf.Speed)
	s.NotNil(dwarf.Subraces)
	s.Len(dwarf.Subraces, 2)

	// Check Human without subraces
	human := resp.Races[1]
	s.Equal("human", human.Id)
	s.Equal("Human", human.Name)
	s.Equal(int32(30), human.Speed)
	s.Empty(human.Subraces)

	// Check Elf with subraces
	elf := resp.Races[2]
	s.Equal("elf", elf.Id)
	s.Equal("Elf", elf.Name)
	s.Len(elf.Subraces, 3)
}

func (s *HandlerTestSuite) TestUpdateRace_WithSubrace() {
	ctx := context.Background()

	// Test with a race that has subraces (Dwarf -> Hill Dwarf)
	request := &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: "draft123",
		Race:    dnd5ev1alpha1.Race_RACE_DWARF,
		Subrace: dnd5ev1alpha1.Subrace_SUBRACE_HILL_DWARF,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				ChoiceId: "dwarf_tool_proficiency",
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
				// Dwarves get to choose tool proficiency
			},
		},
	}

	expectedInput := &orchestrator.UpdateRaceInput{
		DraftID:   "draft123",
		RaceID:    races.Dwarf,
		SubraceID: races.HillDwarf,
		Choices: []toolkitchar.ChoiceData{
			{
				ChoiceID: "dwarf_tool_proficiency",
				Category: shared.ChoiceToolProficiency,
				Source:   shared.SourceRace,
			},
		},
	}

	mockOutput := &orchestrator.UpdateRaceOutput{
		Draft: &toolkitchar.DraftData{
			ID:   "draft123",
			Name: "Thorin",
			RaceChoice: toolkitchar.RaceChoice{
				RaceID:    races.Dwarf,
				SubraceID: races.HillDwarf,
			},
		},
		Warnings: []orchestrator.ValidationWarning{},
	}

	s.mockOrchestrator.EXPECT().
		UpdateRace(gomock.Any(), expectedInput).
		Return(mockOutput, nil)

	// Execute
	resp, err := s.handler.UpdateRace(ctx, request)

	// Verify
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal("draft123", resp.Draft.Id)
	s.Empty(resp.Warnings)
}

func (s *HandlerTestSuite) TestUpdateRace_NoSubrace() {
	ctx := context.Background()

	// Test with a race that has no subraces (Human)
	request := &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: "draft123",
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
		Subrace: dnd5ev1alpha1.Subrace_SUBRACE_UNSPECIFIED,
	}

	expectedInput := &orchestrator.UpdateRaceInput{
		DraftID:   "draft123",
		RaceID:    races.Human,
		SubraceID: "",
		Choices:   nil,
	}

	mockOutput := &orchestrator.UpdateRaceOutput{
		Draft: &toolkitchar.DraftData{
			ID:   "draft123",
			Name: "Aragorn",
			RaceChoice: toolkitchar.RaceChoice{
				RaceID: races.Human,
			},
		},
		Warnings: []orchestrator.ValidationWarning{},
	}

	s.mockOrchestrator.EXPECT().
		UpdateRace(gomock.Any(), expectedInput).
		Return(mockOutput, nil)

	// Execute
	resp, err := s.handler.UpdateRace(ctx, request)

	// Verify
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal("draft123", resp.Draft.Id)
}

// ============================================================================
// UpdateBackground Tests
// ============================================================================

func (s *HandlerTestSuite) TestUpdateBackground_Success() {
	ctx := context.Background()

	// Test with Acolyte background which has equipment choices
	request := &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    "draft123",
		Background: dnd5ev1alpha1.Background_BACKGROUND_ACOLYTE,
		BackgroundChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				ChoiceId: "acolyte_languages",
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_BACKGROUND,
				Selection: &dnd5ev1alpha1.ChoiceData_Languages{
					Languages: &dnd5ev1alpha1.LanguageList{
						Languages: []dnd5ev1alpha1.Language{
							dnd5ev1alpha1.Language_LANGUAGE_ELVISH,
							dnd5ev1alpha1.Language_LANGUAGE_DWARVISH,
						},
					},
				},
			},
		},
	}

	expectedInput := &orchestrator.UpdateBackgroundInput{
		DraftID:      "draft123",
		BackgroundID: backgrounds.Acolyte,
		Choices: []toolkitchar.ChoiceData{
			{
				ChoiceID: "acolyte_languages",
				Category: shared.ChoiceLanguages,
				Source:   shared.SourceBackground,
				LanguageSelection: []languages.Language{
					languages.Elvish,
					languages.Dwarvish,
				},
			},
		},
	}

	mockOutput := &orchestrator.UpdateBackgroundOutput{
		Draft: &toolkitchar.DraftData{
			ID:               "draft123",
			Name:             "Brother Francis",
			BackgroundChoice: backgrounds.Acolyte,
		},
		// No warnings
	}

	s.mockOrchestrator.EXPECT().
		UpdateBackground(gomock.Any(), expectedInput).
		Return(mockOutput, nil)

	// Execute
	resp, err := s.handler.UpdateBackground(ctx, request)

	// Verify
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal("draft123", resp.Draft.Id)
	s.Empty(resp.Warnings)
}

func (s *HandlerTestSuite) TestUpdateBackground_InvalidBackground() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    "draft123",
		Background: dnd5ev1alpha1.Background_BACKGROUND_UNSPECIFIED,
	}

	// No mock expectation - should fail validation

	// Execute
	resp, err := s.handler.UpdateBackground(ctx, request)

	// Verify
	s.Nil(resp)
	s.Error(err)
	s.Equal(codes.InvalidArgument, status.Code(err))
	s.Contains(err.Error(), "invalid background")
}

// ============================================================================
// UpdateAbilityScores Tests
// ============================================================================

func (s *HandlerTestSuite) TestUpdateAbilityScores_SuccessWithRollAssignments() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: "draft123",
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_RollAssignments{
			RollAssignments: &dnd5ev1alpha1.RollAssignments{
				StrengthRollId:     "roll_str_1",
				DexterityRollId:    "roll_dex_2",
				ConstitutionRollId: "roll_con_3",
				IntelligenceRollId: "roll_int_4",
				WisdomRollId:       "roll_wis_5",
				CharismaRollId:     "roll_cha_6",
			},
		},
	}

	expectedInput := &orchestrator.UpdateAbilityScoresInput{
		DraftID: "draft123",
		RollAssignments: &orchestrator.RollAssignments{
			StrengthRollID:     "roll_str_1",
			DexterityRollID:    "roll_dex_2",
			ConstitutionRollID: "roll_con_3",
			IntelligenceRollID: "roll_int_4",
			WisdomRollID:       "roll_wis_5",
			CharismaRollID:     "roll_cha_6",
		},
	}

	mockOutput := &orchestrator.UpdateAbilityScoresOutput{
		Draft: &toolkitchar.DraftData{
			ID:   "draft123",
			Name: "Test Character",
			ClassChoice: toolkitchar.ClassChoice{
				ClassID: classes.Fighter,
			},
			RaceChoice: toolkitchar.RaceChoice{
				RaceID: races.Human,
			},
			AbilityScoreChoice: shared.AbilityScores{
				abilities.STR: 15,
				abilities.DEX: 14,
				abilities.CON: 13,
				abilities.INT: 12,
				abilities.WIS: 10,
				abilities.CHA: 8,
			},
		},
		Warnings: []orchestrator.ValidationWarning{
			{
				Field:   "wisdom",
				Message: "Wisdom is below recommended for Cleric",
				Type:    "suggestion",
			},
		},
	}

	s.mockOrchestrator.EXPECT().
		UpdateAbilityScores(gomock.Any(), expectedInput).
		Return(mockOutput, nil)

	// Execute
	resp, err := s.handler.UpdateAbilityScores(ctx, request)

	// Verify
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Draft)
	s.Equal("draft123", resp.Draft.Id)
	s.Len(resp.Warnings, 1)
	s.Equal("wisdom", resp.Warnings[0].Field)
}

func (s *HandlerTestSuite) TestUpdateAbilityScores_MissingDraftID() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_RollAssignments{
			RollAssignments: &dnd5ev1alpha1.RollAssignments{
				StrengthRollId: "roll_1",
			},
		},
	}

	// No mock expectation - should fail validation

	// Execute
	resp, err := s.handler.UpdateAbilityScores(ctx, request)

	// Verify
	s.Nil(resp)
	s.Error(err)
	s.Equal(codes.InvalidArgument, status.Code(err))
	s.Contains(err.Error(), "draft_id is required")
}

func (s *HandlerTestSuite) TestUpdateAbilityScores_MissingRollIDs() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: "draft123",
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_RollAssignments{
			RollAssignments: &dnd5ev1alpha1.RollAssignments{
				StrengthRollId:  "roll_1",
				DexterityRollId: "roll_2",
				// Missing other rolls
			},
		},
	}

	// No mock expectation - should fail validation

	// Execute
	resp, err := s.handler.UpdateAbilityScores(ctx, request)

	// Verify
	s.Nil(resp)
	s.Error(err)
	s.Equal(codes.InvalidArgument, status.Code(err))
	s.Contains(err.Error(), "all ability score roll IDs must be provided")
}

func (s *HandlerTestSuite) TestUpdateAbilityScores_NoScoresInput() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: "draft123",
		// No ScoresInput
	}

	// No mock expectation - should fail validation

	// Execute
	resp, err := s.handler.UpdateAbilityScores(ctx, request)

	// Verify
	s.Nil(resp)
	s.Error(err)
	s.Equal(codes.InvalidArgument, status.Code(err))
	s.Contains(err.Error(), "scores_input must be provided")
}

// ============================================================================
// RollAbilityScores Tests
// ============================================================================

func (s *HandlerTestSuite) TestRollAbilityScores_Success() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.RollAbilityScoresRequest{
		DraftId: "draft123",
	}

	expectedInput := &orchestrator.RollAbilityScoresInput{
		DraftID: "draft123",
	}

	expiresAt := time.Now().Add(15 * time.Minute)
	mockOutput := &orchestrator.RollAbilityScoresOutput{
		Rolls: []*orchestrator.AbilityScoreRoll{
			{
				RollID:      "roll_1",
				Total:       18,
				Description: "4d6 drop lowest",
				Dice:        []int32{6, 6, 6, 3},
				Dropped:     []int32{3},
			},
			{
				RollID:      "roll_2",
				Total:       15,
				Description: "4d6 drop lowest",
				Dice:        []int32{5, 5, 5, 2},
				Dropped:     []int32{2},
			},
			{
				RollID:      "roll_3",
				Total:       14,
				Description: "4d6 drop lowest",
				Dice:        []int32{6, 4, 4, 2},
				Dropped:     []int32{2},
			},
			{
				RollID:      "roll_4",
				Total:       13,
				Description: "4d6 drop lowest",
				Dice:        []int32{5, 4, 4, 1},
				Dropped:     []int32{1},
			},
			{
				RollID:      "roll_5",
				Total:       12,
				Description: "4d6 drop lowest",
				Dice:        []int32{4, 4, 4, 3},
				Dropped:     []int32{3},
			},
			{
				RollID:      "roll_6",
				Total:       10,
				Description: "4d6 drop lowest",
				Dice:        []int32{4, 3, 3, 2},
				Dropped:     []int32{2},
			},
		},
		SessionID: "session_abc",
		ExpiresAt: expiresAt,
	}

	s.mockOrchestrator.EXPECT().
		RollAbilityScores(gomock.Any(), expectedInput).
		Return(mockOutput, nil)

	// Execute
	resp, err := s.handler.RollAbilityScores(ctx, request)

	// Verify
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Rolls, 6)
	s.Equal("roll_1", resp.Rolls[0].RollId)
	s.Equal(int32(18), resp.Rolls[0].Total)
	s.Equal([]int32{6, 6, 6, 3}, resp.Rolls[0].Dice)
	s.Equal(int32(3), resp.Rolls[0].Dropped)
	s.Equal(expiresAt.Unix(), resp.ExpiresAt)
}

func (s *HandlerTestSuite) TestRollAbilityScores_MissingDraftID() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.RollAbilityScoresRequest{
		// No DraftId
	}

	// No mock expectation - should fail validation

	// Execute
	resp, err := s.handler.RollAbilityScores(ctx, request)

	// Verify
	s.Nil(resp)
	s.Error(err)
	s.Equal(codes.InvalidArgument, status.Code(err))
	s.Contains(err.Error(), "draft_id is required")
}

func (s *HandlerTestSuite) TestRollAbilityScores_ServiceError() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.RollAbilityScoresRequest{
		DraftId: "draft123",
	}

	expectedInput := &orchestrator.RollAbilityScoresInput{
		DraftID: "draft123",
	}

	s.mockOrchestrator.EXPECT().
		RollAbilityScores(gomock.Any(), expectedInput).
		Return(nil, errors.NotFound("draft not found"))

	// Execute
	resp, err := s.handler.RollAbilityScores(ctx, request)

	// Verify
	s.Nil(resp)
	s.Error(err)
	s.Equal(codes.NotFound, status.Code(err))
	s.Contains(err.Error(), "draft not found")
}

// ============================================================================
// FinalizeDraft Tests
// ============================================================================

func (s *HandlerTestSuite) TestFinalizeDraft_Success() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: "draft123",
	}

	expectedInput := &orchestrator.FinalizeDraftInput{
		DraftID: "draft123",
	}

	mockOutput := &orchestrator.FinalizeDraftOutput{
		Character: &toolkitchar.Data{
			ID:      "char_456",
			Name:    "Aragorn",
			Level:   1,
			ClassID: classes.Ranger,
			RaceID:  races.Human,
			AbilityScores: shared.AbilityScores{
				abilities.STR: 16,
				abilities.DEX: 14,
				abilities.CON: 13,
				abilities.INT: 12,
				abilities.WIS: 15,
				abilities.CHA: 10,
			},
		},
		DraftDeleted: true,
	}

	s.mockOrchestrator.EXPECT().
		FinalizeDraft(gomock.Any(), expectedInput).
		Return(mockOutput, nil)

	// Execute
	resp, err := s.handler.FinalizeDraft(ctx, request)

	// Verify
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Character)
	s.Equal("char_456", resp.Character.Id)
	s.Equal("Aragorn", resp.Character.Name)
	s.Equal(int32(1), resp.Character.Level)
	s.True(resp.DraftDeleted)
}

func (s *HandlerTestSuite) TestFinalizeDraft_MissingDraftID() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.FinalizeDraftRequest{
		// No DraftId
	}

	// No mock expectation - should fail validation

	// Execute
	resp, err := s.handler.FinalizeDraft(ctx, request)

	// Verify
	s.Nil(resp)
	s.Error(err)
	s.Equal(codes.InvalidArgument, status.Code(err))
	s.Contains(err.Error(), "draft_id is required")
}

func (s *HandlerTestSuite) TestFinalizeDraft_DraftNotFound() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: "draft123",
	}

	expectedInput := &orchestrator.FinalizeDraftInput{
		DraftID: "draft123",
	}

	s.mockOrchestrator.EXPECT().
		FinalizeDraft(gomock.Any(), expectedInput).
		Return(nil, errors.NotFound("draft not found"))

	// Execute
	resp, err := s.handler.FinalizeDraft(ctx, request)

	// Verify
	s.Nil(resp)
	s.Error(err)
	s.Equal(codes.NotFound, status.Code(err))
	s.Contains(err.Error(), "draft not found")
}

func (s *HandlerTestSuite) TestFinalizeDraft_InvalidDraft() {
	ctx := context.Background()

	request := &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: "draft123",
	}

	expectedInput := &orchestrator.FinalizeDraftInput{
		DraftID: "draft123",
	}

	s.mockOrchestrator.EXPECT().
		FinalizeDraft(gomock.Any(), expectedInput).
		Return(nil, errors.InvalidArgument("draft is incomplete: missing ability scores"))

	// Execute
	resp, err := s.handler.FinalizeDraft(ctx, request)

	// Verify
	s.Nil(resp)
	s.Error(err)
	s.Equal(codes.InvalidArgument, status.Code(err))
	s.Contains(err.Error(), "draft is incomplete")
}
