package fx

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"regexp"
	"sync"
	"syscall"
	"time"
)

// LoginCommand returns a configured "fx login" command for the host to attach
// a terminal to. The SDK never runs it, because the flow is interactive.
func (c *Client) LoginCommand(ctx context.Context) (*exec.Cmd, error) {
	return c.interactiveCommand(ctx, "login")
}

// LoginProviderCommand returns "fx login <provider>" for vercel, codex, or grok.
func (c *Client) LoginProviderCommand(ctx context.Context, provider string) (*exec.Cmd, error) {
	if !validAuthProvider(provider) {
		return nil, validationError("login provider must be \"vercel\", \"codex\", or \"grok\"")
	}
	return c.interactiveCommand(ctx, "login", provider)
}

// SetupCommand returns a configured "fx setup" command for an AI Gateway key.
func (c *Client) SetupCommand(ctx context.Context) (*exec.Cmd, error) {
	return c.interactiveCommand(ctx, "setup")
}

// TeamsCommand returns a configured "fx teams" command for team selection.
func (c *Client) TeamsCommand(ctx context.Context) (*exec.Cmd, error) {
	return c.interactiveCommand(ctx, "teams")
}

// LogoutCommand returns a configured "fx logout" command.
func (c *Client) LogoutCommand(ctx context.Context) (*exec.Cmd, error) {
	return c.interactiveCommand(ctx, "logout")
}

// LogoutProviderCommand returns "fx logout <provider>" for vercel, codex, or grok.
func (c *Client) LogoutProviderCommand(ctx context.Context, provider string) (*exec.Cmd, error) {
	if !validAuthProvider(provider) {
		return nil, validationError("logout provider must be \"vercel\", \"codex\", or \"grok\"")
	}
	return c.interactiveCommand(ctx, "logout", provider)
}

// ProviderCommand returns "fx provider <provider>" for gateway, codex, or grok.
func (c *Client) ProviderCommand(ctx context.Context, provider string) (*exec.Cmd, error) {
	if !validSessionProvider(provider) {
		return nil, validationError("provider must be \"gateway\", \"codex\", or \"grok\"")
	}
	return c.interactiveCommand(ctx, "provider", provider)
}

func (c *Client) interactiveCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	dir, err := c.workDir("")
	if err != nil {
		return nil, err
	}
	cmd := c.command(ctx, args...)
	cmd.Dir = dir
	cmd.Env = c.envWith(managedEnv("", PermissionUnset, nil, nil))
	return cmd, nil
}

func validAuthProvider(provider string) bool {
	return provider == "vercel" || provider == "codex" || provider == "grok"
}

func validSessionProvider(provider string) bool {
	return provider == "gateway" || provider == "codex" || provider == "grok"
}

// LoginFlow is a running "fx login" whose authorization URL has been captured.
// The caller shows URL to the user and then waits for the flow to finish.
type LoginFlow struct {
	URL string

	cmd      *exec.Cmd
	waitDone chan error
	done     chan struct{}
	waitOnce sync.Once
	stopOnce sync.Once
	err      error
}

// Wait blocks until the login process exits. It is safe to call more than once.
func (f *LoginFlow) Wait() error {
	f.waitOnce.Do(func() {
		f.err = <-f.waitDone
		close(f.done)
	})
	<-f.done
	return f.err
}

// Close stops the login process without completing the flow.
func (f *LoginFlow) Close() error {
	f.stopOnce.Do(f.stop)
	if err := f.Wait(); !isExpectedStopExit(err) {
		return err
	}
	return nil
}

func (f *LoginFlow) stop() {
	if f.cmd.Process == nil {
		return
	}
	_ = f.cmd.Process.Signal(syscall.SIGTERM)
	go func() {
		select {
		case <-f.done:
		case <-time.After(5 * time.Second):
			if f.cmd.Process != nil {
				_ = f.cmd.Process.Kill()
			}
		}
	}()
}

var authURLRe = regexp.MustCompile(`https://\S+`)

// LoginURL starts "fx login" with the browser launcher disabled and returns
// the printed authorization URL along with the still running process.
func (c *Client) LoginURL(ctx context.Context) (*LoginFlow, error) {
	cmd, err := c.interactiveCommand(ctx, "login")
	if err != nil {
		return nil, err
	}
	stdout, pipeErr := cmd.StdoutPipe()
	if pipeErr != nil {
		return nil, transportError("open fx login stdout", pipeErr)
	}
	stderr, pipeErr := cmd.StderrPipe()
	if pipeErr != nil {
		return nil, transportError("open fx login stderr", pipeErr)
	}
	if startErr := cmd.Start(); startErr != nil {
		return nil, transportError("start fx login", startErr)
	}
	flow := &LoginFlow{cmd: cmd, waitDone: make(chan error, 1), done: make(chan struct{})}
	go func() { flow.waitDone <- cmd.Wait() }()
	return awaitLoginURL(ctx, flow, stdout, stderr)
}

func awaitLoginURL(ctx context.Context, flow *LoginFlow, streams ...io.Reader) (*LoginFlow, error) {
	found := make(chan string, len(streams))
	var wg sync.WaitGroup
	for _, stream := range streams {
		wg.Add(1)
		go func(r io.Reader) {
			defer wg.Done()
			scanURL(r, found)
		}(stream)
	}
	go func() {
		wg.Wait()
		close(found)
	}()
	select {
	case url, ok := <-found:
		if !ok || url == "" {
			_ = flow.Close()
			return nil, transportError("fx login did not print an authorization URL", nil)
		}
		flow.URL = url
		return flow, nil
	case <-ctx.Done():
		_ = flow.Close()
		return nil, &Error{Kind: KindInterrupted, Message: "fx login canceled", Original: ctx.Err()}
	}
}

func scanURL(r io.Reader, found chan<- string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if match := authURLRe.FindString(scanner.Text()); match != "" {
			select {
			case found <- match:
			default:
			}
			return
		}
	}
}
