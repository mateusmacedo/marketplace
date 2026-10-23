package domain_test

import (
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func TestDeclaredCodesAreValid(t *testing.T) {
	codes := map[string]kernel.Code{
		"CodeStoreRefInvalid":    domain.CodeStoreRefInvalid,
		"CodeStoreNameEmpty":     domain.CodeStoreNameEmpty,
		"CodeStoreReasonEmpty":   domain.CodeStoreReasonEmpty,
		"CodeStoreNotActive":     domain.CodeStoreNotActive,
		"CodeStoreNotSuspended":  domain.CodeStoreNotSuspended,
		"CodeStoreDuplicateName": domain.CodeStoreDuplicateName,
	}
	for name, code := range codes {
		t.Run(name, func(t *testing.T) {
			if !code.Valid() {
				t.Fatalf("%s = %q is not a valid context/reason code", name, code)
			}
		})
	}
}
