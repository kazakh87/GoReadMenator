package gitx

import (
	"os/exec"
)

// runGit führt git in dir aus, für Testfixtures.
func runGit(dir string, args ...string) error {
	full := append([]string{"-C", dir,
		"-c", "user.name=test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false"}, args...)
	return exec.Command("git", full...).Run()
}
