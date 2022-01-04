package cleaner

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/FabEdge/fab-dns/pkg/service-hub/types"
)

// clusterCleaner will run periodically and clean endpoints of global services of expired cluster
type clusterCleaner struct {
	client              client.Client
	log                 logr.Logger
	interval            time.Duration
	store               *types.ClusterStore
	revokeGlobalService types.RevokeGlobalServiceFunc
}

func AddToManager(mgr manager.Manager, store *types.ClusterStore, interval time.Duration, revokeGlobalService types.RevokeGlobalServiceFunc) error {
	return mgr.Add(&clusterCleaner{
		client:              mgr.GetClient(),
		store:               store,
		interval:            interval,
		revokeGlobalService: revokeGlobalService,
		log:                 mgr.GetLogger().WithName("clusterCleaner"),
	})
}

func (cleaner *clusterCleaner) Start(ctx context.Context) error {
	tick := time.NewTicker(cleaner.interval)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			cleaner.cleanExpiredClusterEndpoints()
		case <-ctx.Done():
			return nil
		}
	}
}

func (cleaner *clusterCleaner) cleanExpiredClusterEndpoints() {
	for _, cluster := range cleaner.store.GetExpiredClusters() {
	inner:
		for _, key := range cluster.GetAllServiceKeys() {
			if !cluster.IsExpired() {
				break inner
			}

			err := cleaner.revokeGlobalService(cluster.Name(), key.Namespace, key.Name)
			if err != nil {
				cleaner.log.Error(err, "failed to revoke global service", "cluster", cluster.Name(), "key", key)
			}
		}
	}
}
