# RPG API Client Commands

These commands allow you to test the RPG API by making real gRPC requests to the server.

## Prerequisites

1. Start the gRPC server:
```bash
./server server
```

2. The server runs on `localhost:50051` by default.

## Available Commands

### List Commands

**List all races:**
```bash
./server client list-races
./server client list-races --include-subraces=false  # Exclude subraces
```

**List all classes:**
```bash
./server client list-classes
./server client list-classes --spellcasters-only     # Only show spellcasting classes
./server client list-classes --include-features=false # Hide class features
```

**List all backgrounds:**
```bash
./server client list-backgrounds
```

**List equipment by type:**
```bash
./server client list-equipment --type=simple-melee-weapons
./server client list-equipment --type=martial-melee-weapons
./server client list-equipment --type=light-armor
./server client list-equipment --type=heavy-armor
./server client list-equipment --type=shields
./server client list-equipment --type=adventuring-gear
./server client list-equipment --type=simple-melee-weapons --page-size=10
```

**List spells by level:**
```bash
./server client list-spells --level=0                    # Cantrips
./server client list-spells --level=1                    # 1st level spells
./server client list-spells --level=3 --class=wizard     # 3rd level wizard spells
./server client list-spells --level=9 --class=sorcerer   # 9th level sorcerer spells
./server client list-spells --level=2 --page-size=5      # 2nd level spells, 5 per page
```

### Get Commands

**Get specific race details:**
```bash
./server client get-race dwarf
./server client get-race half-orc
./server client get-race elf
```

**Get specific class details:**
```bash
./server client get-class wizard
./server client get-class fighter
./server client get-class rogue
```

## Connection Options

All commands support these flags:

- `--server`: gRPC server address (default: `localhost:50051`)
- `--timeout`: Request timeout (default: `30s`)

Example with custom server:
```bash
./server client list-races --server=api.example.com:50051 --timeout=10s
```

## Example Output

### List Races
```
🎭 Dwarf (ID: dwarf)
   Speed: 25 ft
   Size: Medium
   Ability Bonuses:
     - constitution: +2
   Traits:
     - Darkvision
     - Dwarven Resilience
     - Stonecunning
   Subraces:
     - Hill Dwarf (wisdom +1)
     - Mountain Dwarf (strength +2)
```

### Get Class
```
⚔️  Wizard (ID: wizard)

Basic Info:
  Hit Die: 1d6
  Primary Abilities: Intelligence

Proficiencies:
  Armor: None
  Weapons: Daggers, darts, slings, quarterstaffs, light crossbows
  Saving Throws: Intelligence, Wisdom

Skills: Choose 2 from:
  - Arcana
  - History
  - Insight
  - Investigation
  - Medicine
  - Religion

🔮 Spellcasting:
  Spellcasting Ability: Intelligence
  Spellcasting Focus: Arcane focus
  Ritual Casting: Yes
  Cantrips Known at 1st Level: 3
  1st Level Spell Slots: 2
```

### List Equipment
```
⚔️  Dagger (ID: dagger)
   Category: simple-melee-weapons
   Cost: 2 gp
   Weight: 1 lbs
   🗡️  Weapon Properties:
     - Category: Simple
     - Range: Melee
     - Damage: 1d4 piercing
     - Properties: finesse, light, thrown

⚔️  Shortsword (ID: shortsword)
   Category: martial-melee-weapons
   Cost: 10 gp
   Weight: 2 lbs
   🗡️  Weapon Properties:
     - Category: Martial
     - Range: Melee
     - Damage: 1d6 piercing
     - Properties: finesse, light
```

### List Spells
```
✨ Fire Bolt (ID: fire-bolt)
   Level: 0 (cantrip)
   School: Evocation
   Casting Time: 1 action
   Range: 120 feet
   Components: V, S
   Duration: Instantaneous
   Classes: sorcerer, wizard
   Description: You hurl a mote of fire at a creature or object within range...

✨ Magic Missile (ID: magic-missile)
   Level: 1
   School: Evocation
   Casting Time: 1 action
   Range: 120 feet
   Components: V, S
   Duration: Instantaneous
   Classes: sorcerer, wizard
   Description: You create three glowing darts of magical force...
```

## Testing the External Client

These commands are perfect for testing that the external client integration is working correctly:

1. **Check basic connectivity:**
   ```bash
   ./server client list-races
   ```

2. **Verify caching is working** - Run the same command twice. The second call should be faster:
   ```bash
   time ./server client list-races
   time ./server client list-races  # Should be faster
   ```

3. **Test concurrent loading** - The list commands load all details concurrently:
   ```bash
   ./server client list-classes --include-features
   ```

4. **Test specific lookups:**
   ```bash
   ./server client get-race tiefling
   ./server client get-class paladin
   ```

5. **Test equipment and spell filtering:**
   ```bash
   ./server client list-equipment --type=simple-melee-weapons
   ./server client list-spells --level=0 --class=wizard
   ```

6. **Test pagination:**
   ```bash
   ./server client list-equipment --type=adventuring-gear --page-size=5
   ./server client list-spells --level=1 --page-size=3
   ```
