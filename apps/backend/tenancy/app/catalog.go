package app

import (
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/channel"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
)

const (
	storeRegisteredEventType = "com.company.tenancy.store-registered"
	storeRegisteredContract  = "company/tenancy/event/v1/store_registered.proto#StoreRegistered"
)

// TenancyChannel is the channel the relay publishes to, named after the
// destination the use case authors, because the publisher resolves by it.
func TenancyChannel(cfg Config) channel.Channel {
	return channel.NewKafka(application.Destination, cfg.TenancyTopic, cfg.TenancyDLQ, cfg.Group,
		storeRegisteredEventType, storeRegisteredContract)
}
