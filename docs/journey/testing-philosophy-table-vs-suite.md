# Journey 006: The Great Testing Philosophy Debate - When to Use Table-Driven vs Suite Tests

## The Context

While migrating rpg-api from string-based constants to typed constants (races.Race, classes.Class, etc.), we hit a fundamental question about testing philosophy. With only 46% handler coverage and missing tests for most type conversions, how should we write tests for complex orchestration logic?

## The Realization

Not all tests are created equal, and **different types of logic need different testing approaches**.

## The Problem with Table-Driven Tests for Complex Logic

I initially suggested table-driven tests for the type conversions:

```go
func TestProtoRaceConversions(t *testing.T) {
    tests := []struct {
        name       string
        protoRace  dnd5ev1alpha1.Race
        expected   races.Race
        shouldFail bool
    }{
        {"human", dnd5ev1alpha1.Race_RACE_HUMAN, races.Human, false},
        {"elf", dnd5ev1alpha1.Race_RACE_ELF, races.Elf, false},
        // ... 20 more cases
    }
    // ...
}
```

The user called this out immediately: *"I really do not like having to interpret table driven tests. 'this string says x failed because of reason y'... let me look up at all the test cases where x is... why... what are we doing here?"*

## The Key Insight

**We're optimizing for the wrong thing.** Table-driven tests optimize for:
- Line count reduction
- Easy coverage addition
- DRY principles

But for complex orchestrations, we should optimize for:
- **Debuggability** - When a test fails at 2 AM, can you understand what broke?
- **Readability** - Can you read ONE test and understand the scenario?
- **Maintainability** - Can you modify one scenario without affecting others?

## When Table-Driven Tests ARE Appropriate

Simple input/output validation:

```go
func TestValidateEmail(t *testing.T) {
    tests := []struct {
        email string
        valid bool
    }{
        {"user@example.com", true},
        {"invalid", false},
        {"", false},
        {"user+tag@domain.co.uk", true},
    }
    // This is fine! Each case is independent and simple
}
```

Why this works:
- Each test case is atomic
- No complex setup required
- Failure message "TestValidateEmail/invalid_email" is self-explanatory
- No mock expectations to trace

## When Suite Tests with SetupSubTest ARE Better

Complex orchestration with mocks and state:

```go
func (s *OrchestratorTestSuite) SetupTest() {
    // Runs ONCE per Test function - create mocks
    s.ctrl = gomock.NewController(s.T())
    s.mockRepo = NewMockRepository(s.ctrl)
    s.orchestrator = NewOrchestrator(s.mockRepo)
}

func (s *OrchestratorTestSuite) SetupSubTest() {
    // Runs BEFORE EACH s.Run() - reset test data
    s.testDraft = &DraftData{
        ID:   "draft-123",
        Name: "Test Character",
    }
}

func (s *OrchestratorTestSuite) TestUpdateRace_Scenarios() {
    s.Run("adding_high_elf_adds_language_choice", func() {
        // ALL the context is RIGHT HERE
        s.testDraft.RaceChoice = RaceChoice{RaceID: races.Elf}
        
        s.mockRepo.EXPECT().
            Get(gomock.Any(), "draft-123").
            Return(s.testDraft, nil)
        
        result, err := s.orchestrator.UpdateRace(ctx, &UpdateRaceInput{
            DraftID:   "draft-123",
            RaceID:    races.Elf,
            SubraceID: races.HighElf,
        })
        
        s.NoError(err)
        s.Contains(result.Choices, expectedLanguageChoice)
    })
}
```

Why this is better for complex logic:
- Test name `TestUpdateRace_Scenarios/adding_high_elf_adds_language_choice` tells the whole story
- All setup, action, and assertion in one place
- Mock expectations are visible in the test, not in a table
- Each s.Run() gets fresh test data (no pollution)

## The Pattern We Settled On

1. **Use table-driven tests for**:
   - Simple validations
   - Input/output mappings without side effects
   - Cases where the test name alone explains the failure

2. **Use suite tests with SetupSubTest for**:
   - Complex orchestrations
   - Tests requiring mock setup
   - Scenarios where understanding requires seeing the full context
   - Testing variations of complex behavior

## The Philosophy

As the user put it: *"Tests are probably the easiest way to verify explicit assumptions."*

This means:
- Each test should make its assumptions explicit
- Complex tests need their full context visible
- We're optimizing for the person debugging, not the person writing

## The Lesson

**Don't cargo-cult testing patterns.** Table-driven tests became a Go "best practice" for good reasons, but those reasons don't apply to every situation. Complex orchestration logic needs tests that tell a complete story in one place.

The right test pattern depends on what you're testing. Simple validations? Table-driven is great. Complex business logic with mocks and state? Suite tests with SetupSubTest give you the clarity you need when things break.

## References

- PR #266: Constants package removal that triggered this discussion
- CLAUDE.md: Updated with testing philosophy
- The moment of clarity: "Who are you optimizing for?"