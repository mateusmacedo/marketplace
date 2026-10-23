package app_test

import (
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/obstest"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app"
)

const manifestPath = "../deploy/.env.example"

func TestTheTelemetryManifest(t *testing.T) {
	obstest.Manifest[app.Config, string]{Subject: edgeTelemetry, EnvFile: manifestPath}.Run(t)
}
