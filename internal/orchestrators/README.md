# Orchestrators

Business logic implementations that coordinate between repositories, engine (rpg-toolkit), and external services.

## Scope & Responsibilities

**✅ What Orchestrators Do:**
- Implement service interfaces from `/services/`
- Coordinate multiple repositories and external dependencies
- Apply business rules and validation logic
- Handle complex workflows and state transitions
- Manage transactions and error handling
- Transform data between layers

**❌ What Orchestrators Don't Do:**
- Direct data storage (delegate to repositories)
- Game mechanics calculations (delegate to engine/rpg-toolkit)
- Protocol/transport concerns (delegate to handlers)
- Raw external API calls (delegate to clients)

## Package Structure

```
/orchestrators/
  └── character/
      ├── orchestrator.go     # Main implementation
      ├── orchestrator_test.go
      └── README.md          # Package-specific scope
```

## Dependencies

Orchestrators depend on interfaces only:
- **Repositories**: Storage operations
- **Engine**: Game mechanics via rpg-toolkit
- **External Clients**: Third-party API integration
- **Services**: Other domain orchestrators (when needed)

## Testing Approach

- Use `testify.Suite` for organization
- Mock all dependencies (repositories, engine, clients)
- Test business logic and workflows
- Test error handling and validation
- Test complex coordination scenarios

## Design Principles

1. **Interface Dependencies**: Never depend on concrete implementations
2. **Single Responsibility**: One orchestrator per business domain
3. **Input/Output Types**: Always use structured types
4. **Error Context**: Wrap errors with meaningful context
5. **Testability**: Design for easy mocking and testing

## Current Status

- [ ] Character orchestrator (Issue #4)
- [ ] Session orchestrator
- [ ] Encounter orchestrator