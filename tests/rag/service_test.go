package rag_test

import (
	"testing"

	"github.com/arashrasoulzadeh/appgent/internal/rag"
)

// TestIsComponentFile is a regression test for a bug where the heuristic
// checked whether the first character of the *entire path* was uppercase
// (path[0]) instead of the base filename's first character. Real generated
// code always lives under nested directories like "src/components/Card.tsx",
// whose path[0] is 's' — so the old check silently matched nothing and
// ExtractPatternsFromRun never learned from generated components.
func TestIsComponentFile(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"src/components/Card.tsx", true},
		{"src/app/Header.tsx", true},
		{"Card.tsx", true},
		{"src/components/card.tsx", false},   // lowercase filename
		{"src/app/page.tsx", false},          // lowercase filename (Next.js convention)
		{"src/lib/utils.ts", false},          // wrong extension
		{"src/components/Card.jsx", false},   // wrong extension
		{"", false},
		{"src/components/", false},
	}
	for _, tc := range cases {
		if got := rag.IsComponentFile(tc.path); got != tc.want {
			t.Errorf("IsComponentFile(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
