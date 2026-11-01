# Reference Keep

`reference-keep` is a large layout and placement reference with a small gatehouse dialogue proof of concept. It is a starting point for authors, not a completed adventure.

## Use your own copy

With an API deployment containing this file, open the ordinary dungeon/studio reference list and select **Reference Keep — Layout and Gatehouse POC**. Save under a new key to create your own editable copy. Shipped references are read-only; editing your copy does not change the reference or another author's copy.

The shipped file is `content/reference-keep.yaml`. Its native v5 document includes the existing connected floor, structural walls, doors, named areas, and placement markers. No separate local file or private content mount is required to compile it.

## What is present

- A 28,934-hex footprint across 28 named areas, including exterior grounds and connected interiors.
- 216 markers labeled `OBSTACLE`, `MOVE`, `NPC`, `AREA` and similar planning labels. They preserve the intended circulation and placement plan.
- Four neutral gate guards. The first offers entertainment (Performance DC 11) and invited guests (Deception DC 11).
- Successful checks give an admission reply and record `gate-admitted`; failed checks give a refusal reply and record `gate-first-warning`.
- Ordinary placeholder guard combat loadouts, not the intended special lieutenant kit.

Markers are annotations, not placed furniture, blocking volumes, spawned NPCs or scripted activities. Most rooms remain a layout to furnish and build upon. Asset references use the normal game asset pipeline; this reference does not bundle model or texture bytes.

## Proof-of-concept limits

The current dialogue does **not** enforce admission, limit attempts, arrest or relocate the party, move surviving guards to the ballroom, run shared voting, or supply the full branching conversation. Selecting Cancel exits dialogue; ordinary combat controls remain separate. A successful reply is not an implemented entrance restriction. The missing entrance sequence is tracked in [rpg-project#769](https://github.com/KirkDiggler/rpg-project/issues/769).

This reference deliberately preserves the implemented Performance/Deception choices. The later Performance/Persuasion entrance requirement in #769 is not silently substituted into this POC.

This is a large reference. Compiling a valid document does not certify populated multiplayer performance. The related scale work is tracked in [rpg-project#753](https://github.com/KirkDiggler/rpg-project/issues/753) and [#766](https://github.com/KirkDiggler/rpg-project/issues/766).

Publication scope and verification: [rpg-project#789](https://github.com/KirkDiggler/rpg-project/issues/789). Private story documents, session data and personal worldbuilding are not part of this reference.
