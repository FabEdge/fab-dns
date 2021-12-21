package service_hub

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/klog/v2/klogr"

	"github.com/FabEdge/fab-dns/pkg/service-hub/apiserver"
	"github.com/FabEdge/fab-dns/pkg/service-hub/types"
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

	ClusterExpireTime      time.Duration
	APIServerListenAddress string
	TLSKeyFile             string
	TLSCertFile            string
	TLSCACertFile          string

	ClusterStore *types.ClusterStore
	APIServer    *http.Server
}

func (opts *Options) AddFlags(flag *pflag.FlagSet) {
	flag.StringVar(&opts.Cluster, "cluster", "", "The name of cluster must be unique among all clusters and be a valid dns name(RFC 1123)")
	flag.StringVar(&opts.Zone, "zone", "default", "The zone where the cluster is located, a zone name may contain the letters ‘a-z’ or ’A-Z’ or digits 0-9")
	flag.StringVar(&opts.Region, "region", "default", "The region where the cluster is located, a region name may contain the letters ‘a-z’ or ’A-Z’ or digits 0-9")

	flag.StringVar(&opts.APIServerListenAddress, "api-server-listen-address", "0.0.0.0:3000", "The address on which API server listen")
	flag.StringVar(&opts.TLSKeyFile, "tls-key-file", "", "The key file for API server/client")
	flag.StringVar(&opts.TLSKeyFile, "tls-cert-file", "", "The cert file for API server/client")
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
	opts.ClusterStore = types.NewClusterStore()
	opts.APIServer, err = apiserver.New(apiserver.Config{
		Address:      opts.APIServerListenAddress,
		ClusterStore: opts.ClusterStore,
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

	return nil
}

func (opts Options) Run() error {
	return opts.APIServer.ListenAndServeTLS("", "")
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
