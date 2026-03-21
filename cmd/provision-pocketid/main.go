package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jxsl13/asset-service/cmd/provision-pocketid/pocketid"
)

func main() {
	if err := run(); err != nil {
		slog.Error("provision-pocketid", "err", err)
		os.Exit(1)
	}
}

func run() error {
	envFile := flag.String("env-file", "", "path to env file to update with OIDC_CLIENT_ID and OIDC_CLIENT_SECRET")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	get := func(key string) string { return strings.TrimSpace(os.Getenv(key)) }
	must := func(key string) (string, error) {
		if v := get(key); v != "" {
			return v, nil
		}
		return "", fmt.Errorf("required environment variable %s is not set", key)
	}

	issuerURL, err := must("OIDC_ISSUER_URL")
	if err != nil {
		return err
	}
	apiKey, err := must("POCKET_ID_STATIC_API_KEY")
	if err != nil {
		return err
	}
	redirectURL, err := must("OIDC_REDIRECT_URL")
	if err != nil {
		return err
	}
	postLogoutURL, err := must("OIDC_POST_LOGOUT_REDIRECT_URL")
	if err != nil {
		return err
	}

	adminEmail := get("POCKET_ID_ADMIN_EMAIL")
	if adminEmail == "" {
		adminEmail = "admin@example.com"
	}
	clientName := get("POCKET_ID_CLIENT_NAME")
	if clientName == "" {
		clientName = "Teeworlds Asset Database"
	}
	insecure := get("INSECURE") == "true"

	result, err := pocketid.Provision(ctx, pocketid.Config{
		BaseURL:            issuerURL,
		StaticAPIKey:       apiKey,
		ClientName:         clientName,
		CallbackURLs:       []string{redirectURL},
		LogoutCallbackURLs: []string{postLogoutURL},
		AdminEmail:         adminEmail,
		AdminGroupName:     "admin",
		Insecure:           insecure,
	})
	if err != nil {
		return fmt.Errorf("provision: %w", err)
	}

	if *envFile != "" {
		if err := updateEnvFile(*envFile, result.ClientID, result.Secret); err != nil {
			return fmt.Errorf("update env file: %w", err)
		}
		fmt.Printf("Updated %s with OIDC_CLIENT_ID and OIDC_CLIENT_SECRET\n", *envFile)
	} else {
		fmt.Println()
		fmt.Println("=== Pocket-ID Provisioning Complete ===")
		fmt.Println()
		fmt.Println("Add the following to docker/dev.env:")
		fmt.Println()
		fmt.Printf("  OIDC_CLIENT_ID=%s\n", result.ClientID)
		fmt.Printf("  OIDC_CLIENT_SECRET=%s\n", result.Secret)
	}

	if result.LoginURL != "" {
		fmt.Println()
		fmt.Println("One-time admin login URL (open in browser to register your passkey):")
		fmt.Println()
		fmt.Printf("  %s\n", result.LoginURL)
	}
	fmt.Println()

	return nil
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
