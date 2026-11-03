package provider

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	kernelhttp "github.com/mateusmacedo/dmpf/libs/backend/go/http"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/metrics"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/redact"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/resilience"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/deadline"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"
)

// ErrKeycloakConfig is a configuration the Keycloak adapter cannot run with;
// the message names the field, never its value.
var ErrKeycloakConfig = errors.New("provider: keycloak configuration is invalid")

const (
	keycloakDependency = "keycloak"
	keycloakProvider   = "keycloak"

	// The names the spec fixes in the realm: the client whose roles are the
	// permissions of a store, and the realm role of a buyer.
	keycloakStoreClient = "marketplace-store"
	keycloakBuyerRole   = "iam:buyer"

	keycloakCallLimit    = resilience.DefaultDeadline
	keycloakCallEstimate = 200 * time.Millisecond

	keycloakAdminContract = "https://www.keycloak.org/docs-api/26.8.1/rest-api/index.html#"
	keycloakTokenContract = "https://www.keycloak.org/securing-apps/oidc-layers#_token_endpoint"
)

var keycloakRetryableStatus = []int{http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout}

var keycloakOutcomes = kernelhttp.OutcomeTable{
	Version: "keycloak-admin-26.8/1",
	Status: map[int]ports.ExternalClass{
		http.StatusOK:                  ports.ClassAccepted,
		http.StatusCreated:             ports.ClassAccepted,
		http.StatusNoContent:           ports.ClassAccepted,
		http.StatusBadRequest:          ports.ClassFinalValidation,
		http.StatusUnauthorized:        ports.ClassFinalAuthorization,
		http.StatusForbidden:           ports.ClassFinalAuthorization,
		http.StatusNotFound:            ports.ClassFinalBusiness,
		http.StatusConflict:            ports.ClassFinalBusiness,
		http.StatusTooManyRequests:     ports.ClassRetryableUpstream,
		http.StatusInternalServerError: ports.ClassRetryableUpstream,
		http.StatusBadGateway:          ports.ClassRetryableUpstream,
		http.StatusServiceUnavailable:  ports.ClassRetryableUpstream,
		http.StatusGatewayTimeout:      ports.ClassRetryableUpstream,
	},
}

// KeycloakOutcomes is the versioned table of the Admin API: each documented
// status and its class. A 500 is retryable because every command reads the
// partner again before each change it makes.
func KeycloakOutcomes() kernelhttp.OutcomeTable {
	return kernelhttp.OutcomeTable{Version: keycloakOutcomes.Version, Status: maps.Clone(keycloakOutcomes.Status)}
}

var keycloakProfile = ports.ExternalProfile{
	KeySemantics:         ports.KeyNone,
	Efficacy:             ports.EfficacyOnSubmit,
	Quiescence:           0,
	AuthoritativeAbsence: false,
	MaxQueries:           1,
	KeyAlphabet:          "0123456789abcdef",
	KeyMaxLength:         64,
}

// KeycloakSecret is the client secret of the service account. Every rendering
// of it — fmt, JSON, text, slog — is the redaction placeholder.
type KeycloakSecret struct {
	// WHY: a pointer prints as an address when an enclosing unexported field
	// makes fmt bypass the methods below.
	value *string
}

// NewKeycloakSecret wraps the secret read from the environment.
func NewKeycloakSecret(value string) KeycloakSecret {
	return KeycloakSecret{value: &value}
}

// IsZero reports an absent or empty secret.
func (s KeycloakSecret) IsZero() bool { return s.value == nil || *s.value == "" }

func (s KeycloakSecret) String() string { return redact.Placeholder }

func (s KeycloakSecret) GoString() string { return redact.Placeholder }

func (s KeycloakSecret) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, redact.Placeholder) }

func (s KeycloakSecret) MarshalJSON() ([]byte, error) {
	return []byte(`"` + redact.Placeholder + `"`), nil
}

func (s KeycloakSecret) MarshalText() ([]byte, error) { return []byte(redact.Placeholder), nil }

func (s KeycloakSecret) reveal() string {
	if s.value == nil {
		return ""
	}
	return *s.value
}

// KeycloakConfig is the service account of the iam in a realm. OperatorTenant
// fences the commands: the store ones never run in it, BuyerAccess runs only in it.
type KeycloakConfig struct {
	BaseURL        string
	Realm          string
	ClientID       string
	Secret         KeycloakSecret
	OperatorTenant string
}

// KeycloakDeps is what the client records through, from the composition root;
// Transport and Rand stay nil outside tests.
type KeycloakDeps struct {
	Clock          clock.Clock
	Tracer         trace.Tracer
	Instruments    *metrics.Instruments
	LoggerProvider log.LoggerProvider
	Transport      http.RoundTripper
	Rand           func() float64
}

// Keycloak is the partner of the external commands of the iam, over the Admin
// API of Keycloak 26.8 and a service account under the FGAP v2 of the spec.
type Keycloak struct {
	client   *kernelhttp.Client
	routes   map[string]kernelhttp.Route
	tokens   *keycloakTokens
	adminURL string
	operator string
}

// NewKeycloak validates the configuration and composes the kernel client once,
// with the platform sheet of the dependency; no request reaches the partner here.
func NewKeycloak(cfg KeycloakConfig, deps KeycloakDeps) (*Keycloak, error) {
	base, err := keycloakBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	switch {
	// WHY: url.PathEscape keeps a dot segment, which lifts the token and the
	// Admin API URLs out of the realm.
	case unsafeSegment(cfg.Realm):
		return nil, fmt.Errorf("%w: realm is empty or a dot segment", ErrKeycloakConfig)
	case cfg.ClientID == "":
		return nil, fmt.Errorf("%w: client id", ErrKeycloakConfig)
	case cfg.Secret.IsZero():
		return nil, fmt.Errorf("%w: secret", ErrKeycloakConfig)
	case cfg.OperatorTenant == "":
		return nil, fmt.Errorf("%w: operator tenant", ErrKeycloakConfig)
	case deps.Clock == nil:
		return nil, fmt.Errorf("%w: clock", ErrKeycloakConfig)
	}
	routes := keycloakRoutes()
	client, err := kernelhttp.NewClient(kernelhttp.Config{
		Routes:         routes,
		Sheet:          keycloakSheet(),
		Transport:      deps.Transport,
		Clock:          deps.Clock,
		Tracer:         deps.Tracer,
		Instruments:    deps.Instruments,
		LoggerProvider: deps.LoggerProvider,
		Rand:           deps.Rand,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrKeycloakConfig, err)
	}
	realm := url.PathEscape(cfg.Realm)
	return &Keycloak{
		client: client,
		routes: routes,
		tokens: &keycloakTokens{
			client:   client,
			clock:    deps.Clock,
			url:      base + "/realms/" + realm + "/protocol/openid-connect/token",
			clientID: cfg.ClientID,
			secret:   cfg.Secret,
			granting: make(chan struct{}, 1),
		},
		adminURL: base + "/admin/realms/" + realm,
		operator: cfg.OperatorTenant,
	}, nil
}

func keycloakBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: base URL must be an http or https origin without credentials, query or fragment", ErrKeycloakConfig)
	}
	return strings.TrimRight(raw, "/"), nil
}

// Profile is the declaration of Keycloak as a partner: no outbound key, the
// accepted answer is already the effect, and no query by key exists.
func (k *Keycloak) Profile() ports.ExternalProfile { return keycloakProfile }

func keycloakSheet() resilience.Sheet { return resilience.Defaults(keycloakDependency) }

// The route names are the methods of the resilience operation, so they name
// the series and the spans of each call.
const (
	routeToken                = "token"
	routeFindOrganizations    = "find_organizations"
	routeFindClients          = "find_clients"
	routeListClientRoles      = "list_client_roles"
	routeFindGroups           = "find_groups"
	routeCreateGroup          = "create_group"
	routeReadGroupRoles       = "read_group_roles"
	routeAddGroupRoles        = "add_group_roles"
	routeRemoveGroupRoles     = "remove_group_roles"
	routeFindUsers            = "find_users"
	routeCreateUser           = "create_user"
	routeAddMember            = "add_member"
	routeReadMember           = "read_member"
	routeRemoveMember         = "remove_member"
	routeListMemberGroups     = "list_member_groups"
	routeAddGroupMember       = "add_group_member"
	routeRemoveGroupMember    = "remove_group_member"
	routeReadRealmRole        = "read_realm_role"
	routeReadUserRealmRoles   = "read_user_realm_roles"
	routeAddUserRealmRoles    = "add_user_realm_roles"
	routeRemoveUserRealmRoles = "remove_user_realm_roles"
)

func keycloakRoutes() map[string]kernelhttp.Route {
	const (
		organization = "/admin/realms/{realm}/organizations/{org-id}"
		groupRoles   = organization + "/groups/{group-id}/role-mappings/clients/{client-id}"
		groupMember  = organization + "/groups/{group-id}/members/{userId}"
		member       = organization + "/members/{member-id}"
		userRealm    = "/admin/realms/{realm}/users/{user-id}/role-mappings/realm"
	)
	routes := []kernelhttp.Route{
		keycloakRoute(routeToken, http.MethodPost, "/realms/{realm}/protocol/openid-connect/token", keycloakTokenContract),
		adminRoute(routeFindOrganizations, http.MethodGet, "/admin/realms/{realm}/organizations", "_get_adminrealmsrealmorganizations"),
		adminRoute(routeFindClients, http.MethodGet, "/admin/realms/{realm}/clients", "_get_adminrealmsrealmclients"),
		adminRoute(routeListClientRoles, http.MethodGet, "/admin/realms/{realm}/clients/{client-uuid}/roles", "_get_adminrealmsrealmclientsclient_uuidroles"),
		adminRoute(routeFindGroups, http.MethodGet, organization+"/groups", "_get_adminrealmsrealmorganizationsorg_idgroups"),
		adminRoute(routeCreateGroup, http.MethodPost, organization+"/groups", "_post_adminrealmsrealmorganizationsorg_idgroups"),
		adminRoute(routeReadGroupRoles, http.MethodGet, groupRoles, "_get_adminrealmsrealmorganizationsorg_idgroupsgroup_idrole_mappingsclientsclient_id"),
		adminRoute(routeAddGroupRoles, http.MethodPost, groupRoles, "_post_adminrealmsrealmorganizationsorg_idgroupsgroup_idrole_mappingsclientsclient_id"),
		adminRoute(routeRemoveGroupRoles, http.MethodDelete, groupRoles, "_delete_adminrealmsrealmorganizationsorg_idgroupsgroup_idrole_mappingsclientsclient_id"),
		adminRoute(routeFindUsers, http.MethodGet, "/admin/realms/{realm}/users", "_get_adminrealmsrealmusers"),
		adminRoute(routeCreateUser, http.MethodPost, "/admin/realms/{realm}/users", "_post_adminrealmsrealmusers"),
		adminRoute(routeAddMember, http.MethodPost, organization+"/members", "_post_adminrealmsrealmorganizationsorg_idmembers"),
		adminRoute(routeReadMember, http.MethodGet, member, "_get_adminrealmsrealmorganizationsorg_idmembersmember_id"),
		adminRoute(routeRemoveMember, http.MethodDelete, member, "_delete_adminrealmsrealmorganizationsorg_idmembersmember_id"),
		adminRoute(routeListMemberGroups, http.MethodGet, member+"/groups", "_get_adminrealmsrealmorganizationsorg_idmembersmember_idgroups"),
		adminRoute(routeAddGroupMember, http.MethodPut, groupMember, "_put_adminrealmsrealmorganizationsorg_idgroupsgroup_idmembersuserid"),
		adminRoute(routeRemoveGroupMember, http.MethodDelete, groupMember, "_delete_adminrealmsrealmorganizationsorg_idgroupsgroup_idmembersuserid"),
		adminRoute(routeReadRealmRole, http.MethodGet, "/admin/realms/{realm}/roles/{role-name}", "_get_adminrealmsrealmrolesrole_name"),
		adminRoute(routeReadUserRealmRoles, http.MethodGet, userRealm, "_get_adminrealmsrealmusersuser_idrole_mappingsrealm"),
		adminRoute(routeAddUserRealmRoles, http.MethodPost, userRealm, "_post_adminrealmsrealmusersuser_idrole_mappingsrealm"),
		adminRoute(routeRemoveUserRealmRoles, http.MethodDelete, userRealm, "_delete_adminrealmsrealmusersuser_idrole_mappingsrealm"),
	}
	byName := make(map[string]kernelhttp.Route, len(routes))
	for _, route := range routes {
		byName[route.Name] = route
	}
	return byName
}

func adminRoute(name, method, path, anchor string) kernelhttp.Route {
	return keycloakRoute(name, method, path, keycloakAdminContract+anchor)
}

// keycloakRoute retries a transient status only on what RST-02 admits: a POST
// carries no key Keycloak deduplicates on, so the command's reread retries it.
func keycloakRoute(name, method, path, contract string) kernelhttp.Route {
	route := kernelhttp.Route{
		Name:        name,
		Method:      method,
		Path:        path,
		ContractRef: contract,
		Budget: deadline.Budget{
			Dependency:        keycloakDependency,
			Method:            name,
			Limit:             keycloakCallLimit,
			Slack:             deadline.DefaultSlack,
			EstimatedDuration: keycloakCallEstimate,
		},
		Requires: kernelhttp.RequireTenant,
	}
	if method != http.MethodPost {
		route.RetryableStatus = slices.Clone(keycloakRetryableStatus)
	}
	return route
}
