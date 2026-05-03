# Testing Strategy for RPG Platform

## Current Problems
1. **No visibility into what actually works** - We make changes, click around, and discover failures
2. **Bundle processing is broken** - Fighter equipment bundles don't properly expand (shield missing)
3. **Fighting style doesn't persist** - Selections aren't saved correctly
4. **No confidence in changes** - Every "fix" potentially breaks something else
5. **Assumptions vs Reality** - We assume the code does X but it actually does Y

## Testing Principles
1. **Test what we ship** - Integration tests over unit tests for critical paths
2. **Test the contract** - API responses and UI interactions, not implementation details
3. **Test incrementally** - Each layer should be testable in isolation
4. **Fast feedback** - Tests must run quickly to be useful during development

## Backend Testing Strategy

### 1. Integration Tests with Real Redis
**Location**: `/internal/handlers/dnd5e/v1alpha1/*_integration_test.go`

#### Character Creation Flow Tests
```go
// Test: Fighter with martial weapon + shield bundle
func TestFighterEquipmentBundleExpansion(t *testing.T) {
    // Given: A draft with fighter class
    // When: Selecting bundle_0 (martial weapon + shield) with longsword choice
    // Then: Character should have BOTH longsword AND shield in equipment
}

// Test: Fighting style persistence
func TestFightingStylePersistence(t *testing.T) {
    // Given: A draft with fighter class
    // When: Selecting "Defense" fighting style
    // Then: Fighting style should persist through draft updates
}
```

#### Key Test Scenarios
1. **Bundle Expansion**
   - Bundle with only concrete items (e.g., 2 handaxes)
   - Bundle with choice + concrete items (martial weapon + shield)
   - Multiple bundles in same choice

2. **Choice Persistence**
   - Equipment choices persist across updates
   - Fighting style choices persist
   - Skill choices persist
   - All choices survive draft reload

3. **Data Flow**
   - UpdateClass -> choices saved correctly
   - GetDraft -> choices returned correctly
   - FinalizeDraft -> choices become character equipment

### 2. Test Infrastructure
```go
// Shared test utilities
type IntegrationTestSuite struct {
    suite.Suite
    redis      *miniredis.Miniredis
    handler    *Handler
    draftID    string
}

func (s *IntegrationTestSuite) SetupTest() {
    // Fresh Redis for each test
    s.redis = miniredis.RunT(s.T())
    // Real handler with mocked external client
    s.handler = NewHandler(...)
}
```

## Frontend Testing Strategy

### 1. Component Tests
**Tool**: React Testing Library + Vitest

#### EquipmentChoice Component Tests
```typescript
// Test: Bundle with concrete items
test('selecting bundle with shield includes shield in selection', () => {
    const onSelectionChange = vi.fn();
    
    render(<EquipmentChoice 
        choice={fighterEquipmentChoice}
        onSelectionChange={onSelectionChange}
    />);
    
    // Select bundle with martial weapon + shield
    userEvent.click(screen.getByText('Martial weapon and shield'));
    userEvent.selectOptions(screen.getByLabelText('Choose weapon'), 'longsword');
    
    // Should send BOTH items
    expect(onSelectionChange).toHaveBeenCalledWith('fighter_equipment_2', [
        'bundle_0:0:longsword',
        'bundle_0:1:shield'
    ]);
});

// Test: Fighting style selection
test('fighting style selection is properly formatted', () => {
    const onSelectionChange = vi.fn();
    
    render(<FightingStyleChoice 
        choice={fighterFightingStyleChoice}
        onSelectionChange={onSelectionChange}
    />);
    
    userEvent.click(screen.getByText('Defense'));
    
    expect(onSelectionChange).toHaveBeenCalledWith('fighter_fighting_style', 'defense');
});
```

### 2. Integration Tests
**Tool**: Playwright or Cypress

```typescript
test('Fighter character creation flow', async ({ page }) => {
    // Start character creation
    await page.goto('/character/create');
    
    // Select fighter class
    await page.click('text=Fighter');
    
    // Select equipment bundle
    await page.click('text=Martial weapon and shield');
    await page.selectOption('select[name="weapon"]', 'longsword');
    
    // Select fighting style
    await page.click('text=Defense');
    
    // Save draft
    await page.click('button:has-text("Save")');
    
    // Verify equipment shows both items
    await expect(page.locator('.equipment-list')).toContainText('Longsword');
    await expect(page.locator('.equipment-list')).toContainText('Shield');
    
    // Verify fighting style persisted
    await expect(page.locator('.fighting-style')).toContainText('Defense');
});
```

## Implementation Plan

### Phase 1: Backend Integration Tests (Week 1)
1. Set up integration test suite with miniredis
2. Write tests for current broken functionality:
   - Fighter equipment bundle expansion
   - Fighting style persistence
   - Choice data flow through draft lifecycle
3. Fix the actual issues found by tests

### Phase 2: Frontend Component Tests (Week 2)
1. Set up React Testing Library
2. Write tests for EquipmentChoice component
3. Write tests for FightingStyleChoice component
4. Write tests for ClassSelectionModal
5. Fix issues discovered by tests

### Phase 3: E2E Tests (Week 3)
1. Set up Playwright/Cypress
2. Write critical path tests:
   - Complete fighter creation
   - Complete wizard creation
   - Draft save/load cycle
3. Run tests in CI pipeline

### Phase 4: Documentation (Ongoing)
1. Document actual behavior vs expected
2. Create decision log for why things work the way they do
3. Add comments explaining non-obvious logic

## Success Criteria
1. **No surprises** - Tests catch issues before manual testing
2. **Fast feedback** - Full test suite runs in < 2 minutes
3. **Confidence** - Can refactor without fear
4. **Documentation** - Tests serve as living documentation

## Anti-patterns to Avoid
1. **Testing implementation details** - Test behavior, not structure
2. **Brittle tests** - Don't test exact HTML structure or CSS classes
3. **Slow tests** - Keep integration tests focused
4. **Flaky tests** - No random data, consistent setup/teardown

## Next Steps
1. Start with the most broken feature: Fighter equipment bundles
2. Write integration test that demonstrates the bug
3. Fix the bug
4. Verify with test
5. Move to next issue

This approach ensures we KNOW what works instead of hoping and clicking.
