package types

import (
	"context"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlpkg "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
	nsutil "github.com/fabedge/fab-dns/pkg/util/namespace"
)

type GlobalServiceManager interface {
	// CreateOrMergeGlobalService will create global service if not exists
	// otherwise merge endpoints and ports of service passed in
	CreateOrMergeGlobalService(service apis.GlobalService) error

	// RevokeGlobalService will remove endpoints of cluster from global service
	// specified by namespace/name, if no endpoints left, the global service will
	// be also deleted
	RevokeGlobalService(clusterName, namespace, serviceName string) error
}

var _ GlobalServiceManager = &globalServiceManager{}

type globalServiceManager struct {
	allowCreateNamespace bool
	client               client.Client

	// this lock is used to protect a global service
	// from being changed by requests simultaneously
	// todo: implement object lock
	lock sync.RWMutex
}

func NewGlobalServiceManager(cli client.Client, allowCreateNamespace bool) GlobalServiceManager {
	return &globalServiceManager{
		client:               cli,
		allowCreateNamespace: allowCreateNamespace,
	}
}

func (manager *globalServiceManager) CreateOrMergeGlobalService(externalService apis.GlobalService) error {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if manager.allowCreateNamespace {
		if err := nsutil.Ensure(ctx, manager.client, externalService.Namespace); err != nil {
			return err
		}
	}

	localService := &apis.GlobalService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalService.Name,
			Namespace: externalService.Namespace,
			Labels: map[string]string{
				"fabedge.io/created-by": "service-hub",
			},
		},
	}

	_, err := ctrlpkg.CreateOrUpdate(ctx, manager.client, localService, func() error {
		// remove old endpoints from this cluster
		allEndpoints := removeEndpoints(localService.Spec.Endpoints, externalService.ClusterName)
		allEndpoints = append(allEndpoints, externalService.Spec.Endpoints...)

		// todo: handle cases when ports or type are different
		localService.Spec = apis.GlobalServiceSpec{
			Type:      externalService.Spec.Type,
			Ports:     externalService.Spec.Ports,
			Endpoints: allEndpoints,
		}

		return nil
	})

	return err
}

func (manager *globalServiceManager) RevokeGlobalService(clusterName, namespace, serviceName string) error {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var (
		svc apis.GlobalService
		key = client.ObjectKey{Name: serviceName, Namespace: namespace}
	)
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
