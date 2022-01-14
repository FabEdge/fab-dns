module github.com/fabedge/fab-dns

go 1.16

require (
	github.com/caddyserver/caddy v1.0.5
	github.com/coredns/caddy v1.1.1
	github.com/coredns/coredns v1.8.6
	github.com/go-chi/chi/v5 v5.0.0
	github.com/go-logr/logr v1.0.0
	github.com/miekg/dns v1.1.43
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/klog/v2 v2.20.0
	sigs.k8s.io/controller-runtime v0.9.1
)

replace (
	github.com/go-logr/logr => github.com/go-logr/logr v0.3.0
	k8s.io/klog/v2 => k8s.io/klog/v2 v2.4.0
)
