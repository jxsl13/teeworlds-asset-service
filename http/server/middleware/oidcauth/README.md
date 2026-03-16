# Middleware usage example

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"

    "myapp/oidcauth"
)

func main() {
    ctx := context.Background()

    // Initialize the OIDC provider (discovers Pocket ID endpoints automatically)
    auth, err := oidcauth.NewProvider(ctx, oidcauth.Config{
        IssuerURL:             env("POCKET_ID_URL", "https://id.example.com"),
        ClientID:              env("OIDC_CLIENT_ID", "my-go-app"),
        ClientSecret:          env("OIDC_CLIENT_SECRET", ""),
        RedirectURL:           env("OIDC_REDIRECT_URL", "http://localhost:8080/auth/callback"),
        PostLogoutRedirectURL: env("POST_LOGOUT_URL", "http://localhost:8080"),
        EnablePKCE:            true,
        CookieSecure:          false, // Set to true in production with HTTPS
    })
    if err != nil {
        log.Fatalf("Failed to initialize OIDC provider: %v", err)
    }

    mux := http.NewServeMux()

    // Register the login, callback, and logout handlers
    auth.RegisterHandlers(mux)

    // --- Public routes (no auth required) ---
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, `<html><body>
            <h1>My App</h1>
            <a href="/auth/login">Login with Pocket ID</a> |
            <a href="/public">Public Endpoint</a> |
            <a href="/dashboard">Dashboard (requires login)</a> |
            <a href="/admin">Admin Panel (requires "admins" group)</a>
        </body></html>`)
    })

    mux.HandleFunc("/public", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintln(w, "This endpoint is publicly accessible.")
    })

    // --- Protected routes ---

    // Any authenticated user
    mux.Handle("/dashboard", auth.RequireAuthFunc(dashboardHandler))

    // Any authenticated user (API endpoint returning JSON)
    mux.Handle("/api/me", auth.RequireAuthFunc(meAPIHandler))

    // Only users in the "admins" group
    mux.Handle("/admin", auth.RequireGroupFunc(adminHandler, "admins"))

    // Optional auth — different behavior for logged-in vs anonymous users
    mux.Handle("/greeting", auth.OptionalAuth(http.HandlerFunc(greetingHandler)))

    log.Println("Server starting on :8080")
    log.Fatal(http.ListenAndServe(":8080", mux))
}

// dashboardHandler is a protected page. The user is guaranteed to be authenticated.
func dashboardHandler(w http.ResponseWriter, r *http.Request) {
    claims := oidcauth.ClaimsFromContext(r.Context())

    fmt.Fprintf(w, `<html><body>
        <h1>Dashboard</h1>
        <p>Welcome, <strong>%s</strong> (%s)</p>
        <p>Groups: %v</p>
        <a href="/auth/logout">Logout</a>
    </body></html>`, claims.Name, claims.Email, claims.Groups)
}

// meAPIHandler returns user info as JSON. Supports both session cookies and Bearer tokens.
func meAPIHandler(w http.ResponseWriter, r *http.Request) {
    claims := oidcauth.ClaimsFromContext(r.Context())

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(claims)
}

// adminHandler requires the "admins" group.
func adminHandler(w http.ResponseWriter, r *http.Request) {
    claims := oidcauth.ClaimsFromContext(r.Context())

    fmt.Fprintf(w, `<html><body>
        <h1>Admin Panel</h1>
        <p>Hello admin <strong>%s</strong>!</p>
        <a href="/auth/logout">Logout</a>
    </body></html>`, claims.Name)
}

// greetingHandler shows different content based on whether the user is authenticated.
func greetingHandler(w http.ResponseWriter, r *http.Request) {
    claims := oidcauth.ClaimsFromContext(r.Context())
    if claims != nil {
        fmt.Fprintf(w, "Hello, %s! You are logged in.", claims.Name)
    } else {
        fmt.Fprintln(w, "Hello, stranger! <a href='/auth/login'>Log in</a> to see your name.")
    }
}

func env(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}
```
