// Command controller runs the Restora operator: it carries out the custom resources the API creates.
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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"restora.local/restora/internal/discovery"
	"restora.local/restora/internal/location"
	"restora.local/restora/internal/offsite"
	"restora.local/restora/internal/policy"
	"restora.local/restora/internal/reconcile"
	"restora.local/restora/internal/restore"
	"restora.local/restora/internal/server"
	"restora.local/restora/internal/snapshot"
	"restora.local/restora/internal/version"
)

// copyWorkers is how many snapshots are handled at the same time. A copy to a storage location can take hours,
// and the other backups should not wait for it.
const copyWorkers = 4

// locationTestWorkers is how many storage locations are tested at the same time. A test of NFS can take a minute,
// and the others should not wait for it.
const locationTestWorkers = 4

func main() {
	// The data movers copy volumes to and from storage locations and run this same binary.
	if len(os.Args) == 3 && os.Args[1] == offsite.MoverCommand {
		os.Exit(offsite.RunMover(context.Background(), os.Args[2:], os.Getenv, offsite.ExecRunner{}, "/dev/termination-log"))
	}
	// The test pod of an NFS location runs this same binary to probe the mounted share.
	if len(os.Args) == 3 && os.Args[1] == location.ProbeCommand {
		os.Exit(location.RunProbe(os.Args[2], "/dev/termination-log"))
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("component", "controller")
	if err := run(log); err != nil {
		log.Error("controller stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("load cluster access (is the service account token mounted?): %w", err)
	}
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create kubernetes client: %w", err)
	}

	typed, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create kubernetes client: %w", err)
	}
	source, err := discovery.NewKubeSource(cfg)
	if err != nil {
		return err
	}

	namespace := envOr("RESTORA_NAMESPACE", "restora-system")
	snapshotStore := snapshot.NewStore(client, namespace)
	restoreStore := restore.NewStore(client, namespace)
	exporter := snapshot.NewExporter(client, envOr("RESTORA_DATA_DIR", "/data"))
	image := envOr("RESTORA_IMAGE", "restora-controller:dev")
	pullPolicy := corev1.PullPolicy(envOr("RESTORA_IMAGE_PULL_POLICY", string(corev1.PullIfNotPresent)))
	locationStore := location.NewStore(client, namespace)
	copies := &offsite.Exporter{
		Locations: locationStore,
		Keys:      location.NewSecretReader(typed, namespace),
		Passwords: &offsite.Passwords{Client: typed, Namespace: namespace},
		Mover:     &offsite.Mover{Client: typed, Image: image, PullPolicy: pullPolicy},
		Bundles:   exporter,
		Records:   offsite.NewRecordStore(client, namespace),
		Client:    typed,
		Namespace: namespace,
		Log:       log,
		Now:       time.Now,
	}
	snapshots := &snapshot.Reconciler{
		Store:    snapshotStore,
		Volumes:  snapshot.NewVolumes(client),
		Exporter: exporter,
		Export:   copies,
		Log:      log,
		Now:      time.Now,
	}
	restores := &restore.Reconciler{
		Client:    client,
		Store:     restoreStore,
		Snapshots: snapshotStore,
		Exports:   exporter,
		Records:   copies.Records,
		Offsite: &offsite.Restorer{
			Locations: locationStore,
			Keys:      copies.Keys,
			Passwords: copies.Passwords,
			Mover:     copies.Mover,
			Bundles:   exporter,
			Client:    typed,
			Namespace: namespace,
			Log:       log,
			Now:       time.Now,
		},
		Log: log,
		Now: time.Now,
	}

	// Scheduled backups follow the same rules as manual ones: not while the application is restored.
	scheduler := snapshot.NewService(snapshotStore, nil)
	restoreRequests := restore.NewService(restoreStore, snapshotStore)
	scheduler.AddGuard(restoreRequests.Guard)
	scheduler.AddDeleteGuard(restoreRequests.SnapshotGuard)
	policies := &policy.Reconciler{
		Store:     policy.NewStore(client, namespace),
		Apps:      discovery.New(source),
		Backups:   scheduler,
		Snapshots: snapshotStore,
		Deleter:   scheduler,
		Log:       log,
		Now:       time.Now,
	}

	locations := &location.Reconciler{
		Store: locationStore,
		S3:    &location.S3Prober{Keys: location.NewSecretReader(typed, namespace)},
		NFS: &location.NFSProber{
			Client:     typed,
			Namespace:  namespace,
			Image:      image,
			PullPolicy: pullPolicy,
		},
		Log: log,
		Now: time.Now,
	}

	mux := http.NewServeMux()
	server.RegisterHealth(mux)
	addr := envOr("RESTORA_ADDR", ":8081")
	srv := server.New(addr, server.Logging(log, mux))
	log.Info("starting", "addr", addr, "version", version.Version, "namespace", namespace)

	// Either part ending stops the other, so the pod restarts instead of half-working.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	const parts = 5
	errs := make(chan error, parts)
	go func() { errs <- server.Run(ctx, log, srv) }()
	go func() {
		errs <- reconcile.RunWorkers(ctx, log, client, snapshot.GVR, namespace, 30*time.Second, copyWorkers, snapshots.Reconcile)
	}()
	go func() {
		errs <- reconcile.Run(ctx, log, client, restore.GVR, namespace, 30*time.Second, restores.Reconcile)
	}()
	go func() {
		errs <- reconcile.Run(ctx, log, client, policy.GVR, namespace, 30*time.Second, policies.Reconcile)
	}()
	go func() {
		errs <- reconcile.RunWorkers(ctx, log, client, location.GVR, namespace, 30*time.Second, locationTestWorkers, locations.Reconcile)
	}()
	err = <-errs
	cancel()
	for range parts - 1 {
		<-errs
	}
	return err
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
