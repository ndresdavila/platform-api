package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

type Config struct {
	Issuer                                                        string
	Aliases                                                       []string
	JWKSURI, MobileClientID, BackendClientID, BackendClientSecret string
}
type StaffConfig struct {
	Issuer  string
	Aliases []string
	JWKSURI string
}
type TokenSet struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
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
func ClientConfig() Config {
	issuer := normalized(env("KEYCLOAK_ISSUER", "http://localhost:8080/realms/nails"))
	return Config{issuer, aliases(issuer, os.Getenv("KEYCLOAK_ISSUER_ALIASES")), env("KEYCLOAK_JWKS_URI", issuer+"/protocol/openid-connect/certs"), env("KEYCLOAK_MOBILE_CLIENT_ID", "nails-mobile"), env("KEYCLOAK_BACKEND_CLIENT_ID", "nails-backend"), env("KEYCLOAK_BACKEND_CLIENT_SECRET", "change-me-nails-backend-secret")}
}
func Staff() StaffConfig {
	issuer := normalized(env("KEYCLOAK_STAFF_ISSUER", "http://localhost:8080/realms/nails-staff"))
	return StaffConfig{issuer, aliases(issuer, os.Getenv("KEYCLOAK_STAFF_ISSUER_ALIASES")), env("KEYCLOAK_STAFF_JWKS_URI", issuer+"/protocol/openid-connect/certs")}
}
func tokenURL(issuer string) string { return issuer + "/protocol/openid-connect/token" }
func postForm(ctx context.Context, endpoint string, values url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return errors.New("Keycloak rechazó la solicitud")
	}
	return nil
}
func PasswordGrant(ctx context.Context, username, password string) (*TokenSet, error) {
	cfg := ClientConfig()
	v := url.Values{"grant_type": {"password"}, "client_id": {cfg.MobileClientID}, "username": {username}, "password": {password}, "scope": {"openid profile email"}}
	var out TokenSet
	return &out, postForm(ctx, tokenURL(cfg.Issuer), v, &out)
}
func RefreshGrant(ctx context.Context, refresh string) (*TokenSet, error) {
	cfg := ClientConfig()
	var out TokenSet
	return &out, postForm(ctx, tokenURL(cfg.Issuer), url.Values{"grant_type": {"refresh_token"}, "client_id": {cfg.MobileClientID}, "refresh_token": {refresh}}, &out)
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
func VerifyAccessToken(ctx context.Context, raw string) (*Payload, error) {
	c := ClientConfig()
	return verify(ctx, raw, c.JWKSURI, c.Aliases)
}
func VerifyStaffAccessToken(ctx context.Context, raw string) (*Payload, error) {
	c := Staff()
	return verify(ctx, raw, c.JWKSURI, c.Aliases)
}
func RolesFromPayload(p *Payload) string {
	for _, r := range p.RealmAccess.Roles {
		switch strings.ToLower(r) {
		case "admin":
			return "ADMIN"
		case "estilista":
			return "ESTILISTA"
		}
	}
	return "CLIENTE"
}
func StaffRole(p *Payload) string {
	for _, r := range p.RealmAccess.Roles {
		if strings.EqualFold(r, "admin") {
			return "ADMIN"
		}
	}
	return "RECEPCIONISTA"
}
func HasStaffAccess(p *Payload) bool {
	for _, r := range p.RealmAccess.Roles {
		if strings.EqualFold(r, "admin") || strings.EqualFold(r, "recepcionista") {
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
		last = "Salón"
	}
	return
}
func serviceToken(ctx context.Context) (string, error) {
	c := ClientConfig()
	var v struct {
		AccessToken string `json:"access_token"`
	}
	err := postForm(ctx, tokenURL(c.Issuer), url.Values{"grant_type": {"client_credentials"}, "client_id": {c.BackendClientID}, "client_secret": {c.BackendClientSecret}}, &v)
	return v.AccessToken, err
}
func realmParts(issuer string) (server, realm string) {
	a := strings.Split(issuer, "/realms/")
	return a[0], a[1]
}
func CreateUser(ctx context.Context, email, password, first, last string) (string, error) {
	c := ClientConfig()
	token, err := serviceToken(ctx)
	if err != nil {
		return "", err
	}
	server, realm := realmParts(c.Issuer)
	body, _ := json.Marshal(map[string]any{"username": strings.ToLower(email), "email": strings.ToLower(email), "enabled": true, "emailVerified": true, "firstName": first, "lastName": last, "credentials": []map[string]any{{"type": "password", "value": password, "temporary": false}}})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, server+"/admin/realms/"+realm+"/users", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode == 409 {
		return "", fmt.Errorf("Email ya registrado en Keycloak")
	}
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("Keycloak create user: %s", res.Status)
	}
	return strings.TrimRight(res.Header.Get("Location"), "/")[strings.LastIndex(strings.TrimRight(res.Header.Get("Location"), "/"), "/")+1:], nil
}
func AssignRealmRole(ctx context.Context, userID, role string) error {
	c := ClientConfig()
	token, err := serviceToken(ctx)
	if err != nil {
		return err
	}
	server, realm := realmParts(c.Issuer)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server+"/admin/realms/"+realm+"/roles/"+url.PathEscape(role), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return nil
	}
	var roleJSON any
	if err = json.NewDecoder(res.Body).Decode(&roleJSON); err != nil {
		return err
	}
	b, _ := json.Marshal([]any{roleJSON})
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, server+"/admin/realms/"+realm+"/users/"+userID+"/role-mappings/realm", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("no se pudo asignar rol")
	}
	return nil
}

var _ = time.Second
