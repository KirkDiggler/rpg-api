# Spare the Dying stabilization API adoption (#982)

The API consumes published toolkit session v0.86.1, root v0.168.0,
resolution v0.48.0, and encounter v0.82.0. The session tag is exactly toolkit
PR #1743 merge `18d5b6e87c1d70f3b645519a5cc28014d4129638`.
Proto SDK v0.1.190 includes proto PR #334; the Go submodule pin is
`v0.0.0-20260913202226-efb300b87555`. No newer existing pins were downgraded.

`internal/handlers/dnd5e/session/v1alpha1/convert.go` maps
`ActivationResultBody.Stabilized` to `ActivationResult.stabilized` (oneof arm 8).
It copies target, source ref/name, before/after life states, unchanged HP, and
all six provider progress fields. Progress stays present even for all-zero
values; remaining counts and false flags are copied, never inferred.
The existing one-result invariant still rejects contradictory result bodies.
`StreamEvents` and `GetStory` share this converter. CastResponse is unchanged.

API integration evidence is in
`internal/integration/session/stabilization_acceptance_test.go`: a Cleric casts
through the public handler against a dying or already-stable player character.
Explicit fixture setup injures a participant after initiative and supplies the
known cantrip; this is cast acceptance, not a new native-creation claim.
The test verifies action payment, unchanged spell slots, zero dice calls,
zero HP, cleared successes/failures, stable turn advancement, and identical
live/rehydrated Story events separately for caster and recipient.

Owner-private CharacterData already maps HP (field 10), life state (15), and
death-save progress (16) from the strict provider StatusView in
`internal/handlers/dnd5e/v2/character/character_data.go`. Refresh after the cast
is verified through GetCharacterData. Unauthenticated calls fail; another
player receives NotFound, preserving the existing existence-hiding boundary.
No private fields or HP-based life-state inference were added.

## Browser follow-on after the API is available

1. Consume published SDK v0.1.190 and handle `activation_result.stabilized`
   in both live and catch-up paths, in delivered order and recipient-local sequence.
2. Render the provider's target/source and stabilized state at zero HP. This
   result has no healing, revival or dice animation. Before and after can both
   be stabilized; do not discard that valid result as a no-op.
3. Copy all progress fields, including cleared zero counters and false dead.
   Refresh the authenticated owner's sheet through its existing endpoint.
4. Verify an actual browser cast, action cost without a slot or die, dying and
   already-stable recipients, reconnect narration and turn advancement. API
   integration tests do not establish browser or deployment acceptance.

Preparation, monster stabilization and timed natural recovery remain deferred.
