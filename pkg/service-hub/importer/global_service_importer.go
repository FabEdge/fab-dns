package importer

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlpkg "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
	"github.com/fabedge/fab-dns/pkg/constants"
	"github.com/fabedge/fab-dns/pkg/service-hub/types"
	nsutil "github.com/fabedge/fab-dns/pkg/util/namespace"
)

type GetGlobalServicesFunc func(ctx context.Context) ([]apis.GlobalService, error)
type Config struct {
	Interval             time.Duration
	RequestTimeout       time.Duration
	Manager              ctrlpkg.Manager
	GetGlobalServices    GetGlobalServicesFunc
	AllowCreateNamespace bool
}

func AddToManager(cfg Config) error {
	if cfg.Interval == 0 {
		return fmt.Errorf("interval is too small")
	}

	if cfg.RequestTimeout == 0 {
		return fmt.Errorf("request timeout is too small")
	}

	if cfg.Manager == nil {
		return fmt.Errorf("controller manager is required")
	}

	if cfg.GetGlobalServices == nil {
		return fmt.Errorf("GetGlobalServices is required")
	}

	return cfg.Manager.Add(&globalServiceImporter{
		Config: cfg,
		client: cfg.Manager.GetClient(),
		log:    cfg.Manager.GetLogger(),
	})
}

type globalServiceImporter struct {
	Config
	client client.Client
	log    logr.Logger
}

func (importer *globalServiceImporter) Start(ctx context.Context) error {
	tick := time.NewTicker(importer.Interval)
	defer tick.Stop()

	importer.importServices()
	for {
		select {
		case <-tick.C:
			importer.importServices()
		case <-ctx.Done():
			return nil
		}
	}
}

func (importer *globalServiceImporter) importServices() {
	ctx, cancel := context.WithTimeout(context.Background(), importer.RequestTimeout)
	defer cancel()

	services, err := importer.GetGlobalServices(ctx)
	if err != nil {
		importer.log.Error(err, "failed to get global services")
		return
	}

	importedKeySet := types.NewObjectKeySet()
	for _, svc := range services {
		importedKeySet.Add(keyFromObject(&svc))
		go importer.createOrUpdateGlobalService(svc)
	}

	var localGlobalServices apis.GlobalServiceList
	if err = importer.client.List(ctx, &localGlobalServices); err != nil {
		importer.log.Error(err, "failed to list local global services")
		return
	}

	for _, svc := range localGlobalServices.Items {
		if !importedKeySet.Has(keyFromObject(&svc)) {
			go importer.deleteService(svc)
		}
	}
}

func (importer *globalServiceImporter) createOrUpdateGlobalService(sourceService apis.GlobalService) {
	ctx, cancel := context.WithTimeout(context.Background(), importer.RequestTimeout)
	defer cancel()

	if importer.AllowCreateNamespace {
		if err := nsutil.Ensure(ctx, importer.client, sourceService.Namespace); err != nil {
			importer.log.Error(err, "failed to create namespace", "namespace", sourceService.Namespace)
			return
		}
	}

	service := &apis.GlobalService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceService.Name,
			Namespace: sourceService.Namespace,
		},
	}

	_, err := ctrlpkg.CreateOrUpdate(ctx, importer.client, service, func() error {
		if len(service.Labels) == 0 {
			service.Labels = map[string]string{
				constants.KeyCreatedBy: constants.AppServiceHub,
			}
		}

		originResourceVersion := service.Labels[constants.KeyOriginResourceVersion]
		if originResourceVersion != "" && originResourceVersion == sourceService.ResourceVersion {
			return nil
		}

		service.Labels[constants.KeyOriginResourceVersion] = sourceService.ResourceVersion
		service.Spec = sourceService.Spec

		return nil
	})

	if err != nil {
		importer.log.Error(err, "failed to create or update global service", "globalService", *service)
	}
}

func (importer *globalServiceImporter) deleteService(svc apis.GlobalService) {
	ctx, cancel := context.WithTimeout(context.Background(), importer.RequestTimeout)
	defer cancel()

	if err := importer.client.Delete(ctx, &svc); err != nil {
		importer.log.Error(err, "failed to delete global service", "globalService", svc)
	}
}

func keyFromObject(obj client.Object) client.ObjectKey {
	return client.ObjectKey{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
}
