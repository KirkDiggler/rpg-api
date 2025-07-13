# Proto API Definitions

This directory contains the protocol buffer definitions for rpg-api's gRPC services.

## Structure

```
proto/
└── v1alpha1/           # Alpha version of v1 API
    └── dnd5e/          # D&D 5th Edition specific APIs
        ├── character.proto  # Character management
        ├── enums.proto     # D&D 5e enumerations
        └── common.proto    # Shared message types
```

## Versioning Strategy

We follow the Kubernetes-style versioning approach:
- `v1alpha1` - Initial development version, breaking changes allowed
- `v1beta1` - More stable, breaking changes discouraged
- `v1` - Stable release, no breaking changes

## Using Buf

We use [Buf](https://buf.build) for proto management:

```bash
# Lint proto files
make buf-lint

# Generate Go code
make buf-generate

# Check for breaking changes
make buf-breaking
```

## API Design Guidelines

1. **Use enums over strings** for known values (races, classes, etc.)
2. **Explicit is better** - D&D 5e specific types rather than generic
3. **Input/Output pattern** - Separate request/response messages
4. **Field comments** - Document constraints and valid ranges
5. **HTTP annotations** - Support REST gateway if needed

## D&D 5e API

The `dnd5e` package contains APIs specific to Dungeons & Dragons 5th Edition:

### CharacterAPI
- `CreateCharacter` - Create a new character with validation
- `GetCharacter` - Retrieve a character by ID
- `ListCharacters` - List characters with filtering
- `UpdateCharacter` - Update character fields
- `DeleteCharacter` - Remove a character

### Future APIs
- `SessionAPI` - Game session management
- `CombatAPI` - Combat and initiative tracking
- `DiceAPI` - Dice rolling with modifiers
- `InventoryAPI` - Equipment and item management

## Adding New Rulesets

To add support for a new ruleset (e.g., Pathfinder):

1. Create a new directory: `v1alpha1/pathfinder/`
2. Define ruleset-specific enums and messages
3. Keep the service patterns consistent with D&D 5e
4. Update this README with the new APIs
