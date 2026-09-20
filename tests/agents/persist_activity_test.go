package agents_test

import (
	"testing"

	"github.com/arashrasoulzadeh/appgent/internal/agents"
	"github.com/stretchr/testify/assert"
)

func TestAppStatusFor(t *testing.T) {
	tests := []struct {
		runStatus string
		want      string
	}{
		{"succeeded", "ready"},
		{"needs_review", "needs_review"},
		{"failed", "failed"},
		{"queued", "failed"}, // unexpected/unknown input falls back to failed, not silently "ready"
		{"", "failed"},
	}

	for _, tc := range tests {
		t.Run(tc.runStatus, func(t *testing.T) {
			assert.Equal(t, tc.want, agents.AppStatusFor(tc.runStatus))
		})
	}
}
