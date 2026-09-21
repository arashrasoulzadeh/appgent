package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/arashrasoulzadeh/appgent/internal/agents"
	"github.com/arashrasoulzadeh/appgent/internal/config"
	"github.com/arashrasoulzadeh/appgent/internal/sandbox"
	"github.com/arashrasoulzadeh/appgent/internal/storage"
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

	storageClient, err := storage.NewClient(
		cfg.ObjectStorageEndpoint,
		cfg.ObjectStorageAccessKey,
		cfg.ObjectStorageSecretKey,
		false,
	)
	if err != nil {
		log.Fatalf("Failed to connect to object storage: %v", err)
	}
	provisioner := sandbox.NewStaticExportProvisioner(storageClient, cfg.ObjectStorageBucket)
	agents.SetProvisioner(provisioner)
	agents.SetBuilder(sandbox.NewBuilder())

	// Register workflows and activities. Activities are registered under
	// explicit names matching what the workflows reference by string
	// (internal/temporal/workflow.go) — the real implementations live in
	// internal/agents, not in the temporal package itself.
	persistActivities := &agents.PersistActivities{Pool: pool, Provisioner: provisioner}

	w.RegisterWorkflow(temporal.GenerateAppWorkflow)
	w.RegisterWorkflow(temporal.RedeployWorkflow)
	w.RegisterActivityWithOptions(agents.PlanActivity, activity.RegisterOptions{Name: "PlanActivity"})
	w.RegisterActivityWithOptions(agents.DesignActivity, activity.RegisterOptions{Name: "DesignActivity"})
	w.RegisterActivityWithOptions(agents.CodeActivity, activity.RegisterOptions{Name: "CodeActivity"})
	w.RegisterActivityWithOptions(agents.QAActivity, activity.RegisterOptions{Name: "QAActivity"})
	w.RegisterActivityWithOptions(persistActivities.PersistRunResult, activity.RegisterOptions{Name: "PersistRunResultActivity"})
	w.RegisterActivityWithOptions(agents.PublishBundleActivity, activity.RegisterOptions{Name: "PublishBundleActivity"})
	w.RegisterActivityWithOptions(agents.PublishSourceActivity, activity.RegisterOptions{Name: "PublishSourceActivity"})
	w.RegisterActivityWithOptions(agents.FetchRunSourceActivity, activity.RegisterOptions{Name: "FetchRunSourceActivity"})
	w.RegisterActivityWithOptions(persistActivities.UpdateRunBundlePath, activity.RegisterOptions{Name: "UpdateRunBundlePathActivity"})
	w.RegisterActivityWithOptions(persistActivities.PromoteDeployment, activity.RegisterOptions{Name: "PromoteDeploymentActivity"})
	w.RegisterActivityWithOptions(persistActivities.MarkDeploymentFailed, activity.RegisterOptions{Name: "MarkDeploymentFailedActivity"})

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
