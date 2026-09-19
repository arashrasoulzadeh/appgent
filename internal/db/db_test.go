package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPool_InvalidDSN(t *testing.T) {
	ctx := context.Background()
	_, err := NewPool(ctx, "invalid-dsn")
	assert.Error(t, err)
}

func TestMustNewPool_PanicsOnError(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			assert.Contains(t, r.(error).Error(), "parse config")
		}
	}()
	MustNewPool(context.Background(), "invalid-dsn")
}

// Integration tests require a real database
func TestNewPool_Integration(t *testing.T) {
	dsn := "postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable"
	ctx := context.Background()

	pool, err := NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	assert.NotNil(t, pool)
	assert.NoError(t, pool.Ping(ctx))

	// Test pool stats
	stats := pool.Stat()
	assert.GreaterOrEqual(t, stats.TotalConns(), int32(0))
}

func TestNewPool_WithConfig(t *testing.T) {
	dsn := "postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable"
	ctx := context.Background()

	pool, err := NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}
	defer pool.Close()

	config := pool.Config()
	assert.NotNil(t, config)
	assert.Equal(t, int32(25), config.MaxConns)
	assert.Equal(t, int32(5), config.MinConns)
}