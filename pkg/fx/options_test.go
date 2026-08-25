package fx

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func TestAskOptionsValidateErrors(t *testing.T) {
	negativeSteps := -1
	tests := []struct {
		name    string
		opts    *AskOptions
		wantSub string
	}{
		{
			name:    "yolo flag without the dangerous acknowledgement",
			opts:    &AskOptions{Yolo: true},
			wantSub: "Yolo requires AllowDangerousMode",
		},
		{
			name:    "yolo permission mode without the dangerous acknowledgement",
			opts:    &AskOptions{PermissionMode: PermissionYolo},
			wantSub: "requires AllowDangerousMode",
		},
		{
			name:    "no-save with resume",
			opts:    &AskOptions{NoSave: true, Resume: "last"},
			wantSub: "NoSave conflicts with Resume",
		},
		{
			name:    "no-save with resume id",
			opts:    &AskOptions{NoSave: true, ResumeID: "abc"},
			wantSub: "NoSave conflicts with Resume",
		},
		{
			name:    "resume and resume id together",
			opts:    &AskOptions{Resume: "last", ResumeID: "abc"},
			wantSub: "mutually exclusive",
		},
		{
			name:    "unknown permission mode",
			opts:    &AskOptions{PermissionMode: PermissionMode("wild")},
			wantSub: "unknown permission mode",
		},
		{
			name:    "context limit is neither a byte count nor off",
			opts:    &AskOptions{ContextLimits: map[string]string{"file": "big"}},
			wantSub: "must be a byte count",
		},
		{
			name:    "empty context limit name",
			opts:    &AskOptions{ContextLimits: map[string]string{"": "1024"}},
			wantSub: "name must not be empty",
		},
		{
			name:    "blank image path",
			opts:    &AskOptions{Images: []string{"  "}},
			wantSub: "image path must not be empty",
		},
		{name: "negative max steps", opts: &AskOptions{MaxAgentSteps: &negativeSteps}, wantSub: "MaxAgentSteps"},
		{name: "negative timeout", opts: &AskOptions{Timeout: -time.Second}, wantSub: "Timeout"},
		{name: "negative retry attempts", opts: &AskOptions{RetryPolicy: &RetryPolicy{MaxAttempts: -1}}, wantSub: "MaxAttempts"},
		{name: "negative retry delay", opts: &AskOptions{RetryPolicy: &RetryPolicy{InitialDelay: -time.Second}}, wantSub: "delays"},
		{name: "invalid retry multiplier", opts: &AskOptions{RetryPolicy: &RetryPolicy{Multiplier: math.Inf(1)}}, wantSub: "Multiplier"},
		{name: "decreasing retry multiplier", opts: &AskOptions{RetryPolicy: &RetryPolicy{Multiplier: 0.5}}, wantSub: "Multiplier"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.Validate()
			fxErr := requireFxError(t, err, KindValidation)
			if !strings.Contains(fxErr.Message, tc.wantSub) {
				t.Fatalf("message %q does not contain %q", fxErr.Message, tc.wantSub)
			}
		})
	}
}

func TestRetryDelaySaturatesInsteadOfOverflowing(t *testing.T) {
	policy := &RetryPolicy{InitialDelay: time.Hour, Multiplier: math.MaxFloat64}
	got := policy.delayFor(2, &Error{})
	if got != time.Duration(math.MaxInt64) {
		t.Fatalf("delay %v, want saturation at %v", got, time.Duration(math.MaxInt64))
	}
}

func TestAskOptionsValidateAccepts(t *testing.T) {
	steps := 3
	tests := []struct {
		name string
		opts *AskOptions
	}{
		{name: "nil options", opts: nil},
		{name: "empty options", opts: &AskOptions{}},
		{name: "yolo with the acknowledgement", opts: &AskOptions{Yolo: true, PermissionMode: PermissionYolo, AllowDangerousMode: true}},
		{name: "context limit off", opts: &AskOptions{ContextLimits: map[string]string{"file": "off"}}},
		{name: "steps and model", opts: &AskOptions{Model: "zai/glm-5.2", MaxAgentSteps: &steps, Timeout: time.Second}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.opts.Validate(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestAskOptionsValidateReturnsUntypedNil(t *testing.T) {
	var opts *AskOptions
	if err := opts.Validate(); err != nil {
		t.Fatalf("nil options must validate cleanly, got %v (%T)", err, err)
	}
	if err := (&AskOptions{}).Validate(); err != nil {
		t.Fatalf("empty options must validate cleanly, got %v (%T)", err, err)
	}
}

func TestCloneDoesNotAliasCallerState(t *testing.T) {
	steps := 4
	original := &AskOptions{
		Images:        []string{"a.png"},
		AddDirs:       []string{"/a"},
		Env:           []string{"FX_TRACE=1"},
		ContextLimits: map[string]string{"file": "1024"},
		MaxAgentSteps: &steps,
		RetryPolicy:   DefaultRetryPolicy(),
	}
	clone := original.Clone()
	clone.Images[0] = "b.png"
	clone.AddDirs[0] = "/b"
	clone.Env[0] = "FX_TRACE=0"
	clone.ContextLimits["file"] = "2048"
	*clone.MaxAgentSteps = 9
	clone.RetryPolicy.MaxAttempts = 99

	if original.Images[0] != "a.png" || original.AddDirs[0] != "/a" || original.Env[0] != "FX_TRACE=1" {
		t.Fatal("clone aliased a caller slice")
	}
	if original.ContextLimits["file"] != "1024" {
		t.Fatal("clone aliased the caller context limits")
	}
	if *original.MaxAgentSteps != 4 {
		t.Fatal("clone aliased the caller step pointer")
	}
	if original.RetryPolicy.MaxAttempts == 99 {
		t.Fatal("clone aliased the caller retry policy")
	}
}

func TestACPConfigValidateErrors(t *testing.T) {
	negativeSteps := -1
	tests := []struct {
		name    string
		cfg     *ACPConfig
		wantSub string
	}{
		{name: "yolo without the acknowledgement", cfg: &ACPConfig{PermissionMode: PermissionYolo}, wantSub: "AllowDangerousMode"},
		{name: "relative log file", cfg: &ACPConfig{LogFile: "fx.log"}, wantSub: "absolute path"},
		{name: "bad context limit", cfg: &ACPConfig{ContextLimits: map[string]string{"file": "huge"}}, wantSub: "byte count"},
		{name: "unknown permission mode", cfg: &ACPConfig{PermissionMode: PermissionMode("nope")}, wantSub: "unknown permission mode"},
		{name: "negative max steps", cfg: &ACPConfig{MaxAgentSteps: &negativeSteps}, wantSub: "MaxAgentSteps"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.validate()
			if err == nil {
				t.Fatal("expected a validation error")
			}
			if err.Kind != KindValidation || !strings.Contains(err.Message, tc.wantSub) {
				t.Fatalf("got %v, want validation containing %q", err, tc.wantSub)
			}
		})
	}
}

func TestErrorUnwrapSupportsErrorsIs(t *testing.T) {
	sentinel := errors.New("boom")
	err := transportError("spawn", sentinel)
	if !errors.Is(err, sentinel) {
		t.Fatal("errors.Is did not see the wrapped cause")
	}
	var target *Error
	if !errors.As(error(err), &target) {
		t.Fatal("errors.As did not match *fx.Error")
	}
}
