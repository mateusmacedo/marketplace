package domain

import kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

const (
	CodeStoreRefInvalid    kernel.Code = "tenancy/store/ref-invalid"
	CodeStoreNameEmpty     kernel.Code = "tenancy/store/name-empty"
	CodeStoreReasonEmpty   kernel.Code = "tenancy/store/reason-empty"
	CodeStoreNotActive     kernel.Code = "tenancy/store/not-active"
	CodeStoreNotSuspended  kernel.Code = "tenancy/store/not-suspended"
	CodeStoreDuplicateName kernel.Code = "tenancy/store/duplicate-name"
)

var RejectStoreDuplicateName = kernel.Reject(CodeStoreDuplicateName, "a store with this name already exists in the tenant")
