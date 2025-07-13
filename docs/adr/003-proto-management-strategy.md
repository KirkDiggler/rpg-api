# ADR-003: Proto Management Strategy

Date: 2025-01-13

## Status

Accepted

## Context

We're building a multi-service, multi-rulebook RPG API system that will need:
- TypeScript and Go SDKs
- Multiple services (character, session, combat, etc.)
- Multiple rulebooks (D&D 5e, Pathfinder, etc.)
- Clean API versioning
- Fast development iteration

## Decision

### Short-term (Current)
- Keep protos in rpg-api repo under `/api/proto/`
- Generate to `/gen/go/` with optimization to skip if unchanged
- Commit generated files for now (controversial but practical)

### Mid-term (3-6 months)
- Create `rpg-api-protos` repository for proto source files
- Use Buf Schema Registry (BSR) for distribution
- Generate TypeScript and Go SDKs on-demand
- Remove generated files from service repos

### Long-term (6-12 months)
- Evaluate if we need OpenAPI support (consider Smithy)
- Potentially separate SDK repos per language
- Consider gRPC-Web for browser support

## Consequences

### Positive
- Clean separation of API contracts from implementation
- Multi-language SDK support without manual work
- Breaking change detection across all consumers
- Automatic API documentation
- Near-instant publishing (<5 seconds from merge to available)

### Negative
- Additional complexity in development workflow
- Another repository to maintain
- BSR requires internet connection (or self-hosting)

## Implementation Plan

### Phase 1: Optimize Current Setup ✅
```makefile
# Only regenerate if protos changed
buf-generate:
	@if [ protos are newer ]; then generate; fi
```

### Phase 2: Create Proto Repository
```
rpg-api-protos/
├── buf.yaml
├── buf.md              # API documentation
├── dnd5e/
│   └── v1alpha1/
│       ├── character.proto
│       └── session.proto
└── .github/
    └── workflows/
        └── publish.yml  # Auto-publish to BSR
```

### Phase 3: BSR Integration
```yaml
# buf.yaml in rpg-api-protos
version: v1
name: buf.build/kirkdiggler/rpg-api
deps:
  - buf.build/googleapis/googleapis  # For common types

# GitHub Action
- name: Push to BSR
  run: buf push --tag ${{ github.sha }}
```

### Phase 4: Consumer Configuration
```yaml
# buf.gen.yaml in rpg-api
version: v1
deps:
  - buf.build/kirkdiggler/rpg-api
plugins:
  - plugin: go
    out: gen/go
    opt: paths=source_relative
  - plugin: connect-es  # For TypeScript
    out: gen/ts
```

## SDK Generation Examples

### TypeScript
```typescript
// Auto-generated from BSR
import { CharacterService } from "@kirkdiggler/rpg-api/dnd5e/v1alpha1";
import { createGrpcWebTransport } from "@connectrpc/connect-web";

const client = new CharacterService(
  createGrpcWebTransport({
    baseUrl: "https://api.rpg.example"
  })
);
```

### Go
```go
// Auto-generated from BSR
import "buf.build/gen/go/kirkdiggler/rpg-api/dnd5e/v1alpha1"
```

## Timeline

1. **Now**: Optimize current generation (done)
2. **Month 1-2**: Set up rpg-api-protos repo
3. **Month 2-3**: Integrate BSR, test publishing
4. **Month 3-4**: Migrate consumers to use BSR
5. **Month 4-6**: Add TypeScript SDK generation