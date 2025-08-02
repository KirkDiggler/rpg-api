# Proposed CLAUDE.md Rules to Prevent Sloppiness

## 🚨 CRITICAL RULES - NEVER VIOLATE

### 1. NO MAGIC STRINGS - EVER
```go
// ❌ NEVER DO THIS
if category == "equipment" { }
if itemType == "single_item" { }
bundleItem.Type = "item"

// ✅ ALWAYS DO THIS
if category == shared.ChoiceEquipment { }
if itemType == constants.ItemTypeSingle { }
bundleItem.Type = constants.BundleItemTypeItem
```

**RULE**: Before writing ANY string literal, ask:
- Does a constant exist for this?
- Should a constant exist for this?
- If no constant exists, CREATE ONE FIRST

### 2. STRICT LAYER SEPARATION

#### Handler Layer - ONLY ALLOWED TO:
- Map proto ↔ domain objects
- Basic field validation (empty strings, nil checks)
- Call orchestrator methods
- Return errors as gRPC status codes

**FORBIDDEN IN HANDLERS**:
- Business logic
- Complex transformations
- Calling external services
- Making decisions
- Creating complex structures

#### Orchestrator Layer - RESPONSIBLE FOR:
- ALL business logic
- Calling external services
- Complex transformations
- Data enhancement
- Coordinating between services
- Making decisions

#### External Client - ONLY ALLOWED TO:
- Call external APIs
- Map API responses to domain objects
- Basic error handling
- NO business logic
- NO complex transformations

### 3. WHEN IN DOUBT, STOP

Before implementing ANYTHING:
1. Which layer should this go in?
2. Are there constants for all strings?
3. Is this the simplest approach?
4. Am I following existing patterns?

If unsure about ANY of these - STOP and ask.

### 4. NO "FIX IT LATER"

**NEVER** implement something wrong with the intention to fix it later:
- Don't add complex logic to handlers
- Don't use magic strings temporarily
- Don't violate layer separation "for now"

Do it right the first time or don't do it.

### 5. CHECK EXISTING PATTERNS

Before creating anything new:
1. Check if similar code exists
2. Follow the existing pattern EXACTLY
3. Don't create new patterns without discussion

### 6. CONSTANTS FIRST

When implementing a feature:
1. Define all constants FIRST
2. Create types and interfaces
3. THEN implement logic

Never write implementation with string literals planning to extract constants later.

## ENFORCEMENT CHECKLIST

Before submitting ANY code, verify:

- [ ] Zero magic strings (all strings are constants)
- [ ] Handlers only map data (no business logic)
- [ ] Orchestrators contain all business logic
- [ ] External clients only translate APIs
- [ ] Following existing patterns exactly
- [ ] No "temporary" solutions
- [ ] All types from toolkit/constants are used

## RED FLAGS TO CATCH

If you find yourself:
- Writing `"some_string"` instead of a constant
- Adding logic to a handler beyond simple mapping
- Saying "we'll refactor this later"
- Creating new patterns different from existing code
- Making handlers more than 50 lines for a single method
- Using `string` type for things that should be constants

STOP IMMEDIATELY and reconsider the approach.

## Example Violations I've Made

1. ❌ Using `"equipment"` instead of `shared.ChoiceEquipment`
2. ❌ Adding nested choice logic to handler
3. ❌ Using `Type: "single_item"` instead of creating constants
4. ❌ Complex transformation in handler (100+ lines of equipment choice logic)
5. ❌ Saying "we'll move this to orchestrator later"

These should NEVER happen.