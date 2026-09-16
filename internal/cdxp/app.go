// Package cdxp implements the cdxp launcher: a named-profile front end for the
// Codex CLI.
//
// A profile is a JSON file holding connection parameters (model / base_url /
// wire_api / API key …) plus display metadata (description / tags / notes /
// homepage). cdxp turns it into `codex -c …` arguments and starts Codex.
package cdxp

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Version is the released cdxp version.
const Version = "1.0.0"

// App carries everything the CLI touches outside its own memory: the standard
// streams, the environment, and the two effectful operations that tests want to
// replace (handing the process over to codex, and the connectivity probe).
type App struct {
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader

	Getenv func(string) string
	Home   string

	pal palette

	Exec     func(argv0 string, argv, env []string) error
	LookPath func(string) (string, error)
	HTTP     *http.Client
}

// New returns an App wired to the real process environment.
func New() *App {
	return &App{
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Stdin:    os.Stdin,
		Getenv:   os.Getenv,
		Home:     userHome(),
		pal:      newPalette(ColorEnabled(os.Stdout)),
		Exec:     execProcess,
		LookPath: exec.LookPath,
		HTTP:     &http.Client{Timeout: 20 * time.Second},
	}
}

func userHome() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	return os.Getenv("HOME")
}

// Run executes the CLI and returns the process exit status.
func (a *App) Run(args []string) int {
	err := a.dispatch(args)
	if err == nil {
		return 0
	}
	switch e := err.(type) {
	case exitError:
		return e.code
	case cliError:
		a.eprintf("cdxp: %s\n", e.msg)
	default:
		a.eprintf("cdxp: %v\n", err)
	}
	return 1
}

func (a *App) dispatch(args []string) error {
	if len(args) == 0 {
		return a.listProfiles(nil)
	}
	switch args[0] {
	case "help", "-h", "--help":
		a.printUsage()
		return nil
	case "version", "-V", "--version":
		a.printf("cdxp %s\n", Version)
		return nil
	case "ls", "list":
		return a.listProfiles(args[1:])
	case "show", "info", "describe":
		return a.showProfile(args[1:])
	case "path", "file":
		return a.cmdPath(args[1:])
	case "edit":
		return a.cmdEdit(args[1:])
	case "new", "init", "create":
		return a.cmdNew(args[1:])
	case "rm", "remove":
		return a.cmdRm(args[1:])
	case "check", "ping", "validate":
		return a.cmdCheck(args[1:])
	case "doctor":
		return a.cmdDoctor()
	case "env":
		return a.cmdEnv(args[1:])
	case "schema", "template":
		a.printSchema()
		return nil
	default:
		return a.launch(args)
	}
}

// cliError is a fatal error that Run reports as "cdxp: <message>".
type cliError struct{ msg string }

func (e cliError) Error() string { return e.msg }

// exitError stops the CLI with a status code and no extra output.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

func dief(format string, args ...any) error { return cliError{fmt.Sprintf(format, args...)} }

func (a *App) printf(format string, args ...any)  { fmt.Fprintf(a.Stdout, format, args...) }
func (a *App) eprintf(format string, args ...any) { fmt.Fprintf(a.Stderr, format, args...) }

// warn reports a non-fatal problem the way the shell version's warn() did.
func (a *App) warn(format string, args ...any) {
	a.eprintf("cdxp: "+format+"\n", args...)
}

// ColorEnabled mirrors the shell guard: colors only when stdout is a terminal,
// NO_COLOR is unset, and TERM is neither empty nor "dumb".
func ColorEnabled(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if t := os.Getenv("TERM"); t == "" || t == "dumb" {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

// palette holds the ANSI escapes, or empty strings when color is off.
type palette struct {
	Rst, Bld, Dim, Cyn, Grn, Yel, Red, Mag string
}

func newPalette(on bool) palette {
	if !on {
		return palette{}
	}
	return palette{
		Rst: "\033[0m", Bld: "\033[1m", Dim: "\033[2m",
		Cyn: "\033[36m", Grn: "\033[32m", Yel: "\033[33m", Red: "\033[31m", Mag: "\033[35m",
	}
}

// padRight pads s to width counting bytes, the way bash's printf %-*s does.
// The shell version laid these columns out with byte widths, so a CJK label
// pads to fewer visual columns than an ASCII one.
func padRight(s string, width int) string {
	if n := width - len(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// maskKey hides most of an API key: short keys collapse to ****, longer ones
// keep the first six and last four characters.
func maskKey(k string) string {
	r := []rune(k)
	if len(r) <= 12 {
		return "****"
	}
	return string(r[:6]) + "…" + string(r[len(r)-4:])
}

// shellQuote renders s the way bash's printf %q does: words that need no
// escaping pass through, control characters force the $'…' form, and every
// other shell metacharacter gets a backslash. Copying the dry-run line back
// into a shell therefore reproduces the exact same argv.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		if isControl(r) {
			return ansiQuote(s)
		}
	}
	var b strings.Builder
	for _, r := range s {
		if !shellSafe(r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// shellSafe reports whether %q leaves r alone: alphanumerics, non-ASCII bytes,
// and the punctuation bash considers readable.
func shellSafe(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r >= 0x80:
		return true
	}
	return strings.ContainsRune("_-+.,/:@%=~#", r)
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }

// ansiQuote produces the $'…' form %q uses for control characters.
func ansiQuote(s string) string {
	var b strings.Builder
	b.WriteString("$'")
	for _, r := range s {
		switch r {
		case '\a':
			b.WriteString(`\a`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\v':
			b.WriteString(`\v`)
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		default:
			if isControl(r) {
				fmt.Fprintf(&b, `\%03o`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteString("'")
	return b.String()
}

// mergeEnv applies KEY=value pairs on top of base, replacing whole entries so a
// variable never appears twice.
func mergeEnv(base, pairs []string) []string {
	if len(pairs) == 0 {
		return base
	}
	drop := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		if k, _, ok := strings.Cut(p, "="); ok {
			drop[k] = true
		}
	}
	out := make([]string, 0, len(base)+len(pairs))
	for _, kv := range base {
		if k, _, ok := strings.Cut(kv, "="); ok && drop[k] {
			continue
		}
		out = append(out, kv)
	}
	return append(out, pairs...)
}

// firstNonEmpty returns the first non-empty string, standing in for jq's `//`.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
