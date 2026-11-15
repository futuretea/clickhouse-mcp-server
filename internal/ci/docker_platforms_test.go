package ci

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerPlatformStepEmitsBothArchitectures(t *testing.T) {
	root := repoRoot(t)
	script := dockerPlatformsRun(t, root)
	output, stderr, err := runBashStep(t, script, root, nil)
	if err != nil {
		t.Fatalf("platforms step: %v\nstderr:\n%s", err, stderr)
	}
	const want = "platforms=linux/amd64,linux/arm64\n"
	if output != want {
		t.Fatalf("platforms output = %q, want %q\nstderr:\n%s", output, want, stderr)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func dockerPlatformsRun(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".github/workflows/release-docker.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-")) != "id: platforms" {
			continue
		}
		for _, next := range lines[i+1:] {
			trimmed := strings.TrimSpace(next)
			if trimmed == "" {
				continue
			}
			const prefix = "run: "
			if !strings.HasPrefix(trimmed, prefix) {
				t.Fatalf("platforms step has no run script: %q", trimmed)
			}
			body := strings.TrimPrefix(trimmed, prefix)
			if body == "|" || body == ">" {
				return indentedBlock(lines[i+2:], leadingSpaces(next)+2)
			}
			return body
		}
	}
	t.Fatal("platforms step not found")
	return ""
}

func indentedBlock(lines []string, width int) string {
	var b strings.Builder
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			b.WriteByte('\n')
			continue
		}
		if leadingSpaces(line) < width {
			break
		}
		b.WriteString(strings.TrimPrefix(line, strings.Repeat(" ", width)))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

func runBashStep(t *testing.T, script, dir string, extraEnv []string) (string, string, error) {
	t.Helper()
	output := filepath.Join(t.TempDir(), "github-output")
	cmd := exec.Command("bash", "-e", "-c", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GITHUB_OUTPUT="+output)
	cmd.Env = append(cmd.Env, extraEnv...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	data, readErr := os.ReadFile(output)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	if err != nil {
		return string(data), stderr.String(), &stepError{err: err, stderr: stderr.String()}
	}
	return string(data), stderr.String(), nil
}

type stepError struct {
	err    error
	stderr string
}

func (e *stepError) Error() string {
	return e.err.Error() + "\n" + e.stderr
}
