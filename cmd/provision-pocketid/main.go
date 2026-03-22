package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/jxsl13/teeworlds-asset-service/cmd/provision-pocketid/pocketid"
)

func main() {
	if err := run(); err != nil {
		slog.Error("provision-pocketid", "err", err)
		os.Exit(1)
	}
}

func run() error {
	envFile := flag.String("env-file", "", "path to .env file (required — reads config from it and writes OIDC_CLIENT_ID and OIDC_CLIENT_SECRET back)")
	flag.Parse()

	if *envFile == "" {
		return fmt.Errorf("flag -env-file is required (e.g. -env-file docker/dev.env)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	env, err := readEnvFile(*envFile)
	if err != nil {
		return fmt.Errorf("read env file: %w", err)
	}

	get := func(key string) string { return strings.TrimSpace(env[key]) }
	must := func(key string) (string, error) {
		if v := get(key); v != "" {
			return v, nil
		}
		return "", fmt.Errorf("required key %s is not set in %s", key, *envFile)
	}

	issuerURL, err := must("OIDC_ISSUER_URL")
	if err != nil {
		return err
	}
	apiKey, err := must("POCKET_ID_STATIC_API_KEY")
	if err != nil {
		return err
	}
	externalURL, err := must("EXTERNAL_URL")
	if err != nil {
		return err
	}
	externalURL = strings.TrimRight(externalURL, "/")

	redirectURL := externalURL + "/auth/callback"
	postLogoutURL := externalURL + "/auth/post-logout"

	adminEmail := get("POCKET_ID_ADMIN_EMAIL")
	if adminEmail == "" {
		adminEmail = "admin@example.com"
	}
	clientName := get("POCKET_ID_CLIENT_NAME")
	if clientName == "" {
		clientName = "Teeworlds Asset Database"
	}
	insecure := strings.EqualFold(get("INSECURE"), "true")

	result, err := pocketid.Provision(ctx, pocketid.Config{
		BaseURL:            issuerURL,
		StaticAPIKey:       apiKey,
		ClientName:         clientName,
		CallbackURLs:       []string{redirectURL},
		LogoutCallbackURLs: []string{postLogoutURL},
		LaunchURL:          externalURL,
		AdminEmail:         adminEmail,
		AdminGroupName:     "admin",
		Insecure:           insecure,
	})
	if err != nil {
		return fmt.Errorf("provision: %w", err)
	}

	if err := updateEnvFile(*envFile, result.ClientID, result.Secret); err != nil {
		return fmt.Errorf("update env file: %w", err)
	}
	fmt.Printf("Updated %s with OIDC_CLIENT_ID and OIDC_CLIENT_SECRET\n", *envFile)

	if result.LoginURL != "" {
		fmt.Println()
		fmt.Println("One-time admin login URL (open in browser to register your passkey):")
		fmt.Println()
		fmt.Printf("  %s\n", result.LoginURL)
	}
	fmt.Println()

	return nil
}

// readEnvFile parses a KEY=VALUE env file (comments and blank lines are ignored)
// and returns a map of all key/value pairs.
func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	env := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		env[strings.TrimSpace(key)] = stripQuotes(strings.TrimSpace(value))
	}
	return env, scanner.Err()
}

// stripQuotes removes surrounding quotes from a .env value using strconv.Unquote.
func stripQuotes(s string) string {
	if unquoted, err := strconv.Unquote(s); err == nil {
		return unquoted
	}
	return s
}

// updateEnvFile replaces OIDC_CLIENT_ID and OIDC_CLIENT_SECRET values in the
// given env file. Lines are matched by key prefix; unknown keys are left as-is.
func updateEnvFile(path, clientID, clientSecret string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "OIDC_CLIENT_ID="):
			line = "OIDC_CLIENT_ID=" + clientID
		case strings.HasPrefix(line, "OIDC_CLIENT_SECRET="):
			line = "OIDC_CLIENT_SECRET=" + clientSecret
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}
