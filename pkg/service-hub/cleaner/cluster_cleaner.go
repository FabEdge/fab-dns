package cleaner

import (
	"context"
	"github.com/go-logr/logr"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/fabedge/fab-dns/pkg/service-hub/types"
)

type Config struct {
	Manager             manager.Manager
	Store               *types.ClusterStore
	Interval            time.Duration
	RequestTimeout      time.Duration
	RevokeGlobalService types.RevokeGlobalServiceFunc
}

// clusterCleaner will run periodically and clean endpoints of global services of expired cluster
type clusterCleaner struct {
	Config
	log    logr.Logger
	client client.Client
}

func AddToManager(cfg Config) error {
	return cfg.Manager.Add(&clusterCleaner{
		Config: cfg,
		client: cfg.Manager.GetClient(),
		log:    cfg.Manager.GetLogger().WithName("clusterCleaner"),
	})
}

func (cleaner *clusterCleaner) Start(ctx context.Context) error {
	tick := time.NewTicker(cleaner.Interval)
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
	for _, cluster := range cleaner.Store.GetExpiredClusters() {
	inner:
		for _, key := range cluster.GetAllServiceKeys() {
			if !cluster.IsExpired() {
				break inner
			}

			cleaner.revokeGlobalService(cluster.Name(), key)
		}
	}
}

func (cleaner *clusterCleaner) revokeGlobalService(clusterName string, key client.ObjectKey) {
	ctx, cancel := context.WithTimeout(context.Background(), cleaner.RequestTimeout)
	defer cancel()

	err := cleaner.RevokeGlobalService(ctx, clusterName, key.Namespace, key.Name)
	if err != nil {
		cleaner.log.Error(err, "failed to revoke global service", "cluster", clusterName, "key", key)
	}
}
