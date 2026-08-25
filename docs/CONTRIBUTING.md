# Contributing

## Gates

Everything below must pass before a change lands:

```bash
just gate  # complete isolated gate under Go 1.24 and Go 1.26
```

`just test container 1.26` runs one supported version. The container gate
checks formatting, vet, unit tests, the race detector, mock integration tests,
and example builds without exercising filesystem-mutating tests on the host.

`just test integration-real` runs the same integration flows against the real
`fx`. It bills the configured model and needs an authenticated install, so run
it deliberately, not in a loop.

## House rules

- Standard library only in non-test code. `go.uber.org/goleak` is test only.
- `context.Context` is the first parameter of anything that spawns a process or
  touches the filesystem. Check `ctx.Err()` before long work.
- Never `fmt.Errorf`. Build every error through `validationError`,
  `transportError`, or `processError` and wrap the cause in `Original` so
  `errors.Is` and `errors.As` keep working.
- Watch the typed-nil trap. A function that returns `error` must not tail-call
  something that returns `*Error`: a nil `*Error` in an `error` interface is not
  nil. Convert explicitly:

  ```go
  func (o *AskOptions) Validate() error {
      if err := o.validate(); err != nil {
          return err
      }
      return nil
  }
  ```

- Files stay under 500 lines and functions under 50.
- No comments inside function bodies. Doc comments on exported identifiers only,
  one or two lines.
- No em dashes anywhere in code, docs, or commit messages.

## Testing

Tests are table driven and list the error cases before the happy path.

Unit tests never touch the real `fx`. They point `Client.BinPath` at the mock
binary built from `test/mockfx`, which:

- records argv and the `FX_*` environment into `$FX_MOCK_RECORD`;
- selects a fixture with `$FX_MOCK_SCENARIO` and exits with that scenario's code;
- replays a scripted ACP conversation from `test/testdata/acp/<scenario>.jsonl`.

Useful mock knobs: `FX_MOCK_TESTDATA`, `FX_MOCK_EXIT_CODE`, `FX_MOCK_STDERR`,
`FX_MOCK_SLEEP_MS` (for cancellation tests), `FX_MOCK_ACP_DELAY_MS` (to slow a
scripted turn so a cancel can land mid stream), `FX_MOCK_PERM_REPLY` (records
what the SDK answered to an agent request), `FX_MOCK_READ_STDIN`, and
`FX_MOCK_LOGIN_TAIL_BYTES` (proves login streams remain drained).

Every test that starts an ACP session ends with `defer goleak.VerifyNone(t)`.
The read pump, the stderr drain, the wait goroutine, and any handler goroutine
must all be gone after `Close`.

## Fixtures

`test/testdata` holds sanitized contract fixtures for fx v0.0.6. Update them
only from a verified release binary, release source, or upstream contract test,
and record the source in the fixture README. Never commit user paths, account
identifiers, credentials, or raw session metadata.

The ACP scripts use one synthetic session id throughout a conversation.
`test/testdata/acp/README.md` records their provenance and composition.

## Adding an fx command wrapper

1. Confirm the real JSON shape by running the command with `--json` and saving
   the capture into `test/testdata`.
2. Add the typed struct next to its peers in `admin.go` or `sessions.go`.
3. Route it through `runJSON`, which already maps the
   `{"kind","error","code"}` envelope onto `*Error`.
4. Add the scenario to `test/mockfx/main.go` and a table entry to the tests.
