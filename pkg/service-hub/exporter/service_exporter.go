package exporter

import (
	"context"
	"sort"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlpkg "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
	"github.com/fabedge/fab-dns/pkg/service-hub/types"
)

const (
	nameExporter           = "serviceExporter"
	nameLostServiceRevoker = "lostServiceRevoker"
	labelGlobalService     = "fabedge.io/global-service"
)

type Config struct {
	ClusterName string
	Zone        string
	Region      string

	Manager             manager.Manager
	ExportGlobalService types.ExportGlobalServiceFunc
	RevokeGlobalService types.RevokeGlobalServiceFunc
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

	return addDiffCheckerToManager(cfg.Manager, newLostServiceRevoker(cfg))
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
			err = exporter.revokeGlobalService(ctx, req.NamespacedName)
			return
		}

		log.Error(err, "failed to get service")
		return
	}

	if exporter.shouldSkipService(svc) {
		log.V(5).Info("this service is not a global-service")
		err = exporter.revokeGlobalService(ctx, req.NamespacedName)
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
		endpoints, err = GetEndpointsOfHeadlessService(exporter.client, ctx, svc.Namespace, svc.Name, ClusterInfo{
			Name:   exporter.ClusterName,
			Zone:   exporter.Zone,
			Region: exporter.Region,
		})
		if err != nil {
			exporter.log.Error(err, "failed to get endpointslices of service")
			return
		}
	} else {
		serviceType = apis.ClusterIP
		endpoints = append(endpoints, apis.Endpoint{
			Addresses: svc.Spec.ClusterIPs,
			Cluster:   exporter.ClusterName,
			Zone:      exporter.Zone,
			Region:    exporter.Region,
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
	err = exporter.ExportGlobalService(ctx, globalService)
	if err != nil {
		log.Error(err, "failed to export service")
		return
	}

	exporter.serviceKeySet.Add(req.NamespacedName)
	return result, nil
}

func (exporter serviceExporter) shouldSkipService(svc corev1.Service) bool {
	return !isGlobalService(svc.Labels) || svc.Spec.Type != corev1.ServiceTypeClusterIP
}

func (exporter serviceExporter) revokeGlobalService(ctx context.Context, serviceKey client.ObjectKey) error {
	log := exporter.log.WithValues("serviceKey", serviceKey)

	if !exporter.serviceKeySet.Has(serviceKey) {
		log.V(5).Info("this service is not exported before, skip revoking")
		return nil
	}

	log.V(5).Info("revoke global service and associated endpoints")
	if err := exporter.RevokeGlobalService(ctx, exporter.ClusterName, serviceKey.Namespace, serviceKey.Name); err != nil {
		log.Error(err, "failed to revoke global service")
		return err
	}

	exporter.serviceKeySet.Delete(serviceKey)

	return nil
}

func isGlobalService(labels map[string]string) bool {
	return labels != nil && labels[labelGlobalService] == "true"
}

func GetEndpointsOfHeadlessService(cli client.Client, ctx context.Context, namespace, serviceName string, cluster ClusterInfo) ([]apis.Endpoint, error) {
	var endpointSliceList discoveryv1.EndpointSliceList
	err := cli.List(ctx, &endpointSliceList,
		client.InNamespace(namespace),
		client.MatchingLabels{
			"kubernetes.io/service-name": serviceName,
		},
	)
	if err != nil {
		return nil, err
	}

	endpointByName := make(map[string]apis.Endpoint)
	sort.Sort(ByAddressType(endpointSliceList.Items))
	for _, es := range endpointSliceList.Items {
		if es.AddressType == discoveryv1.AddressTypeFQDN {
			continue
		}

		for _, ep := range es.Endpoints {
			if e, found := endpointByName[ep.TargetRef.Name]; found {
				e.Addresses = append(e.Addresses, ep.Addresses...)
				endpointByName[ep.TargetRef.Name] = e
			} else {
				endpoint := apis.Endpoint{
					Addresses: ep.Addresses,
					Hostname:  ep.Hostname,
					TargetRef: ep.TargetRef,
					Cluster:   cluster.Name,
					Zone:      cluster.Zone,
					Region:    cluster.Region,
				}
				endpointByName[ep.TargetRef.Name] = endpoint
			}
		}
	}

	endpoints := make([]apis.Endpoint, 0, len(endpointByName))
	for _, ep := range endpointByName {
		endpoints = append(endpoints, ep)
	}
	sort.Sort(ByName(endpoints))

	return endpoints, nil
}

type ByName []apis.Endpoint
type ByAddressType []discoveryv1.EndpointSlice
type ClusterInfo struct {
	Name, Region, Zone string
}

func (a ByName) Len() int           { return len(a) }
func (a ByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByName) Less(i, j int) bool { return a[i].TargetRef.Name < a[j].TargetRef.Name }

func (a ByAddressType) Len() int           { return len(a) }
func (a ByAddressType) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByAddressType) Less(i, j int) bool { return a[i].AddressType < a[j].AddressType }
