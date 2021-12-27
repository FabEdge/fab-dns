OUTPUT_DIR ?= _output

GOLDFLAGS ?= -s -w
LDFLAGS := -ldflags "${GOLDFLAGS}"

CRD_OPTIONS ?= "crd:trivialVersions=true"
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

service-hub: fmt vet
	GOOS=${GOOS} go build ${LDFLAGS}  -o ${OUTPUT_DIR}/$@ ./cmd/$@

.PHONY: test
test:
ifneq (,$(shell which ginkgo))
	ginkgo ./pkg/...
else
	go test ./pkg/...
endif

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
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.5 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

install-test-dependencies:
	curl -sL https://github.com/kubernetes-sigs/kubebuilder/releases/download/v$(KUBEBUILDER_VERSION)/kubebuilder_$(KUBEBUILDER_VERSION)_$(GOOS)_$(GOARCH).tar.gz | \
                    tar -zx -C ${GOBIN} --strip-components=2
