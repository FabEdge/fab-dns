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
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	apis "github.com/FabEdge/fab-dns/pkg/apis/v1alpha1"
	"github.com/FabEdge/fab-dns/pkg/service-hub/apiserver"
	"github.com/FabEdge/fab-dns/pkg/service-hub/exporter"
	"github.com/FabEdge/fab-dns/pkg/service-hub/types"
)

func init() {
	_ = apis.AddToScheme(scheme.Scheme)
}

var (
	log            = klogr.New().WithName("agent")
	dns1123Reg, _  = regexp.Compile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	zoneNameReg, _ = regexp.Compile(`[a-zA-Z0-9]+`)
)

type Options struct {
	Cluster string
	Zone    string
	Region  string

	ClusterExpireTime      time.Duration
	APIServerListenAddress string
	TLSKeyFile             string
	TLSCertFile            string
	TLSCACertFile          string

	GlobalServiceManager types.GlobalServiceManager
	Manager              ctrlpkg.Manager
	ClusterStore         *types.ClusterStore
	APIServer            *http.Server
}

func (opts *Options) AddFlags(flag *pflag.FlagSet) {
	flag.StringVar(&opts.Cluster, "cluster", "", "The name of cluster must be unique among all clusters and be a valid dns name(RFC 1123)")
	flag.StringVar(&opts.Zone, "zone", "default", "The zone where the cluster is located, a zone name may contain the letters ‘a-z’ or ’A-Z’ or digits 0-9")
	flag.StringVar(&opts.Region, "region", "default", "The region where the cluster is located, a region name may contain the letters ‘a-z’ or ’A-Z’ or digits 0-9")

	flag.StringVar(&opts.APIServerListenAddress, "api-server-listen-address", "0.0.0.0:3000", "The address on which API server listen")
	flag.StringVar(&opts.TLSKeyFile, "tls-key-file", "", "The key file for API server/client")
	flag.StringVar(&opts.TLSCertFile, "tls-cert-file", "", "The cert file for API server/client")
	flag.StringVar(&opts.TLSCACertFile, "tls-ca-cert-file", "", "The CA cert file for API server/client")
	flag.DurationVar(&opts.ClusterExpireTime, "cluster-expire-duration", 5*time.Minute, "Expiration time after cluster stops heartbeat")
}

func (opts Options) Validate() error {
	if !dns1123Reg.MatchString(opts.Cluster) {
		return fmt.Errorf("invalid cluster name: %s", opts.Cluster)
	}

	if !zoneNameReg.MatchString(opts.Zone) {
		return fmt.Errorf("invalid zone name: %s", opts.Zone)
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

	opts.GlobalServiceManager = types.NewGlobalServiceManager(opts.Manager.GetClient())
	opts.ClusterStore = types.NewClusterStore()

	if err = opts.initAPIServer(); err != nil {
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
		Logger: log.WithName("service-hub"),
	})

	return nil
}

func (opts *Options) initAPIServer() (err error) {
	opts.APIServer, err = apiserver.New(apiserver.Config{
		Address:               opts.APIServerListenAddress,
		Client:                opts.Manager.GetClient(),
		ClusterStore:          opts.ClusterStore,
		ClusterExpireDuration: opts.ClusterExpireTime,
		GlobalServiceManager:  opts.GlobalServiceManager,
		Log:                   log.WithName("apiserver"),
	})
	if err != nil {
		log.Error(err, "failed to create API server")
		return err
	}

	caCertPEM, err := ioutil.ReadFile(opts.TLSCACertFile)
	if err != nil {
		log.Error(err, "failed to read CA cert file")
		return err
	}
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caCertPEM)
	cert, err := tls.LoadX509KeyPair(opts.TLSCertFile, opts.TLSKeyFile)
	opts.APIServer.TLSConfig = &tls.Config{
		ClientCAs:    certPool,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}

	return err
}

func (opts Options) initManagerRunnables() (err error) {
	if err = opts.Manager.Add(manager.RunnableFunc(opts.runAPIServer)); err != nil {
		log.Error(err, "failed to add API Server to manager")
		return err
	}

	err = exporter.AddToManager(exporter.Config{
		ClusterName: opts.Cluster,
		Zone:        opts.Zone,
		Region:      opts.Region,
		Manager:     opts.Manager,

		ExportGlobalService: opts.GlobalServiceManager.CreateOrMergeGlobalService,
		RecallGlobalService: opts.GlobalServiceManager.RecallGlobalService,
	})
	if err != nil {
		log.Error(err, "failed to add global service exporter to manager")
		return err
	}

	return nil
}

func (opts Options) Run() error {
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

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
