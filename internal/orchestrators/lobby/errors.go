package lobby

import "errors"

// Orchestrator-level sentinel errors. These are entity-layer classifications;
// the thin handler maps them onto gRPC status codes (mirrors the v2 encounter
// orchestrator's pat-v2-status-code-mapping — this package never imports
// proto or grpc/status, so it must surface these as typed sentinels).
var (
	// ErrLobbyNotFound means the lobby id (or join_ref) has no stored data.
	// Handler maps to codes.NotFound.
	ErrLobbyNotFound = errors.New("lobby not found")

	// ErrPlayerNotInLobby means the authenticated player is not a member of
	// the lobby. Handler maps to codes.PermissionDenied.
	ErrPlayerNotInLobby = errors.New("player is not a member of this lobby")

	// ErrLobbyAlreadyStarted means the lobby's WAITING -> STARTED transition
	// has already fired (terminal, lobby-surface.md "Lifecycle"). Returned by
	// JoinLobby (late join), SetReady, LeaveLobby, and a repeat StartEncounter.
	// Handler maps to codes.FailedPrecondition.
	ErrLobbyAlreadyStarted = errors.New("lobby has already started")

	// ErrLobbyFull means JoinLobby would push membership past the configured
	// party cap (lobby-surface.md "Party cap"). Handler maps to
	// codes.FailedPrecondition.
	ErrLobbyFull = errors.New("lobby is full")

	// ErrNotHost means a non-host player called StartEncounter.
	// Handler maps to codes.PermissionDenied.
	ErrNotHost = errors.New("only the host can start the encounter")

	// ErrNotAllReady means StartEncounter was called before every member set
	// ready. Handler maps to codes.FailedPrecondition.
	ErrNotAllReady = errors.New("not all members are ready")

	// ErrCharacterNotFound means the character_id supplied to CreateLobby /
	// JoinLobby does not resolve in the character store. Handler maps to
	// codes.NotFound.
	ErrCharacterNotFound = errors.New("character not found")

	// ErrCharacterOwnershipMismatch means the supplied character_id belongs to
	// a different player than the authenticated caller (lobby-surface.md:
	// "the server validates the character belongs to the authenticated
	// player — the v1 lobby never validated this"). Handler maps to
	// codes.PermissionDenied.
	ErrCharacterOwnershipMismatch = errors.New("character does not belong to the authenticated player")

	// ErrLobbyNotStarted means AbandonEncounter was called against a lobby
	// that never reached STARTED (WAITING, or STARTED with no EncounterID —
	// the latter would be a StartEncounter bug, treated identically since the
	// caller-visible outcome is the same: nothing to abandon). Handler maps
	// to codes.FailedPrecondition.
	ErrLobbyNotStarted = errors.New("lobby has not started an encounter")

	// ErrEncounterAlreadyEnded means AbandonEncounter's target encounter has
	// already reached a terminal state — naturally (victory/TPK) or via a
	// prior AbandonEncounter call — before this call ran. Kept distinct from
	// ErrLobbyNotStarted (which means no encounter was ever started) even
	// though both map to the same gRPC code: the caller-visible outcome
	// ("nothing to abandon") is identical, but the message tells a more
	// honest story about which case actually happened. Handler maps to
	// codes.FailedPrecondition.
	ErrEncounterAlreadyEnded = errors.New("encounter has already ended")
)
