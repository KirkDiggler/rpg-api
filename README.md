# rpg-api

> **Real-time API gateway for tabletop RPG sessions, enabling consistent game experiences across any interface while leveraging rpg-toolkit as the core game engine.**

## Vision

rpg-api serves as the orchestration layer between game engines and user interfaces, making tabletop RPGs accessible anywhere - Discord, web browser, mobile app, or even CLI. By separating game mechanics (via rpg-toolkit) from presentation, we enable rich, real-time multiplayer experiences without duplicating logic across platforms.

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   UI Clients    │────▶│    rpg-api      │────▶│   rpg-toolkit   │
│ (Discord, Web)  │ API │  (Orchestrator) │uses │  (Game Engine)  │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

### Core Insight

**rpg-api manages data. rpg-toolkit manages rules.**

- We store simple data models (character stats, session state)
- rpg-toolkit calculates everything (proficiency bonus, attack rolls, spell effects)
- This separation allows multiple rulesets to share the same API

### Core Responsibilities

- **Session Management**: Create and manage multiplayer game sessions
- **Real-time Updates**: Stream game state changes to all connected clients
- **API Gateway**: Provide consistent gRPC API for all client types
- **State Persistence**: Save and restore game sessions
- **Rule Orchestration**: Coordinate between UI actions and game engine

### What This Is NOT

- **Not a game engine**: rpg-toolkit handles mechanics, rules, dice
- **Not a UI**: Clients handle presentation and user interaction
- **Not ruleset-specific**: Built to support any tabletop RPG system

## Getting Started

### Prerequisites

- Go 1.21+
- Redis 7+ (initial storage implementation)
- protoc 3.x with Go plugins
- Storage backend of your choice (via repository adapters)

### Development Setup

```bash
# Clone the repository
git clone https://github.com/KirkDiggler/rpg-api.git
cd rpg-api

# Install dependencies
go mod download

# Run tests
make test

# Run the server
make run
```

### Pre-commit Workflow

**Always** run before committing:
```bash
make pre-commit
```

This ensures:
- Code is formatted (`go fmt`)
- Dependencies are tidy (`go mod tidy`)
- Linting passes
- Tests pass

## Project Structure

```
rpg-api/
├── api/proto/v1alpha1/ # gRPC API definitions
├── cmd/server/         # Application entrypoint
├── internal/           # Private application code
│   ├── entities/       # Simple domain models
│   ├── handlers/       # gRPC handlers by version
│   ├── orchestrators/  # Business logic flows
│   ├── repositories/   # Storage interfaces
│   └── engine/         # rpg-toolkit integration
└── docs/               # Architecture decisions
```

Each package includes a README explaining its purpose and boundaries.

## Documentation

- **[Architecture Decision Records](docs/adr/)**: Understand why we built it this way
- **[Journey Documents](docs/journey/)**: Learn from our exploration and challenges
- **[API Documentation](docs/api/)**: Generated from protobuf definitions

## Design Principles

1. **API-First**: Every feature starts with API design
2. **Explicit Inputs/Outputs**: Every function uses structured types
3. **Data vs Rules**: We store data, rpg-toolkit handles rules
4. **Interface Boundaries**: Clean separation between layers
5. **Test Everything**: High coverage with real dependencies when safe
6. **Document the Journey**: ADRs, journey docs, and clear READMEs

## Related Projects

- [rpg-toolkit](https://github.com/yourusername/rpg-toolkit): Core game engine
- [dnd-bot-discord](https://github.com/KirkDiggler/dnd-bot-discord): Discord-specific renderer

## License

MIT License - see [LICENSE](LICENSE) file for details