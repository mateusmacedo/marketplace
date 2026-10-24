package api

import (
	"net/http"
	"regexp"
	"unicode"
	"unicode/utf8"

	tenancyv1 "github.com/mateusmacedo/marketplace/apps/backend/tenancy/contract/gen/go/company/tenancy/service/v1"
)

const maxNameRunes = 200

var (
	refForm = form{
		valid: regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`).MatchString,
		shape: "have 3 to 63 characters of a-z, 0-9 and -, without a leading or trailing hyphen",
	}
	statusForm = form{
		valid: func(status string) bool { _, known := storeStatusOf[status]; return known },
		shape: "be active or suspended",
	}
)

type registerStoreRequest struct {
	Ref  string `json:"ref"`
	Name string `json:"name"`
}

type suspendStoreRequest struct {
	Reason string `json:"reason"`
}

type storeRefResponse struct {
	Ref string `json:"ref"`
}

type storeView struct {
	Ref              string `json:"ref"`
	Name             string `json:"name"`
	Status           string `json:"status"`
	SuspensionReason string `json:"suspensionReason"`
	RegisteredAt     int64  `json:"registeredAt"`
	StatusChangedAt  int64  `json:"statusChangedAt"`
}

var (
	storeStatuses = map[tenancyv1.StoreStatus]string{
		tenancyv1.StoreStatus_STORE_STATUS_ACTIVE:    "active",
		tenancyv1.StoreStatus_STORE_STATUS_SUSPENDED: "suspended",
	}
	storeStatusOf = map[string]tenancyv1.StoreStatus{
		"active":    tenancyv1.StoreStatus_STORE_STATUS_ACTIVE,
		"suspended": tenancyv1.StoreStatus_STORE_STATUS_SUSPENDED,
	}
)

const textRule = "1 to 200 characters without control, format, line or paragraph separator, variation selector or default ignorable characters and with at least one character that is neither a space nor a combining mark"

// WHY: the ref of a registration is left to the context, which refuses any
// other form with tenancy/store/ref-invalid (422); only the ref of a path is
// shaped here, as the edge shapes every id it routes by.
func (h handlers) registerStore() http.HandlerFunc {
	return endpoint(
		fromBody("RegisterRequest", "name must have "+textRule, validRegisterStore, registerStoreRequestOf),
		h.tenancy.RegisterStore,
		outcome(http.StatusCreated, storeRegisteredOf))
}

func validRegisterStore(b registerStoreRequest) bool {
	return validName(b.Name)
}

var refusedInName = []*unicode.RangeTable{
	unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp, unicode.Variation_Selector, unicode.Other_Default_Ignorable_Code_Point,
}

func validName(name string) bool {
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > maxNameRunes {
		return false
	}
	visible := false
	for _, r := range name {
		if unicode.In(r, refusedInName...) {
			return false
		}
		visible = visible || !unicode.IsSpace(r) && !unicode.IsMark(r)
	}
	return visible
}

func registerStoreRequestOf(b registerStoreRequest) *tenancyv1.RegisterStoreRequest {
	return &tenancyv1.RegisterStoreRequest{Ref: b.Ref, Name: b.Name}
}

func storeRegisteredOf(resp *tenancyv1.RegisterStoreResponse) (any, refusal) {
	switch result := resp.GetResult().(type) {
	case *tenancyv1.RegisterStoreResponse_Registered:
		return storeRefResponse{Ref: result.Registered.GetRef()}, nil
	case *tenancyv1.RegisterStoreResponse_Rejection:
		return nil, result.Rejection
	default:
		return nil, nil
	}
}

func (h handlers) suspendStore() http.HandlerFunc {
	return endpoint(
		fromPathAndBody("ref", refForm, "SuspendRequest", "reason must have "+textRule, validSuspendStore, suspendStoreRequestOf),
		h.tenancy.SuspendStore,
		outcome(http.StatusOK, storeSuspendedOf))
}

func validSuspendStore(b suspendStoreRequest) bool {
	return validName(b.Reason)
}

func suspendStoreRequestOf(ref string, b suspendStoreRequest) *tenancyv1.SuspendStoreRequest {
	return &tenancyv1.SuspendStoreRequest{Ref: ref, Reason: b.Reason}
}

func storeSuspendedOf(resp *tenancyv1.SuspendStoreResponse) (any, refusal) {
	switch result := resp.GetResult().(type) {
	case *tenancyv1.SuspendStoreResponse_Suspended:
		return storeRefResponse{Ref: result.Suspended.GetRef()}, nil
	case *tenancyv1.SuspendStoreResponse_Rejection:
		return nil, result.Rejection
	default:
		return nil, nil
	}
}

func (h handlers) reactivateStore() http.HandlerFunc {
	return endpoint(fromPath("ref", refForm, reactivateStoreRequest), h.tenancy.ReactivateStore, outcome(http.StatusOK, storeReactivatedOf))
}

func reactivateStoreRequest(ref string) *tenancyv1.ReactivateStoreRequest {
	return &tenancyv1.ReactivateStoreRequest{Ref: ref}
}

func storeReactivatedOf(resp *tenancyv1.ReactivateStoreResponse) (any, refusal) {
	switch result := resp.GetResult().(type) {
	case *tenancyv1.ReactivateStoreResponse_Reactivated:
		return storeRefResponse{Ref: result.Reactivated.GetRef()}, nil
	case *tenancyv1.ReactivateStoreResponse_Rejection:
		return nil, result.Rejection
	default:
		return nil, nil
	}
}

func (h handlers) findStore() http.HandlerFunc {
	return endpoint(fromPath("ref", refForm, findStoreRequest), h.tenancy.FindStore, view(storeViewOf))
}

func findStoreRequest(ref string) *tenancyv1.FindStoreRequest {
	return &tenancyv1.FindStoreRequest{Ref: ref}
}

func storeViewOf(resp *tenancyv1.FindStoreResponse) (any, bool) {
	return viewOfStore(resp.GetStore())
}

func (h handlers) findStoresByStatus() http.HandlerFunc {
	return endpoint(fromQuery("status", statusForm, findStoresByStatusRequest), h.tenancy.FindStoresByStatus, view(storeViewsOf))
}

func findStoresByStatusRequest(status string) *tenancyv1.FindStoresByStatusRequest {
	return &tenancyv1.FindStoresByStatusRequest{Status: storeStatusOf[status]}
}

func storeViewsOf(resp *tenancyv1.FindStoresByStatusResponse) (any, bool) {
	views := make([]storeView, 0, len(resp.GetStores()))
	for _, store := range resp.GetStores() {
		v, known := viewOfStore(store)
		if !known {
			return nil, false
		}
		views = append(views, v)
	}
	return views, true
}

func viewOfStore(s *tenancyv1.Store) (storeView, bool) {
	status, known := storeStatuses[s.GetStatus()]
	return storeView{
		Ref:              s.GetRef(),
		Name:             s.GetName(),
		Status:           status,
		SuspensionReason: s.GetSuspensionReason(),
		RegisteredAt:     s.GetRegisteredAt(),
		StatusChangedAt:  s.GetStatusChangedAt(),
	}, known
}
