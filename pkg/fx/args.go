package fx

import (
	"sort"
	"strconv"
)

// BuildAskArgs renders the argv for "fx ask", leading global flags first and
// the prompt after a "--" separator. An empty prompt selects stdin mode.
func BuildAskArgs(prompt string, opts *AskOptions) []string {
	args := leadingGlobalArgs(opts.globals())
	args = append(args, "ask")
	args = append(args, askFlagArgs(opts)...)
	if prompt != "" {
		args = append(args, "--", prompt)
	}
	return args
}

// BuildACPArgs renders the argv for "fx acp".
func BuildACPArgs(cfg *ACPConfig) []string {
	args := leadingGlobalArgs(cfg.globals())
	args = append(args, "acp")
	if cfg == nil {
		return args
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.LogFile != "" {
		args = append(args, "--log-file", cfg.LogFile)
	}
	return args
}

// BuildEnv renders the FX_* overrides the SDK manages for a run. The upgrade
// check and the browser launcher are always disabled.
func BuildEnv(opts *AskOptions) []string {
	if opts == nil {
		return managedEnv("", PermissionUnset, nil, nil)
	}
	return managedEnv(opts.Model, opts.PermissionMode, opts.MaxAgentSteps, opts.Env)
}

type globalFlags struct {
	addDirs          []string
	noAdditionalDirs bool
	contextLimits    map[string]string
}

func (o *AskOptions) globals() globalFlags {
	if o == nil {
		return globalFlags{}
	}
	return globalFlags{addDirs: o.AddDirs, noAdditionalDirs: o.NoAdditionalDirs, contextLimits: o.ContextLimits}
}

func (c *ACPConfig) globals() globalFlags {
	if c == nil {
		return globalFlags{}
	}
	return globalFlags{addDirs: c.AddDirs, noAdditionalDirs: c.NoAdditionalDirs, contextLimits: c.ContextLimits}
}

func leadingGlobalArgs(g globalFlags) []string {
	args := make([]string, 0, 8)
	for _, dir := range g.addDirs {
		args = append(args, "--add-dir", dir)
	}
	if g.noAdditionalDirs {
		args = append(args, "--no-additional-dirs")
	}
	for _, name := range sortedKeys(g.contextLimits) {
		args = append(args, "--context-limit", name+"="+g.contextLimits[name])
	}
	return args
}

func askFlagArgs(opts *AskOptions) []string {
	args := make([]string, 0, 12)
	if opts == nil {
		return append(args, "--json")
	}
	if opts.Auto {
		args = append(args, "--auto")
	}
	if opts.Yolo {
		args = append(args, "--yolo")
	}
	for _, image := range opts.Images {
		args = append(args, "--image", image)
	}
	args = append(args, "--json")
	args = append(args, askToggleArgs(opts)...)
	return append(args, askSessionArgs(opts)...)
}

func askToggleArgs(opts *AskOptions) []string {
	args := make([]string, 0, 3)
	if opts.Quiet {
		args = append(args, "--quiet")
	}
	if opts.NoSave {
		args = append(args, "--no-save")
	}
	if opts.NoColor {
		args = append(args, "--no-color")
	}
	return args
}

func askSessionArgs(opts *AskOptions) []string {
	args := make([]string, 0, 5)
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	}
	if opts.ResumeID != "" {
		args = append(args, "--resume-id", opts.ResumeID)
	}
	if opts.ContinueRecovery {
		args = append(args, "--continue-recovery")
	}
	return args
}

func managedEnv(model string, mode PermissionMode, maxSteps *int, extra []string) []string {
	env := append([]string(nil), extra...)
	if model != "" {
		env = append(env, "FX_MODEL="+model)
	}
	if mode != PermissionUnset {
		env = append(env, "FX_PERMISSION_MODE="+string(mode))
	}
	if maxSteps != nil {
		env = append(env, "FX_MAX_AGENT_STEPS="+strconv.Itoa(*maxSteps))
	}
	return append(env, "FX_AUTO_UPGRADE=0", "FX_NO_OPEN_BROWSER=1")
}

func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
