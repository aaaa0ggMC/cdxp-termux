//go:build windows

package cdxp

import (
	"errors"
	"os"
	"os/exec"
)

// execProcess runs codex as a child and forwards its exit status, since
// Windows has no way to replace a running process image.
func execProcess(argv0 string, argv, env []string) error {
	cmd := exec.Command(argv0, argv[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			os.Exit(ee.ExitCode())
		}
		return err
	}
	os.Exit(0)
	return nil
}
