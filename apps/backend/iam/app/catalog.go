package app

import (
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/channel"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
)

const (
	roleDefinedEventType = "com.company.iam.role-defined"
	roleDefinedContract  = "company/iam/event/v1/role_defined.proto#RoleDefined"

	invitationProvisionedEventType = "com.company.iam.invitation-provisioned"
	invitationProvisionedContract  = "company/iam/event/v1/invitation_provisioned.proto#InvitationProvisioned"
)

// IamChannel is the channel the relay publishes to, named after the
// destination the use case authors, because the publisher resolves by it.
func IamChannel(cfg Config) channel.Channel {
	return channel.NewKafka(application.Destination, cfg.IamTopic, cfg.IamDLQ, cfg.Group,
		roleDefinedEventType, roleDefinedContract)
}

// MembershipChannel is the same topic as the consumer reads it: the context
// consumes its own provisioned invitations and leaves every other event alone.
func MembershipChannel(cfg Config) channel.Channel {
	return channel.NewKafka(application.Destination, cfg.IamTopic, cfg.IamDLQ, cfg.Group,
		invitationProvisionedEventType, invitationProvisionedContract)
}
