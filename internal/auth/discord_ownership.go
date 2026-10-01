package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const guildListPageSize = 200

// GetGuildOwnershipInput selects a guild using the authenticated user's token.
type GetGuildOwnershipInput struct {
	Token   string
	GuildID string
}
type GetGuildOwnershipOutput struct{ Owner bool }

// OwnershipVerifier verifies actual Discord ownership, not Discord admin permissions.
type OwnershipVerifier interface {
	GetGuildOwnership(context.Context, *GetGuildOwnershipInput) (*GetGuildOwnershipOutput, error)
}

type discordGuild struct {
	ID    string `json:"id"`
	Owner bool   `json:"owner"`
}

// GetGuildOwnership uses the OAuth guilds scope and paginates all user guilds.
// A missing guild is denied; provider failures never become an owner grant.
func (c *DiscordClient) GetGuildOwnership(ctx context.Context, input *GetGuildOwnershipInput) (*GetGuildOwnershipOutput, error) {
	if input == nil || input.Token == "" || !isCanonicalUint64(input.GuildID) {
		return nil, fmt.Errorf("%w: ownership input is required", ErrDiscordUnavailable)
	}
	after := ""
	for {
		values := url.Values{"limit": {"200"}}
		if after != "" {
			values.Set("after", after)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/users/@me/guilds?"+values.Encode(), http.NoBody)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrDiscordUnavailable, err)
		}
		req.Header.Set("Authorization", "Bearer "+input.Token)
		guilds, err := c.readGuildPage(req)
		if err != nil {
			return nil, err
		}
		for _, guild := range guilds {
			if !isCanonicalUint64(guild.ID) {
				return nil, fmt.Errorf("%w: unusable guild identity", ErrDiscordUnavailable)
			}
			if guild.ID == input.GuildID {
				return &GetGuildOwnershipOutput{Owner: guild.Owner}, nil
			}
		}
		if len(guilds) < guildListPageSize {
			return nil, ErrGuildMembershipDenied
		}
		next := guilds[len(guilds)-1].ID
		if len(next) < len(after) || (len(next) == len(after) && next <= after) {
			return nil, fmt.Errorf("%w: guild pagination did not advance", ErrDiscordUnavailable)
		}
		after = next
	}
}

func (c *DiscordClient) readGuildPage(req *http.Request) ([]discordGuild, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDiscordUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, ErrInvalidToken
	case http.StatusForbidden:
		// Missing OAuth scope is not evidence that the user is a non-owner.
		return nil, fmt.Errorf("%w: Discord guild ownership scope is unavailable", ErrDiscordUnavailable)
	case http.StatusOK:
	default:
		return nil, fmt.Errorf("%w: unexpected ownership status %d", ErrDiscordUnavailable, resp.StatusCode)
	}
	var guilds []discordGuild
	if err := json.NewDecoder(resp.Body).Decode(&guilds); err != nil {
		return nil, fmt.Errorf("%w: decode guild ownership: %v", ErrDiscordUnavailable, err)
	}
	return guilds, nil
}
