package dangerous

import (
	"context"
	"encoding/json"
	"os"

	"github.com/Obedience-Corp/vercel-fx-go/pkg/fx"
)

// EnableEnv must equal EnableValue before this package will do anything.
const (
	EnableEnv   = "FX_GO_ENABLE_DANGEROUS"
	EnableValue = "i-accept-all-risks"
)

// Errors returned when the guard rails refuse.
var (
	ErrNotEnabled = &fx.Error{Kind: fx.KindValidation, Message: "dangerous: " + EnableEnv + " must be set to \"" + EnableValue + "\""}
	ErrProduction = &fx.Error{Kind: fx.KindValidation, Message: "dangerous: refusing to run in a production environment"}
)

// Client wraps an fx client with the guarded dangerous entry points.
type Client struct {
	inner *fx.Client
}

// NewDangerousClient builds a guarded client for an explicit binary path.
func NewDangerousClient(binPath string) (*Client, error) {
	if err := Enabled(); err != nil {
		return nil, err
	}
	return &Client{inner: fx.NewClient(binPath)}, nil
}

// NewDangerousClientFromPath locates fx and builds a guarded client.
func NewDangerousClientFromPath() (*Client, error) {
	if err := Enabled(); err != nil {
		return nil, err
	}
	inner, err := fx.NewClientFromPath()
	if err != nil {
		return nil, err
	}
	return &Client{inner: inner}, nil
}

// Wrap guards an existing client.
func Wrap(inner *fx.Client) (*Client, error) {
	if inner == nil {
		return nil, &fx.Error{Kind: fx.KindValidation, Message: "dangerous: client must not be nil"}
	}
	if err := Enabled(); err != nil {
		return nil, err
	}
	return &Client{inner: inner}, nil
}

// Unwrap returns the underlying client.
func (c *Client) Unwrap() *fx.Client { return c.inner }

// Enabled reports whether the environment permits the dangerous entry points.
func Enabled() error {
	if err := checkEnabled(); err != nil {
		return err
	}
	return nil
}

func checkEnabled() *fx.Error {
	if os.Getenv(EnableEnv) != EnableValue {
		return ErrNotEnabled
	}
	if os.Getenv("GO_ENV") == "production" || os.Getenv("NODE_ENV") == "production" {
		return ErrProduction
	}
	return nil
}

// AskOptions returns a copy of opts with yolo mode enabled: the --yolo flag
// and FX_PERMISSION_MODE=yolo. Permission checks are disabled.
func AskOptions(opts *fx.AskOptions) (*fx.AskOptions, error) {
	if err := checkEnabled(); err != nil {
		return nil, err
	}
	out := &fx.AskOptions{}
	if opts != nil {
		out = opts.Clone()
	}
	out.Yolo = true
	out.PermissionMode = fx.PermissionYolo
	out.AllowDangerousMode = true
	return out, nil
}

// ACPConfig returns a copy of cfg with yolo mode enabled for the acp process.
func ACPConfig(cfg *fx.ACPConfig) (*fx.ACPConfig, error) {
	if err := checkEnabled(); err != nil {
		return nil, err
	}
	out := &fx.ACPConfig{}
	if cfg != nil {
		clone := *cfg
		out = &clone
	}
	out.PermissionMode = fx.PermissionYolo
	out.AllowDangerousMode = true
	return out, nil
}

// Yolo runs one "fx ask" with permission checks disabled.
func (c *Client) Yolo(ctx context.Context, prompt string, opts *fx.AskOptions) (*fx.AskResult, error) {
	prepared, err := AskOptions(opts)
	if err != nil {
		return nil, err
	}
	return c.inner.AskCtx(ctx, prompt, prepared)
}

// StartACP starts an acp session with permission checks disabled.
func (c *Client) StartACP(ctx context.Context, cfg *fx.ACPConfig) (*fx.ACPSession, error) {
	prepared, err := ACPConfig(cfg)
	if err != nil {
		return nil, err
	}
	return c.inner.StartACP(ctx, prepared)
}

// UpgradeCheck runs "fx upgrade --json". It can replace the installed binary,
// which is why it lives here. The reply shape is version dependent.
func (c *Client) UpgradeCheck(ctx context.Context) (json.RawMessage, error) {
	if err := checkEnabled(); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if err := c.inner.RunJSON(ctx, &raw, "upgrade", "--json"); err != nil {
		return nil, err
	}
	return raw, nil
}
