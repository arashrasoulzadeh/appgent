package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// BuildRunner builds a run's generated source into a static site. Extracted
// as an interface so callers (internal/agents.PublishBundleActivity) can be
// tested against a fake instead of actually shelling out to Docker. The
// returned log is the build container's combined stdout/stderr (npm
// install + npm run build output) — returned on both success and failure
// so it can be surfaced as the "publish" agent_steps row's output, not
// just used for the error message on failure.
type BuildRunner interface {
	Build(ctx context.Context, runID uuid.UUID, files map[string]string) (built map[string]string, buildLog string, err error)
}

// Builder compiles a run's generated Next.js source into a static HTML/CSS/
// JS bundle by running `npm install && npm run build` inside a short-lived
// sibling container, launched against the host's Docker daemon via the
// mounted socket (docker-outside-of-docker — the worker itself never runs a
// Docker daemon). This is real isolation from the worker process for code
// whose package.json/build scripts are LLM output, not something to trust
// blindly.
//
// Files go in and come out via `docker cp` rather than a bind mount,
// because a path inside the worker container (e.g. from os.MkdirTemp) does
// not exist on the host, and volumes given to the host daemon are resolved
// against the HOST filesystem, not the worker's — cp works over the Docker
// API directly and sidesteps that entirely.
type Builder struct {
	Image        string
	OutputDir    string // directory inside the build container holding the static export, e.g. "out" for Next.js `output: 'export'`
	Timeout      time.Duration
	MemoryLimit  string // e.g. "1g", passed to `docker create --memory`
	CPULimit     string // e.g. "1", passed to `docker create --cpus`
}

func NewBuilder() *Builder {
	return &Builder{
		Image:       "node:20-alpine",
		OutputDir:   "out",
		Timeout:     10 * time.Minute,
		MemoryLimit: "1g",
		CPULimit:    "1",
	}
}

// Build writes files into a fresh container, runs the install+build there,
// and returns the built static site's files (paths relative to OutputDir).
// Returns an error including the container's combined stdout/stderr on
// build failure, since that's the only useful diagnostic for whatever the
// LLM's generated code did wrong.
func (b *Builder) Build(ctx context.Context, runID uuid.UUID, files map[string]string) (map[string]string, string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, "", fmt.Errorf("docker CLI not available in worker image: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, b.Timeout)
	defer cancel()

	stagingDir, err := os.MkdirTemp("", "appgent-build-in-"+runID.String())
	if err != nil {
		return nil, "", fmt.Errorf("create staging dir: %w", err)
	}
	defer os.RemoveAll(stagingDir)
	if err := writeFiles(stagingDir, files); err != nil {
		return nil, "", fmt.Errorf("write staging files: %w", err)
	}

	containerName := "appgent-build-" + runID.String()
	createArgs := []string{
		"create", "--name", containerName,
		"--memory", b.MemoryLimit,
		"--cpus", b.CPULimit,
		"-w", "/app",
		b.Image,
		"sh", "-c", "npm install --no-audit --no-fund --loglevel=error && npm run build",
	}
	if out, err := exec.CommandContext(ctx, "docker", createArgs...).CombinedOutput(); err != nil {
		return nil, "", fmt.Errorf("docker create: %w: %s", err, out)
	}
	defer exec.Command("docker", "rm", "-f", containerName).Run()

	// Copy the staged source tree into the container's /app before it
	// starts. The trailing "/." copies stagingDir's CONTENTS into /app,
	// not stagingDir itself as a subdirectory of /app.
	if out, err := exec.CommandContext(ctx, "docker", "cp", stagingDir+"/.", containerName+":/app").CombinedOutput(); err != nil {
		return nil, "", fmt.Errorf("docker cp in: %w: %s", err, out)
	}

	out, err := exec.CommandContext(ctx, "docker", "start", "-a", containerName).CombinedOutput()
	buildLog := string(out)
	if err != nil {
		return nil, buildLog, fmt.Errorf("build failed: %w\n%s", err, truncate(buildLog, 4000))
	}

	outputDir, err := os.MkdirTemp("", "appgent-build-out-"+runID.String())
	if err != nil {
		return nil, buildLog, fmt.Errorf("create output dir: %w", err)
	}
	defer os.RemoveAll(outputDir)

	if cpOut, err := exec.CommandContext(ctx, "docker", "cp", containerName+":/app/"+b.OutputDir+"/.", outputDir).CombinedOutput(); err != nil {
		return nil, buildLog, fmt.Errorf("docker cp out (build produced no %s/ directory — check next.config.js has output:'export'): %w: %s", b.OutputDir, err, cpOut)
	}

	built, err := readFiles(outputDir)
	if err != nil {
		return nil, buildLog, fmt.Errorf("read built files: %w", err)
	}
	if len(built) == 0 {
		return nil, buildLog, fmt.Errorf("build produced zero files in %s/", b.OutputDir)
	}
	return built, buildLog, nil
}

func writeFiles(root string, files map[string]string) error {
	for path, content := range files {
		full := filepath.Join(root, filepath.Clean("/"+path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func readFiles(root string) (map[string]string, error) {
	files := make(map[string]string)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(content)
		return nil
	})
	return files, err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + strings.TrimSpace(fmt.Sprintf("\n... (%d more bytes)", len(s)-n))
}
