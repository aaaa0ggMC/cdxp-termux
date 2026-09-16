//go:build !windows

package cdxp

import "syscall"

// execProcess replaces the current process with codex, matching the shell
// version's `exec` so signals and the terminal are handed over untouched.
func execProcess(argv0 string, argv, env []string) error {
	return syscall.Exec(argv0, argv, env)
}
