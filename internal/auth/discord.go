package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	defaultDiscordBaseURL = "https://discord.com"
	defaultHTTPTimeout    = 5 * time.Second
)

// DiscordUser represents a Discord user from the API.
type DiscordUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

//go:generate mockgen -destination=mock/mock_discord.go -package=authmock github.com/KirkDiggler/rpg-api/internal/auth TokenValidator

// TokenValidator validates Discord tokens.
type TokenValidator interface {
	GetCurrentUser(ctx context.Context, token string) (*DiscordUser, error)
}

// DiscordClient validates Discord access tokens.
type DiscordClient struct {
	httpClient *http.Client
	baseURL    string
}

// DiscordClientOption configures the Discord client.
type DiscordClientOption func(*DiscordClient)

// WithBaseURL sets a custom base URL (useful for testing).
func WithBaseURL(url string) DiscordClientOption {
	return func(c *DiscordClient) {
		c.baseURL = url
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) DiscordClientOption {
	return func(c *DiscordClient) {
		c.httpClient = client
	}
}

// NewDiscordClient creates a new Discord client.
func NewDiscordClient(opts ...DiscordClientOption) *DiscordClient {
	c := &DiscordClient{
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
		baseURL: defaultDiscordBaseURL,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// GetCurrentUser validates a Discord token and returns the user info.
func (c *DiscordClient) GetCurrentUser(ctx context.Context, token string) (*DiscordUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/users/@me", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDiscordUnavailable, err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDiscordUnavailable, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, ErrInvalidToken
	case resp.StatusCode >= 500:
		return nil, ErrDiscordUnavailable
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%w: unexpected status %d", ErrDiscordUnavailable, resp.StatusCode)
	}

	var user DiscordUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("%w: failed to decode response: %v", ErrDiscordUnavailable, err)
	}

	return &user, nil
}
