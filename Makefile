OUTPUT_DIR ?= _output

VERSION := v0.1.0.alpha
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S%z')
GIT_COMMIT := $(shell git rev-parse --short HEAD)
META := github.com/fabedge/fab-dns/pkg/about
FLAG_VERSION := ${META}.version=${VERSION}
FLAG_BUILD_TIME := ${META}.buildTime=${BUILD_TIME}
FLAG_GIT_COMMIT := ${META}.gitCommit=${GIT_COMMIT}
GOLDFLAGS ?= -s -w
LDFLAGS := -ldflags "${GOLDFLAGS} -X ${FLAG_VERSION} -X ${FLAG_BUILD_TIME} -X ${FLAG_GIT_COMMIT}"

CRD_OPTIONS ?= "crd:generateEmbeddedObjectMeta=true"
KUBEBUILDER_VERSION ?= 2.3.1
GOOS ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
GOARCH ?= amd64
# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

export KUBEBUILDER_ASSETS ?= $(GOBIN)
export ACK_GINKGO_DEPRECATIONS ?= 1.16.4

fmt:
	go fmt ./...

vet:
	go vet ./...

bin: $(if $(QUICK),, fmt vet) service-hub

service-hub:
	GOOS=${GOOS} go build ${LDFLAGS}  -o ${OUTPUT_DIR}/$@ ./cmd/$@

service-hub-image:
	docker build -t fabedge/service-hub:latest -f build/service-hub/Dockerfile .

fabdns:
	GOOS=${GOOS} go build -ldflags="-X github.com/coredns/coredns/coremain.GitCommit=$(GIT_COMMIT)" -o ${OUTPUT_DIR}/$@ ./cmd/$@

fabdns-image:
	docker build -t fabedge/fabdns:latest -f build/fabdns/Dockerfile .

.PHONY: test
test:
ifneq (,$(shell which ginkgo))
	ginkgo ./pkg/...
else
	go test ./pkg/...
endif

e2e-test:
	go test ${LDFLAGS} -c ./test/e2e -o ${OUTPUT_DIR}/fabdns-e2e.test

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=fabedge-admin paths="./pkg/..." output:dir:crd=deploy/crd

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object paths="./pkg/..."

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.7.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

install-test-dependencies:
	curl -sL https://github.com/kubernetes-sigs/kubebuilder/releases/download/v$(KUBEBUILDER_VERSION)/kubebuilder_$(KUBEBUILDER_VERSION)_$(GOOS)_$(GOARCH).tar.gz | \
                    tar -zx -C /usr/local/kubebuilder/bin/  --strip-components=2
