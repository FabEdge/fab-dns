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

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
)

var _ reconcile.Reconciler = &lostServiceRevoker{}

// lostServiceRevoker will watch on global services and
// check if any service should be revoked, lostServiceRevoker
// mainly revoke service that are deleted or lose global-service label during a reboot
type lostServiceRevoker struct {
	Config
	client client.Client
	log    logr.Logger
}

func newLostServiceRevoker(cfg Config) *lostServiceRevoker {
	return &lostServiceRevoker{
		Config: cfg,
		log:    cfg.Manager.GetLogger().WithName(nameLostServiceRevoker),
		client: cfg.Manager.GetClient(),
	}
}

func addDiffCheckerToManager(mgr manager.Manager, reconciler reconcile.Reconciler) error {
	return ctrlpkg.NewControllerManagedBy(mgr).
		For(&apis.GlobalService{}).
		Named(nameLostServiceRevoker).
		Complete(reconciler)
}

func (revoker lostServiceRevoker) Reconcile(ctx context.Context, req reconcile.Request) (result reconcile.Result, err error) {
	log := revoker.log.WithValues("request", req)
	var globalService apis.GlobalService
	if err = revoker.client.Get(ctx, req.NamespacedName, &globalService); err != nil {
		if errors.IsNotFound(err) {
			log.V(5).Info("global service is not found, skip it")
			return result, nil
		}

		log.Error(err, "failed to get global service")
		return result, err
	}

	var service corev1.Service
	if err = revoker.client.Get(ctx, req.NamespacedName, &service); err != nil {
		if errors.IsNotFound(err) {
			return result, revoker.revokeGlobalServiceIfNecessary(ctx, globalService)
		}

		log.Error(err, "failed to get service")
		return result, err
	}

	if !isGlobalService(service.Labels) {
		log.V(5).Info("service is not exported, try to revoke expired endpoints", "service", service)
		return result, revoker.revokeGlobalServiceIfNecessary(ctx, globalService)
	}

	return result, nil
}

func (revoker lostServiceRevoker) revokeGlobalServiceIfNecessary(ctx context.Context, globalService apis.GlobalService) error {
	for _, endpoint := range globalService.Spec.Endpoints {
		if endpoint.Cluster == revoker.ClusterName {
			revoker.log.V(5).Info("this service has some expired endpoints, revoke them", "globalService", globalService)
			err := revoker.RevokeGlobalService(ctx, revoker.ClusterName, globalService.Namespace, globalService.Name)
			if err != nil {
				revoker.log.Error(err, "failed to revoke expired service", "globalService", globalService)
			}
		}
	}

	return nil
}
