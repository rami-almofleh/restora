// Command api serves the Restora REST API and the web interface.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"restora.local/restora/internal/activity"
	"restora.local/restora/internal/discovery"
	"restora.local/restora/internal/info"
	"restora.local/restora/internal/location"
	"restora.local/restora/internal/offsite"
	"restora.local/restora/internal/policy"
	"restora.local/restora/internal/restore"
	"restora.local/restora/internal/server"
	"restora.local/restora/internal/snapshot"
	"restora.local/restora/internal/version"
	"restora.local/restora/ui"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("component", "api")
	if err := run(log); err != nil {
		log.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	uiHandler, err := ui.Handler()
	if err != nil {
		return err
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("load cluster access (is the service account token mounted?): %w", err)
	}
	source, err := discovery.NewKubeSource(cfg)
	if err != nil {
		return err
	}

	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create kubernetes client: %w", err)
	}
	typed, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create kubernetes client: %w", err)
	}
	namespace := envOr("RESTORA_NAMESPACE", "restora-system")
	apps := discovery.New(source)
	snapshotStore := snapshot.NewStore(client, namespace)
	snapshots := snapshot.NewService(snapshotStore, apps)
	restoreStore := restore.NewStore(client, namespace)
	restores := restore.NewService(restoreStore, snapshotStore).WithExports(offsite.NewRecordStore(client, namespace))
	snapshots.AddGuard(restores.Guard)
	snapshots.AddDeleteGuard(restores.SnapshotGuard)

	mux := http.NewServeMux()
	server.RegisterHealth(mux)
	discovery.Register(mux, apps, log)
	snapshot.Register(mux, snapshots, log)
	restore.Register(mux, restores, log)
	locationStore := location.NewStore(client, namespace)
	locations := location.NewService(locationStore, location.NewSecretWriter(typed, namespace))
	records := offsite.NewRecordService(offsite.NewRecordStore(client, namespace))
	locations.CountExports(records)
	policies := policy.NewService(policy.NewStore(client, namespace), apps, snapshotStore, locationStore)
	policies.EnableRunNow(snapshots, log)
	locations.AddDeleteGuard(policies.LocationGuard)
	location.Register(mux, locations, log)
	offsite.RegisterRecords(mux, records, log)
	policy.Register(mux, policies, log)
	activity.Register(mux, activity.NewService(snapshotStore, restoreStore), log)
	info.Register(mux, info.New(version.Version, time.Now()))
	mux.HandleFunc("/api/", server.NotFound)
	mux.Handle("/", uiHandler)

	addr := envOr("RESTORA_ADDR", ":8080")
	srv := server.New(addr, server.Logging(log, server.SecureHeaders(mux)))
	log.Info("starting", "addr", addr, "version", version.Version)
	return server.Run(ctx, log, srv)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
