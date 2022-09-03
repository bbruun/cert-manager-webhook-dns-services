OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)

IMAGE_NAME := "dns-services-webhook"
IMAGE_TAG := "latest"
REGISTRY := docker.io

OUT := $(shell pwd)/_out

KUBE_VERSION=1.23.1

ifeq ($(shell test -e /usr/bin/podman) && echo -n yes),yes)
  PODMAN = 1
else
  PODMAN = 0
endif

$(shell mkdir -p "$(OUT)")
export TEST_ASSET_ETCD=_test/kubebuilder/bin/etcd
export TEST_ASSET_KUBE_APISERVER=_test/kubebuilder/bin/kube-apiserver
export TEST_ASSET_KUBECTL=_test/kubebuilder/bin/kubectl

test: _test/kubebuilder
	go test -v .

_test/kubebuilder:
	curl -fsSL https://go.kubebuilder.io/test-tools/$(KUBE_VERSION)/$(OS)/$(ARCH) -o kubebuilder-tools.tar.gz
	mkdir -p _test/kubebuilder
	tar -xvf kubebuilder-tools.tar.gz
	mv kubebuilder/bin _test/kubebuilder/
	rm kubebuilder-tools.tar.gz
	rm -R kubebuilder

clean: clean-kubebuilder

clean-kubebuilder:
	rm -Rf _test/kubebuilder

build:
ifeq (/usr/bin/podman,/usr/bin/podman)
	echo "Using podman to build container image"
	/usr/bin/podman build . -f Dockerfile -t "$(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)"
else
	echo "Using Docker to build container image"
	/usr/bin/docker build . -t "$(IMAGE_NAME):$(IMAGE_TAG)"
endif

push:    
ifeq (/usr/bin/podman,/usr/bin/podman)
	/usr/bin/podman push "$(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)"
else
	/usr/bin/docker push "$(IMAGE_NAME):$(IMAGE_TAG)"
endif

.PHONY: rendered-manifest.yaml
rendered-manifest.yaml:
	helm template \
	    --name cert-manager-webhook-dns-services \
        --set image.repository=$(IMAGE_NAME) \
        --set image.tag=$(IMAGE_TAG) \
        deploy/example-webhook > "$(OUT)/rendered-manifest.yaml"
