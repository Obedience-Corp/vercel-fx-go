package fx

import (
	"strconv"
	"strings"
	"time"
)

// PermissionMode is the fx permission mode for a spawned process.
type PermissionMode string

// Permission modes accepted by FX_PERMISSION_MODE.
const (
	PermissionUnset PermissionMode = ""
	PermissionAsk   PermissionMode = "ask"
	PermissionAuto  PermissionMode = "auto"
	PermissionYolo  PermissionMode = "yolo"
)

// AskOptions configures a single "fx ask" run.
type AskOptions struct {
	Model              string
	PermissionMode     PermissionMode
	MaxAgentSteps      *int
	Auto               bool
	Yolo               bool
	AllowDangerousMode bool
	Images             []string
	Quiet              bool
	NoSave             bool
	NoColor            bool
	Resume             string
	ResumeID           string
	ContinueRecovery   bool
	AddDirs            []string
	NoAdditionalDirs   bool
	ContextLimits      map[string]string
	WorkingDirectory   string
	Timeout            time.Duration
	Env                []string
	RetryPolicy        *RetryPolicy
}

// Validate reports configuration that fx would reject or that the SDK refuses.
func (o *AskOptions) Validate() error {
	if err := o.validate(); err != nil {
		return err
	}
	return nil
}

func (o *AskOptions) validate() *Error {
	if o == nil {
		return nil
	}
	if err := o.validateDangerous(); err != nil {
		return err
	}
	if err := o.validateSession(); err != nil {
		return err
	}
	if err := validateContextLimits(o.ContextLimits); err != nil {
		return err
	}
	if err := validatePermissionMode(o.PermissionMode); err != nil {
		return err
	}
	return validateImages(o.Images)
}

func (o *AskOptions) validateDangerous() *Error {
	if o.AllowDangerousMode {
		return nil
	}
	if o.Yolo {
		return validationError("Yolo requires AllowDangerousMode; use the dangerous subpackage")
	}
	if o.PermissionMode == PermissionYolo {
		return validationError("PermissionMode \"yolo\" requires AllowDangerousMode; use the dangerous subpackage")
	}
	return nil
}

func (o *AskOptions) validateSession() *Error {
	if o.NoSave && (o.Resume != "" || o.ResumeID != "") {
		return validationError("NoSave conflicts with Resume and ResumeID")
	}
	if o.Resume != "" && o.ResumeID != "" {
		return validationError("Resume and ResumeID are mutually exclusive")
	}
	return nil
}

func validatePermissionMode(mode PermissionMode) *Error {
	switch mode {
	case PermissionUnset, PermissionAsk, PermissionAuto, PermissionYolo:
		return nil
	}
	return validationError("unknown permission mode " + strconv.Quote(string(mode)))
}

func validateContextLimits(limits map[string]string) *Error {
	for name, value := range limits {
		if name == "" {
			return validationError("context limit name must not be empty")
		}
		if value == "off" {
			continue
		}
		if _, err := strconv.ParseUint(value, 10, 64); err != nil {
			return validationErrorWith("context limit "+strconv.Quote(name)+" must be a byte count or \"off\"", err)
		}
	}
	return nil
}

func validateImages(images []string) *Error {
	for _, image := range images {
		if strings.TrimSpace(image) == "" {
			return validationError("image path must not be empty")
		}
	}
	return nil
}

func cloneAskOptions(o *AskOptions) *AskOptions {
	if o == nil {
		return &AskOptions{}
	}
	out := *o
	out.Images = append([]string(nil), o.Images...)
	out.AddDirs = append([]string(nil), o.AddDirs...)
	out.Env = append([]string(nil), o.Env...)
	if o.MaxAgentSteps != nil {
		steps := *o.MaxAgentSteps
		out.MaxAgentSteps = &steps
	}
	if o.ContextLimits != nil {
		out.ContextLimits = make(map[string]string, len(o.ContextLimits))
		for k, v := range o.ContextLimits {
			out.ContextLimits[k] = v
		}
	}
	if o.RetryPolicy != nil {
		policy := *o.RetryPolicy
		out.RetryPolicy = &policy
	}
	return &out
}
