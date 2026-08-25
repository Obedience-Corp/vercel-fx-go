//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Obedience-Corp/vercel-fx-go/pkg/fx"
)

func asFxError(err error, target **fx.Error) bool {
	return errors.As(err, target)
}

func TestStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	status, err := newClient(t, "status").Status(ctx)
	skipIfProviderDown(t, err)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Kind != "status" || status.Model == "" {
		t.Fatalf("status %+v", status)
	}
	if status.Auth == "" {
		t.Fatal("fx reports no credential source; run fx login")
	}
	t.Logf("fx status: model=%s source=%s auth=%s team=%s permission_mode=%s",
		status.Model, status.ModelSource, status.Auth, status.Team, status.PermissionMode)
}

func TestModels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	catalog, err := newClient(t, "models").Models(ctx)
	skipIfProviderDown(t, err)
	if err != nil {
		t.Fatalf("models: %v", err)
	}
	if catalog.Count == 0 || len(catalog.IDs) == 0 {
		t.Fatalf("catalog %+v", catalog)
	}
	if catalog.IDs[0] == "" {
		t.Fatal("catalog contains an empty model id")
	}
	if len(catalog.Models) > 0 && catalog.Models[0].Source == "" {
		t.Fatal("provider-aware catalog contains an empty model source")
	}
	t.Logf("fx models: %d ids", catalog.Count)
}

func TestVersion(t *testing.T) {
	version, err := newClient(t, "status").Version(context.Background())
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if version != fx.TestedFXVersion {
		t.Fatalf("fx version %q, SDK contract target is %q", version, fx.TestedFXVersion)
	}
	t.Logf("fx version: %s", version)
}
