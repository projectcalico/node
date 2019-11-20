PACKAGE_NAME?=github.com/projectcalico/node
GO_BUILD_VER?=v0.27

###############################################################################
# Download and include Makefile.common before anything else
###############################################################################
MAKE_BRANCH?=$(GO_BUILD_VER)
MAKE_REPO?=https://raw.githubusercontent.com/projectcalico/go-build/$(MAKE_BRANCH)

Makefile.common: Makefile.common.$(MAKE_BRANCH)
	cp "$<" "$@"
Makefile.common.$(MAKE_BRANCH):
	# Clean up any files downloaded from other branches so they don't accumulate.
	rm -f Makefile.common.*
	wget -nv $(MAKE_REPO)/Makefile.common -O "$@"

include Makefile.common

###############################################################################

# list of arches *not* to build when doing *-all
#    until s390x works correctly
EXCLUDEARCH ?= s390x
VALIDARCHES = $(filter-out $(EXCLUDEARCH),$(ARCHES))

###############################################################################
BUILD_IMAGE?=calico/node
PUSH_IMAGES?=$(BUILD_IMAGE) quay.io/calico/node
RELEASE_IMAGES?=gcr.io/projectcalico-org/node eu.gcr.io/projectcalico-org/node asia.gcr.io/projectcalico-org/node us.gcr.io/projectcalico-org/node

# Versions and location of dependencies used in the build.
BIRD_VERSION=v0.3.3-145-g049d13dc
BIRD_IMAGE ?= calico/bird:$(BIRD_VERSION)-$(ARCH)

# Versions and locations of dependencies used in tests.
CALICOCTL_VER?=master
CNI_VER?=master
TEST_CONTAINER_NAME_VER?=latest
CTL_CONTAINER_NAME?=calico/ctl:$(CALICOCTL_VER)-$(ARCH)
TEST_CONTAINER_NAME?=calico/test:$(TEST_CONTAINER_NAME_VER)-$(ARCH)
# If building on amd64 omit the arch in the container name.  Fixme!
ETCD_IMAGE?=quay.io/coreos/etcd:$(ETCD_VERSION)
ifneq ($(BUILDARCH),amd64)
	ETCD_IMAGE=$(ETCD_IMAGE)-$(ARCH)
endif

HYPERKUBE_IMAGE?=gcr.io/google_containers/hyperkube-$(ARCH):$(K8S_VERSION)
TEST_CONTAINER_FILES=$(shell find tests/ -type f ! -name '*.created')

# Variables controlling the image
NODE_CONTAINER_CREATED=.calico_node.created-$(ARCH)
NODE_CONTAINER_BIN_DIR=./dist/bin/
NODE_CONTAINER_BINARY = $(NODE_CONTAINER_BIN_DIR)/calico-node-$(ARCH)

# Variables used by the tests
LOCAL_IP_ENV?=$(shell ip route get 8.8.8.8 | head -1 | awk '{print $$7}')
ST_TO_RUN?=tests/st/
K8ST_TO_RUN?=tests/
# Can exclude the slower tests with "-a '!slow'"
ST_OPTIONS?=

# Variables for building the local binaries that go into the image
MAKE_SURE_BIN_EXIST := $(shell mkdir -p dist $(NODE_CONTAINER_BIN_DIR))
NODE_CONTAINER_FILES=$(shell find ./filesystem -type f)

# Calculate a timestamp for any build artefacts.
DATE:=$(shell date -u +'%FT%T%z')

LDFLAGS=-ldflags "\
	-X $(PACKAGE_NAME)/pkg/startup.VERSION=$(CALICO_GIT_VER) \
	-X $(PACKAGE_NAME)/buildinfo.GitVersion=$(GIT_DESCRIPTION) \
	-X $(PACKAGE_NAME)/buildinfo.BuildDate=$(DATE) \
	-X $(PACKAGE_NAME)/buildinfo.GitRevision=$(GIT_COMMIT)"

SRC_FILES=$(shell find ./pkg -name '*.go')

# Build mounts for running in "local build" mode. This allows an easy build using local development code,
# assuming that there is a local checkout of libcalico in the same directory as this repo.
PHONY:local_build

ifdef LOCAL_BUILD
EXTRA_DOCKER_ARGS+=-v $(CURDIR)/../libcalico-go:/go/src/github.com/projectcalico/libcalico-go:rw \
	-v $(CURDIR)/../felix:/go/src/github.com/projectcalico/felix:rw \
	-v $(CURDIR)/../typha:/go/src/github.com/projectcalico/typha:rw \
	-v $(CURDIR)/../confd:/go/src/github.com/projectcalico/confd:rw
local_build:
	$(DOCKER_RUN) $(CALICO_BUILD) go mod edit -replace=github.com/projectcalico/libcalico-go=../libcalico-go
	$(DOCKER_RUN) $(CALICO_BUILD) go mod edit -replace=github.com/projectcalico/felix=../felix
	$(DOCKER_RUN) $(CALICO_BUILD) go mod edit -replace=github.com/projectcalico/typha=../typha
	$(DOCKER_RUN) $(CALICO_BUILD) go mod edit -replace=github.com/kelseyhightower/confd=../confd
else
local_build:
	@echo "Building node"
endif

## Clean enough that a new release build will be clean
clean:
	find . -name '*.created' -exec rm -f {} +
	find . -name '*.pyc' -exec rm -f {} +
	rm -rf .go-pkg-cache
	rm -rf certs *.tar $(NODE_CONTAINER_BIN_DIR)
	rm -rf dist
	rm -rf filesystem/etc/calico/confd/conf.d filesystem/etc/calico/confd/config filesystem/etc/calico/confd/templates
	rm -f crds.yaml
	# Delete images that we built in this repo
	docker rmi $(BUILD_IMAGE):latest-$(ARCH) || true
	docker rmi $(TEST_CONTAINER_NAME) || true

###############################################################################
# Updating pins
###############################################################################
update-pins: update-libcalico-pin update-felix-pin update-confd-pin

###############################################################################
# Building the binary
###############################################################################
build:  $(NODE_CONTAINER_BINARY)

remote-deps: mod-download
	mkdir -p filesystem/etc/calico/confd
	$(DOCKER_RUN) $(CALICO_BUILD) sh -c ' \
		cp -r `go list -m -f "{{.Dir}}" github.com/kelseyhightower/confd`/etc/calico/confd/conf.d filesystem/etc/calico/confd/conf.d; \
		cp -r `go list -m -f "{{.Dir}}" github.com/kelseyhightower/confd`/etc/calico/confd/config filesystem/etc/calico/confd/config; \
		cp -r `go list -m -f "{{.Dir}}" github.com/kelseyhightower/confd`/etc/calico/confd/templates filesystem/etc/calico/confd/templates; \
		cp `go list -m -f "{{.Dir}}" github.com/projectcalico/libcalico-go`/test/crds.yaml crds.yaml; \
		chmod -R +w filesystem/etc/calico/confd/ crds.yaml'

$(NODE_CONTAINER_BINARY): local_build $(SRC_FILES)
	$(DOCKER_RUN) $(CALICO_BUILD) go build -v -o $@ $(BUILD_FLAGS) $(LDFLAGS) ./cmd/calico-node/main.go

###############################################################################
# Building the image
###############################################################################
## Create the image for the current ARCH
image: remote-deps $(BUILD_IMAGE)
## Create the images for all supported ARCHes
image-all: $(addprefix sub-image-,$(VALIDARCHES))
sub-image-%:
	$(MAKE) image ARCH=$*

$(BUILD_IMAGE): $(NODE_CONTAINER_CREATED)
$(NODE_CONTAINER_CREATED): register ./Dockerfile.$(ARCH) $(NODE_CONTAINER_FILES) $(NODE_CONTAINER_BINARY) remote-deps
ifeq ($(LOCAL_BUILD),true)
	# If doing a local build, copy in local confd templates in case there are changes.
	rm -rf filesystem/etc/calico/confd/templates
	cp -r ../confd/etc/calico/confd/templates filesystem/etc/calico/confd/templates
endif
	# Check versions of the binaries that we're going to use to build the image.
	# Since the binaries are built for Linux, run them in a container to allow the
	# make target to be run on different platforms (e.g. MacOS).
	docker run --rm -v $(CURDIR)/dist/bin:/go/bin:rw $(CALICO_BUILD) /bin/sh -c "\
	  echo; echo calico-node-$(ARCH) -v;	 /go/bin/calico-node-$(ARCH) -v; \
	"
	docker build --pull -t $(BUILD_IMAGE):latest-$(ARCH) . --build-arg BIRD_IMAGE=$(BIRD_IMAGE) --build-arg QEMU_IMAGE=$(CALICO_BUILD) --build-arg GIT_VERSION=$(CALICO_GIT_VER) -f ./Dockerfile.$(ARCH)
	touch $@

###############################################################################
# FV Tests
###############################################################################
## Run the ginkgo FVs
fv: run-k8s-apiserver
	docker run --rm \
	-v $(CURDIR):/go/src/$(PACKAGE_NAME):rw \
	-e LOCAL_USER_ID=$(LOCAL_USER_ID) \
	-e ETCD_ENDPOINTS=http://$(LOCAL_IP_ENV):2379 \
	-e GO111MODULE=on \
	--net=host \
	-w /go/src/$(PACKAGE_NAME) \
	$(CALICO_BUILD) ginkgo -cover -r -skipPackage vendor pkg/startup pkg/allocateip $(GINKGO_ARGS)

# etcd is used by the STs
.PHONY: run-etcd
run-etcd:
	@-docker rm -f calico-etcd
	docker run --detach \
	--net=host \
	--name calico-etcd $(ETCD_IMAGE) \
	etcd \
	--advertise-client-urls "http://$(LOCAL_IP_ENV):2379,http://127.0.0.1:2379" \
	--listen-client-urls "http://0.0.0.0:2379"

# Kubernetes apiserver used for tests
run-k8s-apiserver: remote-deps stop-k8s-apiserver run-etcd
	docker run \
		--net=host --name st-apiserver \
		-v $(CURDIR):/manifests \
		-v $(CURDIR):/go/src/$(PACKAGE_NAME):rw \
		--detach \
		${HYPERKUBE_IMAGE} sh -c '\
		go mod download; \
		/hyperkube apiserver \
			--bind-address=0.0.0.0 \
			--insecure-bind-address=0.0.0.0 \
			--etcd-servers=http://127.0.0.1:2379 \
			--admission-control=NamespaceLifecycle,LimitRanger,DefaultStorageClass,ResourceQuota \
			--authorization-mode=RBAC \
			--service-cluster-ip-range=10.101.0.0/16 \
			--v=10 \
			--logtostderr=true'

	# Wait until we can configure a cluster role binding which allows anonymous auth.
	while ! docker exec st-apiserver kubectl create \
		clusterrolebinding anonymous-admin \
		--clusterrole=cluster-admin \
		--user=system:anonymous 2>/dev/null ; \
		do echo "Waiting for st-apiserver to come up"; \
		sleep 1; \
		done

	# ClusterRoleBinding created

	# Create CustomResourceDefinition (CRD) for Calico resources
	# from the manifest crds.yaml
	while ! docker exec st-apiserver kubectl \
		apply -f /manifests/crds.yaml; \
		do echo "Trying to create CRDs"; \
		sleep 1; \
		done

# Stop Kubernetes apiserver
stop-k8s-apiserver:
	@-docker rm -f st-apiserver

ut:
	@echo "No UTs available"

###############################################################################
# System tests
# - Support for running etcd (both securely and insecurely)
###############################################################################
# Pull calicoctl and CNI plugin binaries with versions as per XXX_VER
# variables.  These are used for the STs.
dist/calicoctl:
	-docker rm -f calicoctl
	docker pull $(CTL_CONTAINER_NAME)
	docker create --name calicoctl $(CTL_CONTAINER_NAME)
	docker cp calicoctl:calicoctl dist/calicoctl && \
	  test -e dist/calicoctl && \
	  touch dist/calicoctl
	-docker rm -f calicoctl

dist/calico-cni-plugin dist/calico-ipam-plugin:
	-docker rm -f calico-cni
	docker pull calico/cni:$(CNI_VER)
	docker create --name calico-cni calico/cni:$(CNI_VER)
	docker cp calico-cni:/opt/cni/bin/calico dist/calico-cni-plugin && \
	  test -e dist/calico-cni-plugin && \
	  touch dist/calico-cni-plugin
	docker cp calico-cni:/opt/cni/bin/calico-ipam dist/calico-ipam-plugin && \
	  test -e dist/calico-ipam-plugin && \
	  touch dist/calico-ipam-plugin
	-docker rm -f calico-cni

# Create images for containers used in the tests
busybox.tar:
	docker pull $(ARCH)/busybox:latest
	docker save --output busybox.tar $(ARCH)/busybox:latest

workload.tar:
	cd workload && docker build -t workload --build-arg QEMU_IMAGE=$(CALICO_BUILD) -f Dockerfile.$(ARCH) .
	docker save --output workload.tar workload

stop-etcd:
	@-docker rm -f calico-etcd

IPT_ALLOW_ETCD:=-A INPUT -i docker0 -p tcp --dport 2379 -m comment --comment "calico-st-allow-etcd" -j ACCEPT

# Create the calico/test image
test_image: calico_test.created
calico_test.created: $(TEST_CONTAINER_FILES)
	cd calico_test && docker build --build-arg QEMU_IMAGE=$(CALICO_BUILD) -f Dockerfile.$(ARCH).calico_test -t $(TEST_CONTAINER_NAME) .
	touch calico_test.created

calico-node.tar: $(NODE_CONTAINER_CREATED)
	# Check versions of the Calico binaries that will be in calico-node.tar.
	# Since the binaries are built for Linux, run them in a container to allow the
	# make target to be run on different platforms (e.g. MacOS).
	docker run --rm $(BUILD_IMAGE):latest-$(ARCH) /bin/sh -c "\
	  echo bird --version;	 /bin/bird --version; \
	"
	docker save --output $@ $(BUILD_IMAGE):latest-$(ARCH)

.PHONY: st-checks
st-checks:
	# Check that we're running as root.
	test `id -u` -eq '0' || { echo "STs must be run as root to allow writes to /proc"; false; }

	# Insert an iptables rule to allow access from our test containers to etcd
	# running on the host.
	iptables-save | grep -q 'calico-st-allow-etcd' || iptables $(IPT_ALLOW_ETCD)

## Get the kubeadm-dind-cluster script
K8ST_VERSION?=v1.12
DIND_SCR?=dind-cluster-$(K8ST_VERSION).sh

.PHONY: k8s-test
## Run the k8s tests
k8s-test:
	$(MAKE) k8s-stop
	$(MAKE) k8s-start
	$(MAKE) k8s-run-test
	#$(MAKE) k8s-stop

.PHONY: k8s-start
## Start k8s cluster
k8s-start: $(NODE_CONTAINER_CREATED) tests/k8st/$(DIND_SCR)
	CNI_PLUGIN=calico \
	CALICO_VERSION=master \
	CALICO_NODE_IMAGE=$(BUILD_IMAGE):latest-$(ARCH) \
	POD_NETWORK_CIDR=192.168.0.0/16 \
	SKIP_SNAPSHOT=y \
	tests/k8st/$(DIND_SCR) up

.PHONY: k8s-stop
## Stop k8s cluster
k8s-stop: tests/k8st/$(DIND_SCR)
	tests/k8st/$(DIND_SCR) down
	tests/k8st/$(DIND_SCR) clean

.PHONY: k8s-run-test
## Run k8st in an existing k8s cluster
k8s-run-test: calico_test.created
## Only execute remove-go-build-image if flag is set
ifeq ($(REMOVE_GOBUILD_IMG),true)
	$(MAKE) remove-go-build-image
endif
	docker run \
	    -v $(CURDIR):/code \
	    -v /var/run/docker.sock:/var/run/docker.sock \
	    -v /home/$(USER)/.kube/config:/root/.kube/config \
	    -v /home/$(USER)/.kubeadm-dind-cluster:/root/.kubeadm-dind-cluster \
	    --privileged \
	    --net host \
	$(TEST_CONTAINER_NAME) \
	    sh -c 'cp /root/.kubeadm-dind-cluster/kubectl /bin/kubectl && ls -ltr /bin/kubectl && which kubectl && cd /code/tests/k8st && \
		   nosetests $(K8ST_TO_RUN) -v --with-xunit --xunit-file="/code/report/k8s-tests.xml" --with-timer'

# Needed for Semaphore CI (where disk space is a real issue during k8s-test)
.PHONY: remove-go-build-image
remove-go-build-image:
	@echo "Removing $(CALICO_BUILD) image to save space needed for testing ..."
	@-docker rmi $(CALICO_BUILD)

.PHONY: st
## Run the system tests
st: remote-deps dist/calicoctl busybox.tar calico-node.tar workload.tar run-etcd calico_test.created dist/calico-cni-plugin dist/calico-ipam-plugin
	# Check versions of Calico binaries that ST execution will use.
	docker run --rm -v $(CURDIR)/dist:/go/bin:rw $(CALICO_BUILD) /bin/sh -c "\
	  echo; echo calicoctl version;	  /go/bin/calicoctl version; \
	  echo; echo calico-cni-plugin -v;       /go/bin/calico-cni-plugin -v; \
	  echo; echo calico-ipam-plugin -v;      /go/bin/calico-ipam-plugin -v; echo; \
	"
	# Use the host, PID and network namespaces from the host.
	# Privileged is needed since 'calico node' write to /proc (to enable ip_forwarding)
	# Map the docker socket in so docker can be used from inside the container
	# HOST_CHECKOUT_DIR is used for volume mounts on containers started by this one.
	# All of code under test is mounted into the container.
	#   - This also provides access to calicoctl and the docker client
	# $(MAKE) st-checks
	docker run --uts=host \
		   --pid=host \
		   --net=host \
		   --privileged \
		   -v $(CURDIR):/code \
		   -e HOST_CHECKOUT_DIR=$(CURDIR) \
		   -e DEBUG_FAILURES=$(DEBUG_FAILURES) \
		   -e MY_IP=$(LOCAL_IP_ENV) \
		   -e NODE_CONTAINER_NAME=$(BUILD_IMAGE):latest-$(ARCH) \
		   --rm -t \
		   -v /var/run/docker.sock:/var/run/docker.sock \
		   $(TEST_CONTAINER_NAME) \
		   sh -c 'nosetests $(ST_TO_RUN) -v --with-xunit --xunit-file="/code/report/nosetests.xml" --with-timer $(ST_OPTIONS)'
	$(MAKE) stop-etcd

###############################################################################
# CI/CD
###############################################################################
.PHONY: ci
ci: mod-download static-checks fv image-all st

## Deploys images to registry
cd:
ifndef CONFIRM
	$(error CONFIRM is undefined - run using make <target> CONFIRM=true)
endif
ifndef BRANCH_NAME
	$(error BRANCH_NAME is undefined - run using make <target> BRANCH_NAME=var or set an environment variable)
endif
	$(MAKE) tag-images-all push-all push-manifests push-non-manifests IMAGETAG=${BRANCH_NAME} EXCLUDEARCH="$(EXCLUDEARCH)"
	$(MAKE) tag-images-all push-all push-manifests push-non-manifests IMAGETAG=$(shell git describe --tags --dirty --always --long) EXCLUDEARCH="$(EXCLUDEARCH)"

###############################################################################
# Release
###############################################################################
GIT_VERSION?=$(shell git describe --tags --dirty)
PREVIOUS_RELEASE=$(shell git describe --tags --abbrev=0)

## Tags and builds a release from start to finish.
release: release-prereqs
	$(MAKE) VERSION=$(VERSION) release-tag
	$(MAKE) VERSION=$(VERSION) release-build
	$(MAKE) VERSION=$(VERSION) release-verify

	@echo ""
	@echo "Release build complete. Next, push the produced images."
	@echo ""
	@echo "  make VERSION=$(VERSION) release-publish"
	@echo ""

## Produces a git tag for the release.
release-tag: release-prereqs release-notes
	git tag $(VERSION) -F release-notes-$(VERSION)
	@echo ""
	@echo "Now you can build the release:"
	@echo ""
	@echo "  make VERSION=$(VERSION) release-build"
	@echo ""

## Produces a clean build of release artifacts at the specified version.
release-build: release-prereqs clean
# Check that the correct code is checked out.
ifneq ($(VERSION), $(GIT_VERSION))
	$(error Attempt to build $(VERSION) from $(GIT_VERSION))
endif
	$(MAKE) image-all
	$(MAKE) tag-images-all RELEASE=true IMAGETAG=$(VERSION)
	# Generate the `latest` images.
	$(MAKE) tag-images-all RELEASE=true IMAGETAG=latest

## Verifies the release artifacts produces by `make release-build` are correct.
release-verify: release-prereqs
	# Check the reported version is correct for each release artifact.
	if ! docker run $(BUILD_IMAGE):$(VERSION)-$(ARCH) versions | grep '^$(VERSION)$$'; then echo "Reported version:" `docker run $(BUILD_IMAGE):$(VERSION)-$(ARCH) versions` "\nExpected version: $(VERSION)"; false; else echo "\nVersion check passed\n"; fi

## Generates release notes based on commits in this version.
release-notes: release-prereqs
	mkdir -p dist
	echo "# Changelog" > release-notes-$(VERSION)
	echo "" > release-notes-$(VERSION)
	sh -c "git cherry -v $(PREVIOUS_RELEASE) | cut '-d ' -f 2- | sed 's/^/- /' >> release-notes-$(VERSION)"

## Pushes a github release and release artifacts produced by `make release-build`.
release-publish: release-prereqs
	# Push the git tag.
	git push origin $(VERSION)

	# Push images.
	$(MAKE) push-all push-manifests push-non-manifests RELEASE=true IMAGETAG=$(VERSION)

	@echo "Finalize the GitHub release based on the pushed tag."
	@echo ""
	@echo "  https://$(PACKAGE_NAME)/releases/tag/$(VERSION)"
	@echo ""
	@echo "If this is the latest stable release, then run the following to push 'latest' images."
	@echo ""
	@echo "  make VERSION=$(VERSION) release-publish-latest"
	@echo ""

# WARNING: Only run this target if this release is the latest stable release. Do NOT
# run this target for alpha / beta / release candidate builds, or patches to earlier Calico versions.
## Pushes `latest` release images. WARNING: Only run this for latest stable releases.
release-publish-latest: release-verify
	$(MAKE) push-all push-manifests push-non-manifests RELEASE=true IMAGETAG=latest

.PHONY: node-test-at
# Run docker-image acceptance tests
node-test-at: release-prereqs
	docker run -v $(PWD)/tests/at/calico_node_goss.yaml:/tmp/goss.yaml \
	  $(BUILD_IMAGE):$(VERSION) /bin/sh -c ' \
	   apk --no-cache add wget ca-certificates && \
	   wget -q -O /tmp/goss https://github.com/aelsabbahy/goss/releases/download/v0.3.4/goss-linux-amd64 && \
	   chmod +rx /tmp/goss && \
	   /tmp/goss --gossfile /tmp/goss.yaml validate'

# release-prereqs checks that the environment is configured properly to create a release.
release-prereqs:
ifndef VERSION
	$(error VERSION is undefined - run using make release VERSION=vX.Y.Z)
endif
ifdef LOCAL_BUILD
	$(error LOCAL_BUILD must not be set for a release)
endif

###############################################################################
# Utilities
###############################################################################
.PHONY: help
## Display this help text
help: # Some kind of magic from https://gist.github.com/rcmachado/af3db315e31383502660
	$(info Available targets)
	@awk '/^[a-zA-Z\-\_0-9\/]+:/ {				      \
		nb = sub( /^## /, "", helpMsg );				\
		if(nb == 0) {						   \
			helpMsg = $$0;					      \
			nb = sub( /^[^:]*:.* ## /, "", helpMsg );		   \
		}							       \
		if (nb)							 \
			printf "\033[1;31m%-" width "s\033[0m %s\n", $$1, helpMsg;  \
	}								   \
	{ helpMsg = $$0 }'						  \
	width=20							    \
	$(MAKEFILE_LIST)

$(info "Build dependency versions")
$(info $(shell printf "%-21s = %-10s\n" "BIRD_VERSION" $(BIRD_VERSION)))

$(info "Test dependency versions")
$(info $(shell printf "%-21s = %-10s\n" "CNI_VER" $(CNI_VER)))

$(info "Calico git version")
$(info $(shell printf "%-21s = %-10s\n" "CALICO_GIT_VER" $(CALICO_GIT_VER)))
