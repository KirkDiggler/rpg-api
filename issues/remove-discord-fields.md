# Remove Discord-specific fields from CharacterDraft entity

## Problem
The `CharacterDraft` entity in `internal/entities/dnd5e/character_draft.go` contains Discord-specific fields:
- `DiscordChannelID`
- `DiscordMessageID`

These fields violate the separation of concerns - the core RPG API should not know about Discord or any other specific client implementation.

## Solution
1. Remove these fields from the `CharacterDraft` entity
2. If Discord needs to track this information, it should maintain its own mapping:
   - Discord bot can store `draftID -> Discord metadata` in its own storage
   - Or include Discord metadata in a client-specific context/session

## Impact
- This is a breaking change for any clients using these fields
- Will need to update the protobuf definitions if they include these fields
- Discord bot will need to be updated to handle this separately

## Benefits
- Clean separation of concerns
- API remains client-agnostic
- Other clients (web, mobile, CLI) won't have Discord-specific fields