# 🚨 CRITICAL BLOCKER: Proto Dependency Architecture Issue

**Status**: BLOCKING PR 1/7 of Sandbox Service Implementation  
**Severity**: Critical - Prevents compilation and CI success  
**Date**: 2025-08-28  

## Problem Summary

The sandbox room service implementation cannot compile because required proto packages don't exist in any published/tagged version of `rpg-api-protos`. This reveals a fundamental architectural dependency management issue in our multi-repository setup.

## Technical Details

### What We Need
```go
// These imports are required for the sandbox service:
import (
    roomcommon "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha1"           // ✅ EXISTS  
    sandboxapi "github.com/KirkDiggler/rpg-api-protos/gen/go/sandbox/api/v1alpha1"   // ❌ MISSING
)

// Required types from sandbox namespace:
- *sandboxapi.GenerativeRoomConfig  
- *sandboxapi.StaticRoomConfig
- *sandboxapi.GenerationMetrics
- *sandboxapi.EntitySize
```

### Current State Analysis
1. **Latest Tag (v0.1.33)**: Does NOT contain sandbox protos
2. **Latest Commit**: Contains sandbox protos but isn't tagged
3. **Sandbox Protos**: Exist on `feat/sandbox-namespace-protos` branch but unmerged
4. **Go Module Behavior**: `go mod tidy` tries to resolve to latest **tag**, not latest commit

### Compilation Errors
```
go: github.com/KirkDiggler/rpg-api/internal/services/sandbox_room imports
    github.com/KirkDiggler/rpg-api-protos/gen/go/sandbox/api/v1alpha1: 
    module github.com/KirkDiggler/rpg-api-protos@latest found (v0.1.33), 
    but does not contain package github.com/KirkDiggler/rpg-api-protos/gen/go/sandbox/api/v1alpha1

undefined: roomcommon.EntityPlacement
```

## Root Cause Analysis

### 1. Incomplete Proto Development Workflow
- Sandbox protos were created on feature branch
- Generated code exists in commit but not in any tag
- No clear process for proto releases in multi-repo setup

### 2. Go Module Resolution Conflict  
- `@latest` resolves to latest semantic version tag (v0.1.33)
- Required protos exist in commit `96e6eb6` (after v0.1.33)
- Creates impossible dependency: need newer than latest tagged version

### 3. Fragmented Proto State
- Proto definitions exist in source form
- Generated code scattered across branches/commits  
- No single source of truth for "what protos are available"

## Impact Assessment

### Immediate Impact
- ❌ PR 1/7 cannot compile
- ❌ CI/CD pipeline fails
- ❌ All 7 planned PRs blocked
- ❌ Development velocity completely stopped

### Systemic Risk
- Other services requiring new protos will hit same issue
- No clear upgrade path for proto dependencies
- Unclear ownership of proto release management
- Potential for dependency hell in multi-service architecture

## Required Solutions (Priority Order)

### 🔥 CRITICAL: Immediate Fix Required
**Option A: Emergency Tag** (Recommended - Fastest)
1. Merge `feat/sandbox-namespace-protos` to main in rpg-api-protos
2. Generate all code (Go, C++, TypeScript) 
3. Tag as v0.1.34 immediately
4. Update rpg-api to use v0.1.34
5. ✅ Unblocks development TODAY

**Option B: Temporary Workaround** (If Option A blocked)
1. Replace sandbox proto types with `interface{}` temporarily
2. Add TODO comments for proper types  
3. Accept technical debt for initial PRs
4. Fix properly when protos available

### 🔧 ARCHITECTURAL: Long-term Fix Required  
**Establish Proto Release Workflow**
1. Define clear proto versioning strategy
2. Establish proto release ownership (who can tag/release)
3. Create automated proto generation + tagging on merge to main
4. Document proto dependency upgrade process for all repositories

## Decision Matrix

| Option | Speed | Risk | Technical Debt | Blocks Other Work |
|--------|-------|------|----------------|-------------------|
| Emergency Tag | ⚡ Fast | 🟡 Medium | None | No |
| Workaround | ⚡ Fast | 🔴 High | Significant | No |
| Full Fix | 🐌 Slow | 🟢 Low | None | Yes |

## Recommended Action Plan

1. **TODAY**: Execute Option A (Emergency Tag)
2. **This Week**: Define proto release workflow  
3. **Next Sprint**: Implement automated proto release pipeline

## Blockers to Resolution

- [ ] Who has permissions to merge/tag rpg-api-protos?
- [ ] Are there other unmerged proto changes that should be included?
- [ ] What version number should be used (v0.1.34, v0.2.0, etc.)?
- [ ] Should all generated languages be updated or just Go?

## Contact & Escalation

This blocks the entire sandbox service implementation (7 PRs).  
**Needs immediate attention from project owner.**

---
*Document created: 2025-08-28*  
*Last updated: 2025-08-28*