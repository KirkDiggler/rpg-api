# Module Registry Architecture for RPG Toolkit

## Overview

This document outlines the theoretical architecture for a module/registry system that would allow extensible content like the Artificer class to be added to the RPG Toolkit without modifying core code.

## Problem Statement

D&D 5e is a living game with regular expansions including:
- New classes (Artificer)
- New subclasses (Echo Knight, Aberrant Mind Sorcerer)  
- New races (Fairy, Harengon, Autognome)
- New spells with complex mechanical implementations
- Setting-specific content (Eberron, Ravnica, Spelljammer)
- Third-party content (Critical Role, Kobold Press)
- Homebrew content

Current limitations:
- Cannot add new content without modifying core toolkit
- No way to package related content together (e.g., Artificer class + infusions + spells)
- No mechanism for content to include behavior (spell implementations)
- Community content requires forking

## Proposed Architecture

### 1. Registry System

Create a central registry that can dynamically register content at runtime:

```go
// registry/registry.go
type Registry struct {
    classes map[string]*ClassEntry
    spells  map[string]*SpellEntry
    races   map[string]*RaceEntry
    feats   map[string]*FeatEntry
}

type ClassEntry struct {
    Data      *ClassData
    OnLevelUp func(char Character, level int) error
    Resources func(char Character) map[string]Resource
}

type SpellEntry struct {
    Data     *SpellData
    Cast     func(caster Character, targets []Target, slot int) (*SpellResult, error)
    Validate func(caster Character, targets []Target) error
}
```

### 2. Module Interface

Define a standard interface for content modules:

```go
// modules/module.go
type Module interface {
    ID() string
    Name() string
    Version() string
    MinToolkitVersion() string
    
    Register(r *Registry) error
}
```

### 3. Reference System

Use module references to identify content from different sources:

```
Format: module:type:value
Examples:
- "core:class:fighter"        # Core Fighter class
- "artificer:class:artificer" # Artificer from module
- "xanathar:spell:healing_spirit" # Spell from Xanathar's module
```

### 4. Module Loading in rpg-api

The game server (rpg-api) would be responsible for:

1. **Module Discovery**: Finding and loading available modules
2. **Dependency Resolution**: Ensuring module compatibility
3. **Content Registration**: Registering module content with the registry
4. **Reference Resolution**: Resolving module references when loading characters

```go
// In rpg-api orchestrator
func (o *Orchestrator) LoadModules() error {
    // Load core content
    core.Register(o.registry)
    
    // Load configured modules
    for _, modPath := range o.config.Modules {
        mod, err := LoadModule(modPath)
        if err != nil {
            return err
        }
        
        if err := mod.Register(o.registry); err != nil {
            return err
        }
    }
    
    return nil
}
```

## Example: Artificer Module

Here's how an Artificer module might look:

```go
// github.com/KirkDiggler/rpg-toolkit-artificer
package artificer

import "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/registry"

// Exported "constants" for type safety
var (
    ClassArtificer = registry.ClassID("artificer")
    SpellArcaneWeapon = registry.SpellID("arcane_weapon")
)

func init() {
    registry.RegisterModule(&ArtificerModule{})
}

type ArtificerModule struct{}

func (m *ArtificerModule) ID() string { return "artificer" }
func (m *ArtificerModule) Name() string { return "Artificer Class" }
func (m *ArtificerModule) Version() string { return "1.0.0" }
func (m *ArtificerModule) MinToolkitVersion() string { return "0.2.0" }

func (m *ArtificerModule) Register(r *registry.Registry) error {
    // Register class with behavior
    r.RegisterClass(string(ClassArtificer), &registry.ClassEntry{
        Data:      getArtificerData(),
        OnLevelUp: artificerLevelUp,
        Resources: artificerResources,
    })
    
    // Register spells with implementations
    r.RegisterSpell(string(SpellArcaneWeapon), &registry.SpellEntry{
        Data: getArcaneWeaponData(),
        Cast: castArcaneWeapon,
    })
    
    // Register infusions as features
    for _, infusion := range getInfusions() {
        r.RegisterFeature(infusion.ID, infusion)
    }
    
    return nil
}

// Spell implementation
func castArcaneWeapon(caster Character, targets []Target, slot int) (*SpellResult, error) {
    weapon := targets[0].(*Weapon)
    return &SpellResult{
        Effects: []Effect{{
            Type:     "enhancement",
            Target:   weapon,
            Value:    1,
            Duration: Duration{Amount: 1, Unit: "hour"},
        }},
    }, nil
}
```

## Benefits

1. **No Breaking Changes**: Existing code continues to work with core content
2. **Type Safety**: Module exports provide compile-time checking where possible
3. **Extensibility**: New content can be added without modifying toolkit
4. **Behavior Included**: Spells and abilities have actual implementations
5. **Modular**: Users pick only the content they need
6. **Community Friendly**: Easy to create and share modules
7. **Version Independent**: Modules can update separately from toolkit

## Trade-offs

1. **Runtime Registration**: Extended content validated at startup, not compile time
2. **Dependency Management**: Need to ensure module compatibility
3. **Potential Conflicts**: Two modules could register same content
4. **Slight Complexity**: Two ways to reference content (constant vs registry)

## Implementation Considerations

### For rpg-api

1. **Module Configuration**: Add module list to server config
2. **Storage**: Store module references with character data
3. **API Updates**: Include module info in proto definitions
4. **Validation**: Validate module content on load

### For rpg-toolkit

1. **Registry Infrastructure**: Create registry package
2. **Module Interface**: Define standard module interface
3. **Core Registration**: Register all existing content
4. **Backward Compatibility**: Ensure existing code works unchanged

### For Module Authors

1. **Module Template**: Provide starter template
2. **Testing Framework**: Tools for testing modules
3. **Documentation**: Clear guidelines for module creation
4. **Distribution**: Go modules for easy distribution

## Future Enhancements

1. **Module Marketplace**: Central registry of available modules
2. **Dependency Resolution**: Automatic module dependency handling
3. **Hot Reloading**: Load modules without server restart
4. **Conflict Resolution**: Strategies for handling content conflicts
5. **Module Composition**: Modules that extend other modules

## Conclusion

This module/registry architecture would allow the RPG Toolkit ecosystem to grow organically through community contributions while maintaining the stability and type safety of the core system. The key is keeping the core toolkit focused on infrastructure while allowing modules to provide the actual game content and mechanics.
