package exporter

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlpkg "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apis "github.com/FabEdge/fab-dns/pkg/apis/v1alpha1"
	"github.com/FabEdge/fab-dns/pkg/service-hub/types"
)

const (
	nameExporter       = "serviceExporter"
	nameDiffChecker    = "diffChecker"
	labelGlobalService = "fabedge.io/global-service"
)

type ExportGlobalServiceFunc func(service apis.GlobalService) error
type RevokeGlobalServiceFunc func(clusterName, namespace, serviceName string) error

type Config struct {
	ClusterName string
	Zone        string
	Region      string

	Manager             manager.Manager
	ExportGlobalService ExportGlobalServiceFunc
	RevokeGlobalService RevokeGlobalServiceFunc
}

var _ reconcile.Reconciler = &serviceExporter{}

type serviceExporter struct {
	Config
	client client.Client
	log    logr.Logger

	serviceKeySet types.ObjectKeySet
}

func AddToManager(cfg Config) error {
	if err := addExporterToManager(cfg.Manager, newServiceExporter(cfg)); err != nil {
		return err
	}

	return addDiffCheckerToManager(cfg.Manager, newDiffChecker(cfg))
}

func newServiceExporter(cfg Config) *serviceExporter {
	return &serviceExporter{
		Config:        cfg,
		log:           cfg.Manager.GetLogger().WithName(nameExporter),
		client:        cfg.Manager.GetClient(),
		serviceKeySet: types.NewObjectKeySet(),
	}
}

func addExporterToManager(mgr manager.Manager, reconciler reconcile.Reconciler) error {
	return ctrlpkg.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Owns(&discoveryv1.EndpointSlice{}).
		Named(nameExporter).
		Complete(reconciler)
}

func (exporter serviceExporter) Reconcile(ctx context.Context, req reconcile.Request) (result reconcile.Result, err error) {
	log := exporter.log.WithValues("request", req)

	var svc corev1.Service
	if err = exporter.client.Get(ctx, req.NamespacedName, &svc); err != nil {
		if errors.IsNotFound(err) {
			log.V(5).Info("service is not found")
			err = exporter.revokeGlobalService(req.NamespacedName)
			return
		}

		log.Error(err, "failed to get service")
		return
	}

	if exporter.shouldSkipService(svc) {
		log.V(5).Info("this service is not a global-service")
		err = exporter.revokeGlobalService(req.NamespacedName)
		return
	}

	var ports []apis.ServicePort
	for _, port := range svc.Spec.Ports {
		ports = append(ports, apis.ServicePort{
			Name:        port.Name,
			Port:        port.Port,
			Protocol:    port.Protocol,
			AppProtocol: port.AppProtocol,
		})
	}

	var (
		endpoints   []apis.Endpoint
		serviceType apis.ServiceType
	)

	if svc.Spec.ClusterIP == corev1.ClusterIPNone {
		serviceType = apis.Headless
		endpoints, err = exporter.getEndpointsOfService(ctx, svc)
		if err != nil {
			exporter.log.Error(err, "failed to get endpointslices of service")
			return
		}
	} else {
		serviceType = apis.ClusterIP
		endpoints = append(endpoints, apis.Endpoint{
			Cluster:   exporter.ClusterName,
			Zone:      exporter.Zone,
			Region:    exporter.Region,
			Addresses: []string{svc.Spec.ClusterIP},
		})
	}

	globalService := apis.GlobalService{
		ObjectMeta: metav1.ObjectMeta{
			Name:        svc.Name,
			Namespace:   svc.Namespace,
			ClusterName: exporter.ClusterName,
		},
		Spec: apis.GlobalServiceSpec{
			Type:      serviceType,
			Ports:     ports,
			Endpoints: endpoints,
		},
	}

	log.V(5).Info("global service is exported", "globalService", globalService)
	err = exporter.ExportGlobalService(globalService)
	if err != nil {
		log.Error(err, "failed to export service")
		return
	}

	exporter.serviceKeySet.Add(req.NamespacedName)
	return result, nil
}

func (exporter *serviceExporter) getEndpointsOfService(ctx context.Context, svc corev1.Service) ([]apis.Endpoint, error) {
	var endpointSliceList discoveryv1.EndpointSliceList
	err := exporter.client.List(ctx, &endpointSliceList,
		client.InNamespace(svc.Namespace),
		client.MatchingLabels{
			"kubernetes.io/service-name": svc.Name,
		},
	)
	if err != nil {
		return nil, err
	}

	var endpoints []apis.Endpoint
	for _, es := range endpointSliceList.Items {
		for _, ep := range es.Endpoints {
			endpoints = append(endpoints, apis.Endpoint{
				Cluster:   exporter.ClusterName,
				Zone:      exporter.Zone,
				Region:    exporter.Region,
				Addresses: ep.Addresses,
				Hostname:  ep.Hostname,
				TargetRef: ep.TargetRef,
			})
		}
	}

	return endpoints, nil
}

func (exporter serviceExporter) shouldSkipService(svc corev1.Service) bool {
	return !isGlobalService(svc.Labels) || svc.Spec.Type != corev1.ServiceTypeClusterIP
}

func (exporter serviceExporter) revokeGlobalService(serviceKey client.ObjectKey) error {
	log := exporter.log.WithValues("serviceKey", serviceKey)

	if !exporter.serviceKeySet.Has(serviceKey) {
		log.V(5).Info("this service is not exported before, skip revoking")
		return nil
	}

	log.V(5).Info("revoke global service and associated endpoints")
	if err := exporter.RevokeGlobalService(exporter.ClusterName, serviceKey.Namespace, serviceKey.Name); err != nil {
		log.Error(err, "failed to revoke global service")
		return err
	}

	exporter.serviceKeySet.Delete(serviceKey)

	return nil
}

func isGlobalService(labels map[string]string) bool {
	return labels != nil && labels[labelGlobalService] == "true"
}
