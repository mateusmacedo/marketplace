package rpc

import (
	"context"
	"unicode"
	"unicode/utf8"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/tenancy/contract/gen/go/company/tenancy/service/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

const maxTextRunes = 200

var descriptor = servicev1.File_company_tenancy_service_v1_tenancy_service_proto.Services().ByName("TenancyService")

// ServiceName is the full name the generated descriptor gives the service.
var ServiceName = string(descriptor.FullName())

func Methods() []string { return kernelgrpc.MethodNames(descriptor) }

// Commands lists the methods that require an idempotency key (IDM-01); the
// queries neither require nor read one.
func Commands() []string {
	return []string{"RegisterStore", "SuspendStore", "ReactivateStore"}
}

// FullMethod is the wire name of a method of the service: /<service>/<method>.
func FullMethod(name string) string { return kernelgrpc.FullMethod(ServiceName, name) }

// TenancyServer is the handler type ServiceDesc registers.
type TenancyServer interface {
	RegisterStore(context.Context, *servicev1.RegisterStoreRequest) (*servicev1.RegisterStoreResponse, error)
	SuspendStore(context.Context, *servicev1.SuspendStoreRequest) (*servicev1.SuspendStoreResponse, error)
	ReactivateStore(context.Context, *servicev1.ReactivateStoreRequest) (*servicev1.ReactivateStoreResponse, error)
	FindStore(context.Context, *servicev1.FindStoreRequest) (*servicev1.FindStoreResponse, error)
	FindStoresByStatus(context.Context, *servicev1.FindStoresByStatusRequest) (*servicev1.FindStoresByStatusResponse, error)
}

// ServiceDesc binds the service in the app block: protoc-gen-go-grpc would put
// io.network into the contract block, which admits only wire.codec (ADR-015).
var ServiceDesc = grpc.ServiceDesc{
	ServiceName: ServiceName,
	HandlerType: (*TenancyServer)(nil),
	Methods: []grpc.MethodDesc{
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "RegisterStore"), TenancyServer.RegisterStore),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "SuspendStore"), TenancyServer.SuspendStore),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "ReactivateStore"), TenancyServer.ReactivateStore),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "FindStore"), TenancyServer.FindStore),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "FindStoresByStatus"), TenancyServer.FindStoresByStatus),
	},
	Metadata: descriptor.ParentFile().Path(),
}

// Server realizes TenancyServer over the use cases of the context.
type Server struct {
	Service application.Service
}

// RegisterStore leaves the form of the ref to the domain, whose rejection the
// spec declares; only the name is shaped at the edge.
func (s Server) RegisterStore(ctx context.Context, req *servicev1.RegisterStoreRequest) (*servicev1.RegisterStoreResponse, error) {
	if !validText(req.GetName()) {
		return nil, invalidText("name")
	}
	return kernelgrpc.Command(ctx,
		func(ctx context.Context) (usecase.Outcome[domain.RegisteredResponse], error) {
			return s.Service.RegisterStore(ctx, application.RegisterStore{Ref: domain.StoreRef(req.GetRef()), Name: domain.StoreName(req.GetName())})
		},
		registerStoreRefused,
		storeRegisteredOf)
}

func (s Server) SuspendStore(ctx context.Context, req *servicev1.SuspendStoreRequest) (*servicev1.SuspendStoreResponse, error) {
	ref, err := storeRef(req.GetRef())
	if err != nil {
		return nil, err
	}
	if !validText(req.GetReason()) {
		return nil, invalidText("reason")
	}
	return kernelgrpc.Command(ctx,
		func(ctx context.Context) (usecase.Outcome[domain.SuspendedResponse], error) {
			return s.Service.SuspendStore(ctx, application.SuspendStore{Ref: ref, Reason: req.GetReason()})
		},
		suspendStoreRefused,
		storeSuspendedOf)
}

func (s Server) ReactivateStore(ctx context.Context, req *servicev1.ReactivateStoreRequest) (*servicev1.ReactivateStoreResponse, error) {
	ref, err := storeRef(req.GetRef())
	if err != nil {
		return nil, err
	}
	return kernelgrpc.Command(ctx,
		func(ctx context.Context) (usecase.Outcome[domain.ReactivatedResponse], error) {
			return s.Service.ReactivateStore(ctx, application.ReactivateStore{Ref: ref})
		},
		reactivateStoreRefused,
		storeReactivatedOf)
}

func (s Server) FindStore(ctx context.Context, req *servicev1.FindStoreRequest) (*servicev1.FindStoreResponse, error) {
	ref, err := storeRef(req.GetRef())
	if err != nil {
		return nil, err
	}
	if _, err := kernelgrpc.ExecutionOf(ctx); err != nil {
		return nil, err
	}
	snapshot, err := s.Service.FindStore(ctx, ref)
	if err != nil {
		return nil, kernelgrpc.StatusOf(err)
	}
	return &servicev1.FindStoreResponse{Store: storeOf(snapshot)}, nil
}

func (s Server) FindStoresByStatus(ctx context.Context, req *servicev1.FindStoresByStatusRequest) (*servicev1.FindStoresByStatusResponse, error) {
	wanted, ok := statusOf(req.GetStatus())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "status must be STORE_STATUS_ACTIVE or STORE_STATUS_SUSPENDED")
	}
	if _, err := kernelgrpc.ExecutionOf(ctx); err != nil {
		return nil, err
	}
	snapshots, err := s.Service.FindStoresByStatus(ctx, wanted)
	if err != nil {
		return nil, kernelgrpc.StatusOf(err)
	}
	stores := make([]*servicev1.Store, 0, len(snapshots))
	for _, snapshot := range snapshots {
		stores = append(stores, storeOf(snapshot))
	}
	return &servicev1.FindStoresByStatusResponse{Stores: stores}, nil
}

func registerStoreRefused(r *kernel.Rejection) *servicev1.RegisterStoreResponse {
	return &servicev1.RegisterStoreResponse{Result: &servicev1.RegisterStoreResponse_Rejection{Rejection: rejectionOf(r)}}
}

func storeRegisteredOf(registered domain.RegisteredResponse) *servicev1.RegisterStoreResponse {
	return &servicev1.RegisterStoreResponse{Result: &servicev1.RegisterStoreResponse_Registered{
		Registered: &servicev1.Registered{Ref: string(registered.Ref)},
	}}
}

func suspendStoreRefused(r *kernel.Rejection) *servicev1.SuspendStoreResponse {
	return &servicev1.SuspendStoreResponse{Result: &servicev1.SuspendStoreResponse_Rejection{Rejection: rejectionOf(r)}}
}

func storeSuspendedOf(suspended domain.SuspendedResponse) *servicev1.SuspendStoreResponse {
	return &servicev1.SuspendStoreResponse{Result: &servicev1.SuspendStoreResponse_Suspended{
		Suspended: &servicev1.Suspended{Ref: string(suspended.Ref)},
	}}
}

func reactivateStoreRefused(r *kernel.Rejection) *servicev1.ReactivateStoreResponse {
	return &servicev1.ReactivateStoreResponse{Result: &servicev1.ReactivateStoreResponse_Rejection{Rejection: rejectionOf(r)}}
}

func storeReactivatedOf(reactivated domain.ReactivatedResponse) *servicev1.ReactivateStoreResponse {
	return &servicev1.ReactivateStoreResponse{Result: &servicev1.ReactivateStoreResponse_Reactivated{
		Reactivated: &servicev1.Reactivated{Ref: string(reactivated.Ref)},
	}}
}

func storeRef(raw string) (domain.StoreRef, error) {
	ref := domain.StoreRef(raw)
	if !domain.ValidRef(ref) {
		return "", status.Error(codes.InvalidArgument, "ref must have 3 to 63 characters of a-z, 0-9 and -, without a leading or trailing hyphen")
	}
	return ref, nil
}

func invalidText(field string) error {
	return status.Error(codes.InvalidArgument, field+" must have 1 to 200 characters of valid UTF-8 without control, format, line or paragraph separator, variation selector or default ignorable characters and with at least one character that is neither a space nor a combining mark")
}

var refusedInText = []*unicode.RangeTable{
	unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp, unicode.Variation_Selector, unicode.Other_Default_Ignorable_Code_Point,
}

// validText is the name rule of the golden bookings, which the spec applies to
// the name and to the reason of a suspension alike.
func validText(text string) bool {
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > maxTextRunes {
		return false
	}
	visible := false
	for _, r := range text {
		if unicode.In(r, refusedInText...) {
			return false
		}
		visible = visible || !unicode.IsSpace(r) && !unicode.IsMark(r)
	}
	return visible
}

func storeOf(s domain.StoreSnapshot) *servicev1.Store {
	return &servicev1.Store{
		Ref:              string(s.Ref),
		Name:             string(s.Name),
		Status:           storeStatus(s.Status),
		SuspensionReason: s.SuspensionReason,
		RegisteredAt:     int64(s.RegisteredAt),
		StatusChangedAt:  int64(s.StatusChangedAt),
	}
}

func storeStatus(s domain.StoreStatus) servicev1.StoreStatus {
	switch s {
	case domain.Active:
		return servicev1.StoreStatus_STORE_STATUS_ACTIVE
	case domain.Suspended:
		return servicev1.StoreStatus_STORE_STATUS_SUSPENDED
	default:
		return servicev1.StoreStatus_STORE_STATUS_UNSPECIFIED
	}
}

func statusOf(s servicev1.StoreStatus) (domain.StoreStatus, bool) {
	switch s {
	case servicev1.StoreStatus_STORE_STATUS_ACTIVE:
		return domain.Active, true
	case servicev1.StoreStatus_STORE_STATUS_SUSPENDED:
		return domain.Suspended, true
	default:
		return domain.Unregistered, false
	}
}

func rejectionOf(r *kernel.Rejection) *servicev1.Rejection {
	return &servicev1.Rejection{Code: string(r.Code()), Message: r.Message()}
}
