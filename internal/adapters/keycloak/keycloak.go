package keycloak

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

type StaffConfig struct {
	Issuer  string
	Aliases []string
	JWKSURI string
}

type Payload struct {
	Subject           string `json:"sub"`
	Email             string `json:"email"`
	GivenName         string `json:"given_name"`
	FamilyName        string `json:"family_name"`
	PreferredUsername string `json:"preferred_username"`
	RealmAccess       struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
	jwt.RegisteredClaims
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func normalized(s string) string { return strings.TrimRight(s, "/") }

func aliases(primary, raw string) []string {
	out := []string{normalized(primary)}
	for _, v := range strings.Split(raw, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, normalized(v))
		}
	}
	return out
}

func Staff() StaffConfig {
	issuer := normalized(env("KEYCLOAK_STAFF_ISSUER", "http://localhost:8080/realms/masterview"))
	return StaffConfig{
		issuer,
		aliases(issuer, os.Getenv("KEYCLOAK_STAFF_ISSUER_ALIASES")),
		env("KEYCLOAK_STAFF_JWKS_URI", issuer+"/protocol/openid-connect/certs"),
	}
}

func verify(ctx context.Context, raw, jwksURI string, issuers []string) (*Payload, error) {
	set, err := jwk.Fetch(ctx, jwksURI)
	if err != nil {
		return nil, err
	}
	p := &Payload{}
	token, err := jwt.ParseWithClaims(raw, p, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		key, ok := set.LookupKeyID(kid)
		if !ok {
			return nil, errors.New("clave JWT desconocida")
		}
		var pub any
		if err := key.Raw(&pub); err != nil {
			return nil, err
		}
		return pub, nil
	}, jwt.WithValidMethods([]string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512"}))
	if err != nil || !token.Valid {
		return nil, errors.New("token inválido")
	}
	valid := false
	for _, i := range issuers {
		if p.Issuer == i {
			valid = true
		}
	}
	if !valid {
		return nil, errors.New("issuer inválido")
	}
	return p, nil
}

func VerifyStaffAccessToken(ctx context.Context, raw string) (*Payload, error) {
	c := Staff()
	return verify(ctx, raw, c.JWKSURI, c.Aliases)
}

func StaffRole(p *Payload) string {
	for _, r := range p.RealmAccess.Roles {
		if strings.EqualFold(r, "admin") {
			return "ADMIN"
		}
		if strings.EqualFold(r, "contador") {
			return "CONTADOR"
		}
	}
	return "RECEPCIONISTA"
}

func HasStaffAccess(p *Payload) bool {
	for _, r := range p.RealmAccess.Roles {
		if strings.EqualFold(r, "admin") || strings.EqualFold(r, "recepcionista") || strings.EqualFold(r, "contador") {
			return true
		}
	}
	return false
}

func Profile(p *Payload) (email, first, last string) {
	email = strings.ToLower(p.Email)
	if email == "" {
		email = strings.ToLower(p.PreferredUsername)
	}
	first = p.GivenName
	if first == "" {
		first = strings.Split(email, "@")[0]
	}
	last = p.FamilyName
	if last == "" {
		last = "Usuario"
	}
	return
}
