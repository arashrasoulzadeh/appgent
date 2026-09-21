package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/arashrasoulzadeh/appgent/internal/agents"
	"github.com/arashrasoulzadeh/appgent/internal/config"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func main() {
	cfg := config.Load()

	ctx := context.Background()

	// Connect to Temporal
	c, err := client.Dial(client.Options{
		HostPort:  cfg.TemporalHostPort,
		Namespace: cfg.TemporalNamespace,
	})
	if err != nil {
		log.Fatalf("Failed to connect to Temporal: %v", err)
	}
	defer c.Close()

	// Connect to database
	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Create worker
	w := worker.New(c, "appgent-generation", worker.Options{
		MaxConcurrentActivityExecutionSize: 10,
	})

	// Give the activity functions DB access for progress tracking
	// (agent_steps rows) — see internal/agents/tracking.go.
	agents.SetDBPool(pool)

	// Register workflow and activities. Activities are registered under
	// explicit names matching what GenerateAppWorkflow references by
	// string (internal/temporal/workflow.go) — the real implementations
	// live in internal/agents, not in the temporal package itself.
	persistActivities := &agents.PersistActivities{Pool: pool}

	w.RegisterWorkflow(temporal.GenerateAppWorkflow)
	w.RegisterActivityWithOptions(agents.PlanActivity, activity.RegisterOptions{Name: "PlanActivity"})
	w.RegisterActivityWithOptions(agents.DesignActivity, activity.RegisterOptions{Name: "DesignActivity"})
	w.RegisterActivityWithOptions(agents.CodeActivity, activity.RegisterOptions{Name: "CodeActivity"})
	w.RegisterActivityWithOptions(agents.QAActivity, activity.RegisterOptions{Name: "QAActivity"})
	w.RegisterActivityWithOptions(persistActivities.PersistRunResult, activity.RegisterOptions{Name: "PersistRunResultActivity"})

	// Start worker
	err = w.Start()
	if err != nil {
		log.Fatalf("Failed to start worker: %v", err)
	}

	log.Println("Temporal worker started on task queue: appgent-generation")

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down worker...")
	w.Stop()
	log.Println("Worker stopped")
}
