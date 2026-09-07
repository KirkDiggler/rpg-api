package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/auth"
)

func TestDiscordClient_GetCurrentUser_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/users/@me", r.URL.Path)
		assert.Equal(t, "Bearer valid-token", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(map[string]string{
			"id":       "123456789",
			"username": "testuser",
		})
		require.NoError(t, err)
	}))
	defer server.Close()

	client := auth.NewDiscordClient(auth.WithBaseURL(server.URL))

	user, err := client.GetCurrentUser(context.Background(), "valid-token")

	require.NoError(t, err)
	assert.Equal(t, "123456789", user.ID)
	assert.Equal(t, "testuser", user.Username)
}

func TestDiscordClient_GetCurrentUser_InvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := auth.NewDiscordClient(auth.WithBaseURL(server.URL))

	user, err := client.GetCurrentUser(context.Background(), "invalid-token")

	assert.Nil(t, user)
	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestDiscordClient_GetCurrentUser_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := auth.NewDiscordClient(auth.WithBaseURL(server.URL))

	user, err := client.GetCurrentUser(context.Background(), "any-token")

	assert.Nil(t, user)
	assert.ErrorIs(t, err, auth.ErrDiscordUnavailable)
}

func TestDiscordClient_GetCurrentUser_NetworkError(t *testing.T) {
	// Use a server that closes immediately
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	client := auth.NewDiscordClient(auth.WithBaseURL(server.URL))

	user, err := client.GetCurrentUser(context.Background(), "any-token")

	assert.Nil(t, user)
	assert.ErrorIs(t, err, auth.ErrDiscordUnavailable)
}

func TestDiscordClient_GetCurrentUser_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := auth.NewDiscordClient(auth.WithBaseURL(server.URL))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	user, err := client.GetCurrentUser(ctx, "any-token")

	assert.Nil(t, user)
	assert.Error(t, err)
}

func TestDiscordClient_GetCurrentUserGuildMember_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/users/@me/guilds/123456789012345678/member", r.URL.Path)
		assert.Equal(t, "Bearer membership-credential", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"user": map[string]string{"id": "player-1", "username": "member"},
		}))
	}))
	defer server.Close()

	client := auth.NewDiscordClient(auth.WithBaseURL(server.URL))
	member, err := client.GetCurrentUserGuildMember(context.Background(), &auth.GetCurrentUserGuildMemberInput{
		Token: "membership-credential", GuildID: "123456789012345678",
	})
	require.NoError(t, err)
	require.NotNil(t, member.User)
	require.Equal(t, "player-1", member.User.ID)
}

func TestDiscordClient_GetCurrentUserGuildMember_StatusMapping(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		expected error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, expected: auth.ErrInvalidToken},
		{name: "forbidden", status: http.StatusForbidden, expected: auth.ErrGuildMembershipDenied},
		{name: "not found", status: http.StatusNotFound, expected: auth.ErrGuildMembershipDenied},
		{name: "rate limited", status: http.StatusTooManyRequests, expected: auth.ErrDiscordUnavailable},
		{name: "server error", status: http.StatusInternalServerError, expected: auth.ErrDiscordUnavailable},
		{name: "unexpected", status: http.StatusBadRequest, expected: auth.ErrDiscordUnavailable},
		{name: "bad JSON", status: http.StatusOK, body: `{`, expected: auth.ErrDiscordUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := auth.NewDiscordClient(auth.WithBaseURL(server.URL))
			member, err := client.GetCurrentUserGuildMember(context.Background(), &auth.GetCurrentUserGuildMemberInput{
				Token: "membership-credential", GuildID: "123456789012345678",
			})
			assert.Nil(t, member)
			assert.ErrorIs(t, err, tt.expected)
		})
	}
}

func TestDiscordClient_GetCurrentUserGuildMember_InvalidInput(t *testing.T) {
	client := auth.NewDiscordClient()
	member, err := client.GetCurrentUserGuildMember(context.Background(), nil)
	assert.Nil(t, member)
	assert.ErrorIs(t, err, auth.ErrDiscordUnavailable)
}

func TestDiscordClient_GetCurrentUserGuildMember_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := auth.NewDiscordClient(
		auth.WithBaseURL(server.URL),
		auth.WithHTTPClient(&http.Client{Timeout: time.Millisecond}),
	)

	member, err := client.GetCurrentUserGuildMember(context.Background(), &auth.GetCurrentUserGuildMemberInput{
		Token: "membership-credential", GuildID: "123456789012345678",
	})
	assert.Nil(t, member)
	assert.ErrorIs(t, err, auth.ErrDiscordUnavailable)
}
