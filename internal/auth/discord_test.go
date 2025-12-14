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
