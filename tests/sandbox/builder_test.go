package sandbox_test

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/arashrasoulzadeh/appgent/internal/sandbox"
	"github.com/google/uuid"
)

func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available, skipping builder integration test")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon not reachable, skipping builder integration test")
	}
}

func TestBuilderImplementsBuildRunner(t *testing.T) {
	var _ sandbox.BuildRunner = (*sandbox.Builder)(nil)
}

// TestBuild_Integration exercises a real minimal Next.js static export
// build end to end — this is what turns generated .tsx source into the
// index.html that preview/deploy actually serve. Needs Docker and network
// access to the npm registry, so it's skipped rather than failed when
// either isn't available (e.g. a local dev machine without Docker
// running, or a sandboxed CI runner with no Docker socket).
func TestBuild_Integration(t *testing.T) {
	requireDocker(t)

	b := sandbox.NewBuilder()
	b.Timeout = 5 * time.Minute

	files := map[string]string{
		"package.json": `{
			"name": "test-app",
			"version": "1.0.0",
			"private": true,
			"scripts": {"build": "next build"},
			"dependencies": {"next": "14.2.5", "react": "18.3.1", "react-dom": "18.3.1"},
			"devDependencies": {"@types/node": "20.14.9", "@types/react": "18.3.3", "typescript": "5.5.3"}
		}`,
		"next.config.js": `module.exports = { output: 'export' }`,
		"tsconfig.json": `{
			"compilerOptions": {"jsx": "preserve", "esModuleInterop": true, "moduleResolution": "bundler", "module": "esnext", "target": "es2017", "strict": true, "skipLibCheck": true},
			"include": ["src"]
		}`,
		"src/app/layout.tsx": `export default function RootLayout({ children }: { children: React.ReactNode }) {
			return <html><body>{children}</body></html>
		}`,
		"src/app/page.tsx": `export default function Home() {
			return <main>Hello, test</main>
		}`,
	}

	out, err := b.Build(context.Background(), uuid.New(), files)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := out["index.html"]; !ok {
		t.Errorf("built output missing index.html; got keys: %v", keys(out))
	}
}

func keys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
