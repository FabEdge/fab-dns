package exporter

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrlpkg "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apis "github.com/FabEdge/fab-dns/pkg/apis/v1alpha1"
)

var _ reconcile.Reconciler = &diffChecker{}

// diffChecker will watch on global services and
// check if any service should be recalled
type diffChecker struct {
	Config
	client client.Client
	log    logr.Logger
}

func newDiffChecker(cfg Config) *diffChecker {
	return &diffChecker{
		Config: cfg,
		log:    cfg.Manager.GetLogger().WithName(nameDiffChecker),
		client: cfg.Manager.GetClient(),
	}
}

func addDiffCheckerToManager(mgr manager.Manager, reconciler reconcile.Reconciler) error {
	return ctrlpkg.NewControllerManagedBy(mgr).
		For(&apis.GlobalService{}).
		Named(nameDiffChecker).
		Complete(reconciler)
}

func (dc diffChecker) Reconcile(ctx context.Context, req reconcile.Request) (result reconcile.Result, err error) {
	log := dc.log.WithValues("request", req)
	var globalService apis.GlobalService
	if err = dc.client.Get(ctx, req.NamespacedName, &globalService); err != nil {
		if errors.IsNotFound(err) {
			log.V(5).Info("global service is not found, skip it")
			return result, nil
		}

		log.Error(err, "failed to get global service")
		return result, err
	}

	var service corev1.Service
	if err = dc.client.Get(ctx, req.NamespacedName, &service); err != nil {
		if errors.IsNotFound(err) {
			return result, dc.recallGlobalServiceIfNecessary(globalService)
		}

		log.Error(err, "failed to get service")
		return result, err
	}

	if !isGlobalService(service.Labels) {
		log.V(5).Info("service is not exported, try to recall expired endpoints", "service", service)
		return result, dc.recallGlobalServiceIfNecessary(globalService)
	}

	return result, nil
}

func (dc diffChecker) recallGlobalServiceIfNecessary(globalService apis.GlobalService) error {
	for _, endpoint := range globalService.Spec.Endpoints {
		if endpoint.Cluster == dc.ClusterName {
			dc.log.V(5).Info("this service has some expired endpoints, recall them", "globalService", globalService)
			err := dc.RecallGlobalService(dc.ClusterName, globalService.Namespace, globalService.Name)
			if err != nil {
				dc.log.Error(err, "failed to recall expired service", "globalService", globalService)
			}
		}
	}

	return nil
}
