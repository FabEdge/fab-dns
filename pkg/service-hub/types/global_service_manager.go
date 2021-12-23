package types

import (
	"context"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/FabEdge/fab-dns/pkg/apis/v1alpha1"
)

type GlobalServiceManager interface {
	// CreateOrMergeGlobalService will create global service if not exists
	// otherwise merge endpoints and ports of service passed in
	CreateOrMergeGlobalService(service apis.GlobalService) error

	// RecallGlobalService will remove endpoints of cluster from global service
	// specified by namespace/name, if no endpoints left, the global service will
	// be also deleted
	RecallGlobalService(clusterName, namespace, serviceName string) error
}

var _ GlobalServiceManager = &globalServiceManager{}

type globalServiceManager struct {
	client client.Client
	lock   sync.RWMutex
}

func NewGlobalServiceManager(cli client.Client) GlobalServiceManager {
	return &globalServiceManager{
		client: cli,
	}
}

func (manager *globalServiceManager) CreateOrMergeGlobalService(externalService apis.GlobalService) error {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var (
		localService apis.GlobalService
		key          = client.ObjectKey{Name: externalService.Name, Namespace: externalService.Namespace}
	)

	err := manager.client.Get(ctx, key, &localService)
	if errors.IsNotFound(err) {
		localService = apis.GlobalService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      externalService.Name,
				Namespace: externalService.Namespace,
			},
			Spec: apis.GlobalServiceSpec{
				Type:      externalService.Spec.Type,
				Ports:     externalService.Spec.Ports,
				Endpoints: externalService.Spec.Endpoints,
			},
		}
		return manager.client.Create(ctx, &localService)
	} else if err == nil {
		// remove old endpoints from this cluster
		allEndpoints := removeEndpoints(localService.Spec.Endpoints, externalService.ClusterName)
		allEndpoints = append(allEndpoints, externalService.Spec.Endpoints...)

		// todo: handle cases when ports or type are different
		localService.Spec = apis.GlobalServiceSpec{
			Type:      externalService.Spec.Type,
			Ports:     externalService.Spec.Ports,
			Endpoints: allEndpoints,
		}
		return manager.client.Update(ctx, &localService)
	}

	return err
}

func (manager *globalServiceManager) RecallGlobalService(clusterName, namespace, serviceName string) error {
	var (
		svc apis.GlobalService
		key = client.ObjectKey{Name: serviceName, Namespace: namespace}
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := manager.client.Get(ctx, key, &svc)
	if errors.IsNotFound(err) {
		return nil
	}

	svc.Spec.Endpoints = removeEndpoints(svc.Spec.Endpoints, clusterName)
	if len(svc.Spec.Endpoints) == 0 {
		err = manager.client.Delete(ctx, &svc)
	} else {
		err = manager.client.Update(ctx, &svc)
	}

	return err
}

func removeEndpoints(endpoints []apis.Endpoint, cluster string) []apis.Endpoint {
	for i := 0; i < len(endpoints); {
		if endpoints[i].Cluster == cluster {
			endpoints = append(endpoints[:i], endpoints[i+1:]...)
			continue
		}
		i++
	}

	return endpoints
}
