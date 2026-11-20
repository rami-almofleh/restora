// Package reconcile runs the watch-and-retry loop that every Restora controller shares.
package reconcile

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Handler processes one object by name. A positive requeueAfter asks for another call later,
// for example to poll a long-running job. An error triggers a retry with back-off.
type Handler func(ctx context.Context, name string) (requeueAfter time.Duration, err error)

// Run watches resources of one kind in one namespace and calls handle for every change,
// until ctx ends. Objects are handled one at a time, so a handler never races with itself.
func Run(ctx context.Context, log *slog.Logger, client dynamic.Interface, gvr schema.GroupVersionResource, namespace string, resync time.Duration, handle Handler) error {
	return RunWorkers(ctx, log, client, gvr, namespace, resync, 1, handle)
}

// RunWorkers is Run with several objects handled at the same time, for handlers that wait a long time, such as a
// connection test. One object is still never handled twice at once, so a handler never races with itself.
func RunWorkers(ctx context.Context, log *slog.Logger, client dynamic.Interface, gvr schema.GroupVersionResource, namespace string, resync time.Duration, workers int, handle Handler) error {
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(client, resync, namespace, nil)
	informer := factory.ForResource(gvr).Informer()
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]())
	defer queue.ShutDown()

	enqueue := func(obj any) {
		if key, err := cache.MetaNamespaceKeyFunc(obj); err == nil {
			_, name, _ := cache.SplitMetaNamespaceKey(key)
			queue.Add(name)
		}
	}
	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    enqueue,
		UpdateFunc: func(_, obj any) { enqueue(obj) },
	}); err != nil {
		return fmt.Errorf("register event handler: %w", err)
	}

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return fmt.Errorf("wait for %s cache: %w", gvr.Resource, ctx.Err())
	}
	log.Info("controller started", "resource", gvr.Resource, "namespace", namespace)

	go func() {
		<-ctx.Done()
		queue.ShutDown()
	}()
	var wg sync.WaitGroup
	for range max(workers, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for processNext(ctx, log, queue, handle) {
			}
		}()
	}
	wg.Wait()
	return nil
}

func processNext(ctx context.Context, log *slog.Logger, queue workqueue.TypedRateLimitingInterface[string], handle Handler) bool {
	name, shutdown := queue.Get()
	if shutdown {
		return false
	}
	defer queue.Done(name)

	requeueAfter, err := handle(ctx, name)
	switch {
	case err != nil:
		log.Error("reconcile failed, will retry", "name", name, "error", err)
		queue.AddRateLimited(name)
	case requeueAfter > 0:
		queue.Forget(name)
		queue.AddAfter(name, requeueAfter)
	default:
		queue.Forget(name)
	}
	return true
}
