# Testing Patterns for RPG API

## Overview

This document describes the testing patterns established for the RPG API, focusing on clean, maintainable tests with proper data flow verification at each layer.

## Key Principles

1. **Use SetupSubTest for data reset** - Each test starts with fresh, predictable data
2. **Minimal data variations** - Tests only modify what they're testing
3. **Builder pattern for test data** - Reusable, fluent builders for common entities
4. **Mock expectation helpers** - Centralized helpers for common mock setups
5. **Verify data transformations** - Each layer test verifies its specific transformations

## Layer-Specific Testing

### Handler Layer
**Purpose**: Test proto ↔ domain conversions and request/response handling

```go
// What goes in: Proto requests
// What comes out: Proto responses
// What to test: Field mapping, enum conversions, validation

func (s *HandlerTestSuite) TestCreateDraft() {
    request := &dnd5ev1alpha1.CreateDraftRequest{
        PlayerId: "player-123",
        InitialData: &dnd5ev1alpha1.CharacterDraftData{
            Name: "Test Character",
        },
    }
    
    // Mock service layer returns domain entity
    s.mockCharService.EXPECT().
        CreateDraft(s.ctx, gomock.Any()).
        DoAndReturn(func(_ context.Context, input *character.CreateDraftInput) (*character.CreateDraftOutput, error) {
            // Verify proto was converted correctly
            s.Equal("player-123", input.PlayerID)
            s.Equal("Test Character", input.InitialData.Name)
            
            // Return domain entity
            return &character.CreateDraftOutput{
                Draft: builders.NewCharacterDraftBuilder().
                    WithID("draft-123").
                    WithPlayerID(input.PlayerID).
                    WithName(input.InitialData.Name).
                    Build(),
            }, nil
        })
    
    resp, err := s.handler.CreateDraft(s.ctx, request)
    
    // Verify domain was converted back to proto correctly
    s.NoError(err)
    s.Equal("draft-123", resp.Draft.Id)
    s.Equal("Test Character", resp.Draft.Name)
}
```

### Orchestrator Layer
**Purpose**: Test business logic, orchestration, and hydration

```go
// What goes in: Service input structs
// What transforms: CharacterDraft ↔ CharacterDraftData, hydration
// What comes out: Service output structs with hydrated entities

func (s *OrchestratorTestSuite) TestUpdateRace() {
    // Start with base draft from SetupSubTest
    draft := s.testDraft
    
    // Set up repository to return CharacterDraftData
    draftData := dnd5e.FromCharacterDraft(draft)
    mocks.ExpectDraftGet(s.ctx, s.mockDraftRepo, draft.ID, draftData, nil)
    
    // Set up engine validation
    s.mockEngine.EXPECT().
        ValidateRaceChoice(s.ctx, &engine.ValidateRaceChoiceInput{
            RaceID: dnd5e.RaceElf,
        }).
        Return(&engine.ValidateRaceChoiceOutput{IsValid: true}, nil)
    
    // Expect repository update with CharacterDraftData
    s.mockDraftRepo.EXPECT().
        Update(s.ctx, gomock.Any()).
        DoAndReturn(func(_ context.Context, input draftrepo.UpdateInput) (*draftrepo.UpdateOutput, error) {
            // Verify we're storing CharacterDraftData
            s.Equal(dnd5e.RaceElf, input.Draft.RaceID)
            return &draftrepo.UpdateOutput{Draft: input.Draft}, nil
        })
    
    // Set up hydration expectations
    mocks.ExpectDraftHydration(s.ctx, s.mockExternalClient, draft)
    
    // Execute
    input := &character.UpdateRaceInput{
        DraftID: draft.ID,
        RaceID:  dnd5e.RaceElf,
    }
    output, err := s.orchestrator.UpdateRace(s.ctx, input)
    
    // Verify hydrated result
    s.NoError(err)
    s.Equal(dnd5e.RaceElf, output.Draft.RaceID)
    s.NotNil(output.Draft.Race) // Hydrated info
}
```

### Repository Layer
**Purpose**: Test data persistence and retrieval

```go
// What goes in: Repository input structs with CharacterDraftData
// What transforms: CharacterDraftData ↔ JSON for Redis
// What comes out: Repository output structs with CharacterDraftData

func (s *RedisRepositoryTestSuite) TestCreate() {
    input := builders.NewCharacterDraftDataBuilder().
        WithPlayerID("player-123").
        WithName("Test Character").
        Build()
    
    // Mock Redis operations
    s.mockClient.EXPECT().Get(s.ctx, "draft:player:player-123").Return(redis.NewStringResult("", redis.Nil))
    s.mockClient.EXPECT().TxPipeline().Return(s.mockPipe)
    s.mockPipe.EXPECT().Set(s.ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(redis.NewStatusResult("OK", nil))
    s.mockPipe.EXPECT().Set(s.ctx, "draft:player:player-123", gomock.Any(), time.Duration(0)).Return(redis.NewStatusResult("OK", nil))
    s.mockPipe.EXPECT().Exec(s.ctx).Return([]redis.Cmder{}, nil)
    
    // Execute
    output, err := s.repo.Create(s.ctx, characterdraft.CreateInput{Draft: input})
    
    // Verify
    s.NoError(err)
    s.NotEmpty(output.Draft.ID) // Repository generated ID
    s.Equal("Test Character", output.Draft.Name)
}
```

## Test Data Builders

### CharacterDraftBuilder

```go
// Create a complete draft
draft := builders.NewCharacterDraftBuilder().
    AsComplete().
    Build()

// Create a minimal draft with just name
draft := builders.NewCharacterDraftBuilder().
    WithName("Frodo").
    Build()

// Create draft at specific progress point
draft := builders.NewCharacterDraftBuilder().
    WithName("Gandalf").
    WithRace(dnd5e.RaceHuman).
    WithClass(dnd5e.ClassWizard).
    Build()
// Automatically sets progress flags and calculates percentage
```

### CharacterDraftDataBuilder

```go
// For repository tests
draftData := builders.NewCharacterDraftDataBuilder().
    WithPlayerID("player-123").
    WithExpiration(time.Now().Add(24 * time.Hour).Unix()).
    Build()

// Convert from draft
draftData := builders.NewCharacterDraftBuilder().
    WithName("Test").
    BuildData() // Returns CharacterDraftData
```

## Mock Expectation Helpers

### Common Patterns

```go
// Expect draft hydration (handles race, class, background)
mocks.ExpectDraftHydration(ctx, mockClient, draft)

// Expect draft retrieval
mocks.ExpectDraftGet(ctx, mockRepo, draftID, draftData, nil)

// Expect draft creation (handles ID generation)
mocks.ExpectDraftCreate(ctx, mockRepo)

// Expect draft update (handles timestamp update)
mocks.ExpectDraftUpdate(ctx, mockRepo)
```

## SetupTest vs SetupSubTest

### SetupTest
- Initialize mocks and dependencies
- Set up base test IDs that won't change
- Create the system under test

### SetupSubTest
- Reset test data to known state
- Use builders to create fresh entities
- Clear any stateful data from previous tests

```go
func (s *TestSuite) SetupTest() {
    s.ctrl = gomock.NewController(s.T())
    s.mockRepo = NewMockRepository(s.ctrl)
    s.service = NewService(s.mockRepo)
    
    // Base IDs that don't change
    s.testDraftID = "draft-123"
    s.testPlayerID = "player-123"
}

func (s *TestSuite) SetupSubTest() {
    // Fresh test data for each test
    s.testDraft = builders.NewCharacterDraftBuilder().
        WithID(s.testDraftID).
        WithPlayerID(s.testPlayerID).
        WithName("Default Test Character").
        Build()
}
```

## Benefits

1. **Predictable tests** - Each test starts with known data state
2. **Minimal setup** - Only specify what's different for each test case
3. **Clear data flow** - Easy to trace what transforms at each layer
4. **Reusable patterns** - Builders and helpers reduce duplication
5. **Maintainable** - Changes to entities don't break every test

## Example: Lean Test Pattern

```go
func (s *Suite) TestUpdateField() {
    testCases := []struct {
        name      string
        inputValue string                    // Only what varies
        draftMod  func(*dnd5e.CharacterDraft) // Optional modifications
        setupMock func(*dnd5e.CharacterDraftData)
        wantErr   bool
    }{
        {
            name:       "update field successfully",
            inputValue: "new-value",
            setupMock: func(data *dnd5e.CharacterDraftData) {
                mocks.ExpectDraftGet(s.ctx, s.mockRepo, s.testDraftID, data, nil)
                mocks.ExpectDraftUpdate(s.ctx, s.mockRepo)
            },
        },
        {
            name:       "field already set",
            inputValue: "another-value",
            draftMod: func(d *dnd5e.CharacterDraft) {
                d.SomeField = "existing-value"
            },
            setupMock: func(data *dnd5e.CharacterDraftData) {
                mocks.ExpectDraftGet(s.ctx, s.mockRepo, s.testDraftID, data, nil)
                mocks.ExpectDraftUpdate(s.ctx, s.mockRepo)
            },
        },
    }
    
    for _, tc := range testCases {
        s.Run(tc.name, func() {
            // Start with base draft
            draft := s.testDraft
            
            // Apply modifications if needed
            if tc.draftMod != nil {
                draftCopy := *draft
                tc.draftMod(&draftCopy)
                draft = &draftCopy
            }
            
            // Set up mocks
            tc.setupMock(dnd5e.FromCharacterDraft(draft))
            
            // Execute and verify...
        })
    }
}
```

This pattern ensures tests are:
- Easy to read (clear what's being tested)
- Easy to write (minimal boilerplate)
- Easy to maintain (isolated changes)
- Reliable (predictable starting state)
