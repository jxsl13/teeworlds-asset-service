package oidcauth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// sessionData holds OIDC flow state (pre-auth) and user session (post-auth).
type sessionData struct {
	// Flow state (cleared after callback)
	State        string `json:"state,omitempty"`
	Nonce        string `json:"nonce,omitempty"`
	CodeVerifier string `json:"code_verifier,omitempty"`
	ReturnTo     string `json:"return_to,omitempty"`

	// Authenticated session
	Claims       *Claims `json:"claims,omitempty"`
	AccessToken  string  `json:"access_token,omitempty"`
	RefreshToken string  `json:"refresh_token,omitempty"`
	IDToken      string  `json:"id_token,omitempty"`
	ExpiresAt    int64   `json:"expires_at,omitempty"`
}

// sessionStore is a simple server-side session store keyed by a random session ID.
// In production, replace this with Redis, a database, or an encrypted cookie.
type sessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*sessionData
}

func newSessionStore() *sessionStore {
	return &sessionStore{
		sessions: make(map[string]*sessionData),
	}
}

func (s *sessionStore) get(id string) (*sessionData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func (s *sessionStore) set(id string, data *sessionData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = data
}

func (s *sessionStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *sessionStore) create() (string, *sessionData) {
	id := generateRandom(32)
	data := &sessionData{}
	s.set(id, data)
	return id, data
}

// setSessionCookie writes the session ID cookie to the response.
func setSessionCookie(w http.ResponseWriter, name, value string, maxAge time.Duration, secure bool, domain string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Domain:   domain,
	})
}

// clearSessionCookie deletes the session cookie.
func clearSessionCookie(w http.ResponseWriter, name string, domain string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Domain:   domain,
	})
}

func generateRandom(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// MarshalJSON and UnmarshalJSON are used if you want to serialize session data
// (e.g., for encrypted cookie storage instead of server-side store).
func marshalSession(data *sessionData) (string, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func unmarshalSession(s string) (*sessionData, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var data sessionData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return &data, nil
}
