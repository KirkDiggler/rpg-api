package auth

import "errors"

var (
	// ErrMissingToken is returned when no authorization header is present.
	ErrMissingToken = errors.New("missing authorization token")

	// ErrInvalidTokenFormat is returned when the authorization header format is wrong.
	ErrInvalidTokenFormat = errors.New("invalid token format: expected 'Discord <token>'")

	// ErrInvalidToken is returned when Discord rejects the token (401).
	ErrInvalidToken = errors.New("invalid Discord token")

	// ErrDiscordUnavailable is returned when Discord cannot provide a usable response.
	ErrDiscordUnavailable = errors.New("discord API unavailable")

	// ErrGuildMembershipDenied is returned when Discord reports no access to the selected guild.
	ErrGuildMembershipDenied = errors.New("discord guild membership denied")
)
