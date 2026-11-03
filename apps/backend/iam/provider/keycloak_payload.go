package provider

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/ports"
)

// ErrKeycloakPayload is a payload no external command of Keycloak can run: a
// status without wire value or a malformed state, refused by encoding and decoding.
var ErrKeycloakPayload = errors.New("provider: keycloak payload is invalid")

// The status of the payload is textual and fixed here, so a rename in the
// domain does not change what an attempt ledger already holds.
const (
	accessActive  = "active"
	accessRetired = "retired"
	accessRemoved = "removed"
	accessBlocked = "blocked"
)

type roleAccessPayload struct {
	Ref         string   `json:"ref"`
	Status      string   `json:"status"`
	Permissions []string `json:"permissions"`
}

type memberProvisionPayload struct {
	Email string `json:"email"`
}

type memberAccessPayload struct {
	Subject string   `json:"subject"`
	Status  string   `json:"status"`
	Roles   []string `json:"roles"`
}

type buyerAccessPayload struct {
	Subject string `json:"subject"`
	Status  string `json:"status"`
}

// KeycloakPayloads is the codec of the external commands of Keycloak. It
// refuses what the decoder of the command would, before any attempt.
type KeycloakPayloads struct{}

var _ ports.ExternalPayloads = KeycloakPayloads{}

func (KeycloakPayloads) RoleAccess(a ports.RoleAccess) ([]byte, error) {
	var status string
	switch a.Status {
	case domain.RoleStatusActive:
		status = accessActive
	case domain.RoleStatusRetired:
		status = accessRetired
	default:
		return nil, fmt.Errorf("%w: role status %s", ErrKeycloakPayload, a.Status)
	}
	return readBack(roleAccessPayload{Ref: string(a.Ref), Status: status, Permissions: names(a.Permissions)}, decodeRoleAccess)
}

func (KeycloakPayloads) MemberProvision(m ports.MemberProvision) ([]byte, error) {
	return readBack(memberProvisionPayload{Email: string(m.Email)}, decodeMemberProvision)
}

func (KeycloakPayloads) MemberAccess(a ports.MemberAccess) ([]byte, error) {
	var status string
	switch a.Status {
	case domain.MembershipStatusActive:
		status = accessActive
	case domain.MembershipStatusRemoved:
		status = accessRemoved
	default:
		return nil, fmt.Errorf("%w: membership status %s", ErrKeycloakPayload, a.Status)
	}
	return readBack(memberAccessPayload{Subject: string(a.Subject), Status: status, Roles: names(a.Roles)}, decodeMemberAccess)
}

func (KeycloakPayloads) BuyerAccess(a ports.BuyerAccess) ([]byte, error) {
	var status string
	switch a.Status {
	case domain.BuyerStatusActive:
		status = accessActive
	case domain.BuyerStatusBlocked:
		status = accessBlocked
	default:
		return nil, fmt.Errorf("%w: buyer status %s", ErrKeycloakPayload, a.Status)
	}
	return readBack(buyerAccessPayload{Subject: string(a.Subject), Status: status}, decodeBuyerAccess)
}

// readBack passes the bytes through the decoder of the command, so the codec
// holds no rule of its own to drift from the one that runs.
func readBack[T any](payload any, decode func([]byte) (T, error)) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: the payload could not be encoded", ErrKeycloakPayload)
	}
	if _, err := decode(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// The decoders judge the form only; the address and the subject never go to
// the error, which reaches the attempt ledger and the logs.
func decodeRoleAccess(raw []byte) (ports.RoleAccess, error) {
	var p roleAccessPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return ports.RoleAccess{}, fmt.Errorf("%w: role access is not JSON", ErrKeycloakPayload)
	}
	a := ports.RoleAccess{Ref: domain.RoleRef(p.Ref), Permissions: valuesOf[domain.Permission](p.Permissions)}
	if !domain.ValidRef(a.Ref) {
		return ports.RoleAccess{}, fmt.Errorf("%w: role ref", ErrKeycloakPayload)
	}
	switch p.Status {
	case accessActive:
		a.Status = domain.RoleStatusActive
	case accessRetired:
		a.Status = domain.RoleStatusRetired
	default:
		return ports.RoleAccess{}, fmt.Errorf("%w: role status %q", ErrKeycloakPayload, p.Status)
	}
	for _, permission := range a.Permissions {
		if permission == "" {
			return ports.RoleAccess{}, fmt.Errorf("%w: empty permission", ErrKeycloakPayload)
		}
	}
	return a, nil
}

func decodeMemberProvision(raw []byte) (ports.MemberProvision, error) {
	var p memberProvisionPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return ports.MemberProvision{}, fmt.Errorf("%w: member provision is not JSON", ErrKeycloakPayload)
	}
	email := domain.Email(p.Email)
	if !domain.ValidEmail(email) || domain.NormalizeEmail(p.Email) != email {
		return ports.MemberProvision{}, fmt.Errorf("%w: email", ErrKeycloakPayload)
	}
	return ports.MemberProvision{Email: email}, nil
}

func decodeMemberAccess(raw []byte) (ports.MemberAccess, error) {
	var p memberAccessPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return ports.MemberAccess{}, fmt.Errorf("%w: member access is not JSON", ErrKeycloakPayload)
	}
	a := ports.MemberAccess{Subject: domain.Subject(p.Subject), Roles: valuesOf[domain.RoleRef](p.Roles)}
	if !domain.ValidSubject(a.Subject) || unsafeSegment(p.Subject) {
		return ports.MemberAccess{}, fmt.Errorf("%w: subject", ErrKeycloakPayload)
	}
	switch p.Status {
	case accessActive:
		a.Status = domain.MembershipStatusActive
	case accessRemoved:
		a.Status = domain.MembershipStatusRemoved
	default:
		return ports.MemberAccess{}, fmt.Errorf("%w: membership status %q", ErrKeycloakPayload, p.Status)
	}
	for _, role := range a.Roles {
		if !domain.ValidRef(role) {
			return ports.MemberAccess{}, fmt.Errorf("%w: role ref", ErrKeycloakPayload)
		}
	}
	return a, nil
}

func decodeBuyerAccess(raw []byte) (ports.BuyerAccess, error) {
	var p buyerAccessPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return ports.BuyerAccess{}, fmt.Errorf("%w: buyer access is not JSON", ErrKeycloakPayload)
	}
	a := ports.BuyerAccess{Subject: domain.Subject(p.Subject)}
	if !domain.ValidSubject(a.Subject) || unsafeSegment(p.Subject) {
		return ports.BuyerAccess{}, fmt.Errorf("%w: subject", ErrKeycloakPayload)
	}
	switch p.Status {
	case accessActive:
		a.Status = domain.BuyerStatusActive
	case accessBlocked:
		a.Status = domain.BuyerStatusBlocked
	default:
		return ports.BuyerAccess{}, fmt.Errorf("%w: buyer status %q", ErrKeycloakPayload, p.Status)
	}
	return a, nil
}
