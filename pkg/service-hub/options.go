package service_hub

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2/klogr"
	ctrlpkg "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	apis "github.com/fabedge/fab-dns/pkg/apis/v1alpha1"
	"github.com/fabedge/fab-dns/pkg/service-hub/apiserver"
	"github.com/fabedge/fab-dns/pkg/service-hub/cleaner"
	fclient "github.com/fabedge/fab-dns/pkg/service-hub/client"
	"github.com/fabedge/fab-dns/pkg/service-hub/exporter"
	"github.com/fabedge/fab-dns/pkg/service-hub/importer"
	"github.com/fabedge/fab-dns/pkg/service-hub/types"
)

func init() {
	_ = apis.AddToScheme(scheme.Scheme)
}

const (
	ModeServer = "server"
	ModeClient = "client"
)

var (
	log            = klogr.New().WithName("agent")
	dns1123Reg, _  = regexp.Compile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	zoneNameReg, _ = regexp.Compile(`[a-zA-Z0-9]+`)
)

type Options struct {
	Cluster string
	Zone    string
	Region  string
	Mode    string

	HealthProbeListenAddress string
	APIServerListenAddress   string
	APIServerAddress         string
	TLSKeyFile               string
	TLSCertFile              string
	TLSCACertFile            string

	ClusterExpireTime     time.Duration
	ServiceImportInterval time.Duration
	RequestTimeout        time.Duration
	AllowCreateNamespace  bool

	Manager      ctrlpkg.Manager
	ClusterStore *types.ClusterStore
	APIServer    *http.Server
	Client       fclient.Interface

	ExportGlobalService types.ExportGlobalServiceFunc
	RevokeGlobalService types.RevokeGlobalServiceFunc
}

func (opts *Options) AddFlags(flag *pflag.FlagSet) {
	flag.StringVar(&opts.Mode, "mode", "server", "Mode determines whether to start API server or to use client to import/export global services. Two options: server/client")
	flag.StringVar(&opts.Cluster, "cluster", "", "The name of cluster must be unique among all clusters and be a valid dns name(RFC 1123)")
	flag.StringVar(&opts.Zone, "zone", "default", "The zone where the cluster is located, a zone name may contain the letters ‘a-z’ or ’A-Z’ or digits 0-9")
	flag.StringVar(&opts.Region, "region", "default", "The region where the cluster is located, a region name may contain the letters ‘a-z’ or ’A-Z’ or digits 0-9")

	flag.StringVar(&opts.HealthProbeListenAddress, "health-probe-listen-address", "0.0.0.0:3001", "The address on which health probe listen")
	flag.StringVar(&opts.APIServerListenAddress, "api-server-listen-address", "0.0.0.0:3000", "The address on which API server listen")
	flag.StringVar(&opts.APIServerAddress, "api-server-address", "", "The address with which client uses to visit API server")
	flag.StringVar(&opts.TLSKeyFile, "tls-key-file", "", "The key file for API server/client")
	flag.StringVar(&opts.TLSCertFile, "tls-cert-file", "", "The cert file for API server/client")
	flag.StringVar(&opts.TLSCACertFile, "tls-ca-cert-file", "", "The CA cert file for API server/client")

	flag.DurationVar(&opts.ClusterExpireTime, "cluster-expire-duration", 5*time.Minute, "Expiration time after cluster stops heartbeat")
	flag.DurationVar(&opts.ServiceImportInterval, "service-import-interval", time.Minute, "The interval between each services importing routine")
	flag.DurationVar(&opts.RequestTimeout, "request-timeout", 5*time.Second, "Timeout for kubernetes API request")
	flag.BoolVar(&opts.AllowCreateNamespace, "allow-create-namespace", true, "Determine if service-hub are allowed to create namespace if needed")
}

func (opts Options) Validate() error {
	if !dns1123Reg.MatchString(opts.Cluster) {
		return fmt.Errorf("invalid cluster name: %s", opts.Cluster)
	}

	if !zoneNameReg.MatchString(opts.Zone) {
		return fmt.Errorf("invalid zone name: %s", opts.Zone)
	}

	if opts.Mode != ModeServer && opts.Mode != ModeClient {
		return fmt.Errorf("unsupported mode, only server or client is allowed")
	}

	if !zoneNameReg.MatchString(opts.Region) {
		return fmt.Errorf("invalid region name: %s", opts.Region)
	}

	if !fileExists(opts.TLSKeyFile) {
		return fmt.Errorf("TLS key file does not exist")
	}

	if !fileExists(opts.TLSCertFile) {
		return fmt.Errorf("TLS cert file does not exist")
	}

	if !fileExists(opts.TLSCACertFile) {
		return fmt.Errorf("TLS CA cert file does not exist")
	}

	return nil
}

func (opts *Options) Complete() (err error) {
	if err = opts.initManager(); err != nil {
		return err
	}

	opts.ClusterStore = types.NewClusterStore()
	if opts.Mode == ModeServer {
		err = opts.initAPIServer()
	} else {
		err = opts.initClient()
	}
	if err != nil {
		return err
	}

	return opts.initManagerRunnables()
}

func (opts *Options) initManager() (err error) {
	kubeConfig, err := ctrlpkg.GetConfig()
	if err != nil {
		log.Error(err, "failed to load kubeconfig")
		return err
	}

	opts.Manager, err = ctrlpkg.NewManager(kubeConfig, manager.Options{
		HealthProbeBindAddress: opts.HealthProbeListenAddress,
		Logger:                 log.WithName("service-hub"),
	})
	_ = opts.Manager.AddReadyzCheck("ping", healthz.Ping)

	if err != nil {
		log.Error(err, "failed to create controller manager")
	}

	return err
}

func (opts *Options) initAPIServer() (err error) {
	globalServiceManager := types.NewGlobalServiceManager(opts.Manager.GetClient(), opts.AllowCreateNamespace)
	opts.ExportGlobalService = globalServiceManager.CreateOrMergeGlobalService
	opts.RevokeGlobalService = globalServiceManager.RevokeGlobalService

	opts.APIServer, err = apiserver.New(apiserver.Config{
		Address:               opts.APIServerListenAddress,
		Client:                opts.Manager.GetClient(),
		ClusterStore:          opts.ClusterStore,
		ClusterExpireDuration: opts.ClusterExpireTime,
		RequestTimeout:        opts.RequestTimeout,
		GlobalServiceManager:  globalServiceManager,
		Log:                   log.WithName("apiserver"),
	})
	if err != nil {
		log.Error(err, "failed to create API server")
		return err
	}

	if cert, certPool, err := opts.getKeyPairAndCACertPool(); err != nil {
		return err
	} else {
		opts.APIServer.TLSConfig = &tls.Config{
			ClientCAs:    certPool,
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.RequireAndVerifyClientCert,
		}
	}

	return err
}

func (opts *Options) initClient() error {
	cert, certPool, err := opts.getKeyPairAndCACertPool()
	if err != nil {
		return err
	}

	opts.Client, err = fclient.NewClient(opts.APIServerAddress, opts.Cluster, &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:      certPool,
			Certificates: []tls.Certificate{cert},
		},
	})
	if err != nil {
		log.Error(err, "failed to create API client")
		return err
	}

	opts.ExportGlobalService = opts.Client.UploadGlobalService
	opts.RevokeGlobalService = func(ctx context.Context, clusterName, namespace, serviceName string) error {
		return opts.Client.DeleteGlobalService(ctx, namespace, serviceName)
	}

	err = opts.Client.Heartbeat()
	if err != nil {
		log.Error(err, "failed to send initial hearbeat")
	}

	return err
}

func (opts Options) getKeyPairAndCACertPool() (tls.Certificate, *x509.CertPool, error) {
	cert, err := tls.LoadX509KeyPair(opts.TLSCertFile, opts.TLSKeyFile)
	if err != nil {
		log.Error(err, "failed to load key pair")
		return tls.Certificate{}, nil, err
	}

	caCertPEM, err := ioutil.ReadFile(opts.TLSCACertFile)
	if err != nil {
		log.Error(err, "failed to read CA cert file")
		return tls.Certificate{}, nil, err
	}
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caCertPEM)

	return cert, certPool, err
}

func (opts Options) initManagerRunnables() (err error) {
	if opts.Mode == ModeServer {
		if err = opts.Manager.Add(manager.RunnableFunc(opts.runAPIServer)); err != nil {
			log.Error(err, "failed to add API Server to manager")
			return err
		}

		if err = cleaner.AddToManager(cleaner.Config{
			Manager:             opts.Manager,
			Store:               opts.ClusterStore,
			Interval:            opts.ClusterExpireTime,
			RequestTimeout:      opts.RequestTimeout,
			RevokeGlobalService: opts.RevokeGlobalService,
		}); err != nil {
			log.Error(err, "failed to add cluster cleaner to manager")
			return err
		}
	} else {
		err = importer.AddToManager(importer.Config{
			Interval:             opts.ServiceImportInterval,
			RequestTimeout:       opts.RequestTimeout,
			Manager:              opts.Manager,
			GetGlobalServices:    opts.Client.DownloadAllGlobalServices,
			AllowCreateNamespace: opts.AllowCreateNamespace,
		})
		if err != nil {
			log.Error(err, "failed to add service importer")
			return err
		}
	}

	err = exporter.AddToManager(exporter.Config{
		ClusterName:         opts.Cluster,
		Zone:                opts.Zone,
		Region:              opts.Region,
		Manager:             opts.Manager,
		ExportGlobalService: opts.ExportGlobalService,
		RevokeGlobalService: opts.RevokeGlobalService,
	})
	if err != nil {
		log.Error(err, "failed to add global service exporter to manager")
		return err
	}

	return nil
}

func (opts Options) Run() error {
	if opts.Mode == ModeServer {
		go opts.recordGlobalServicesByCluster()
	}

	if err := opts.Manager.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "failed to start controller manager")
		return err
	}

	return nil
}

func (opts Options) runAPIServer(ctx context.Context) error {
	errChan := make(chan error)

	go func() {
		var err error
		err = opts.APIServer.ListenAndServeTLS("", "")
		if err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	var err error
	select {
	case err = <-errChan:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		err = ctx.Err()
	}

	return err
}

// recordGlobalServicesByCluster will record global services to ClusterStore by cluster
func (opts Options) recordGlobalServicesByCluster() {
	<-opts.Manager.Elected()

	cli := opts.Manager.GetClient()
	var globalServices apis.GlobalServiceList
	if err := cli.List(context.Background(), &globalServices); err != nil {
		log.Error(err, "failed to list all global services")
		return
	}

	for _, gs := range globalServices.Items {
		for _, endpoint := range gs.Spec.Endpoints {
			// if an endpoint's cluster is the same as the cluster argument,
			// it's not necessary to record this cluster's global service
			// since this cluster's service-hub is running in server mode
			if endpoint.Cluster == opts.Cluster {
				continue
			}

			if opts.ClusterStore.Get(endpoint.Cluster) == nil {
				cluster := opts.ClusterStore.New(endpoint.Cluster)
				cluster.SetExpireTime(time.Now().Add(opts.ClusterExpireTime))

				cluster.AddServiceKey(client.ObjectKey{
					Name:      gs.Name,
					Namespace: gs.Namespace,
				})
			}
		}
	}
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
