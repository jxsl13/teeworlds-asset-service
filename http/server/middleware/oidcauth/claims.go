package oidcauth

// Claims represents the user information extracted from a Pocket ID token.
// These map to the claims_supported in Pocket ID's OIDC discovery document:
// "sub", "given_name", "family_name", "name", "email", "email_verified",
// "preferred_username", "picture", "groups"
type Claims struct {
	Subject           string   `json:"sub"`
	Name              string   `json:"name"`
	GivenName         string   `json:"given_name"`
	FamilyName        string   `json:"family_name"`
	Email             string   `json:"email"`
	EmailVerified     bool     `json:"email_verified"`
	PreferredUsername string   `json:"preferred_username"`
	Picture           string   `json:"picture"`
	Groups            []string `json:"groups"`
}

// HasGroup returns true if the user belongs to the given group.
func (c *Claims) HasGroup(group string) bool {
	for _, g := range c.Groups {
		if g == group {
			return true
		}
	}
	return false
}

// HasAnyGroup returns true if the user belongs to at least one of the given groups.
func (c *Claims) HasAnyGroup(groups ...string) bool {
	set := make(map[string]struct{}, len(c.Groups))
	for _, g := range c.Groups {
		set[g] = struct{}{}
	}
	for _, g := range groups {
		if _, ok := set[g]; ok {
			return true
		}
	}
	return false
}
