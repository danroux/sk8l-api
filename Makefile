# https://gist.github.com/mpneuried/0594963ad38e68917ef189b4e6a269db

# HELP
# This will output the help for each task
# thanks to https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
.PHONY: help

help: ## This help.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Bump these on release
APP_VERSION ?= 0.20.0

API_VERSION_MAJOR ?= 0
API_VERSION_MINOR ?= 17
API_VERSION_PATCH ?= 0

CHART_VERSION_MAJOR ?= 0
CHART_VERSION_MINOR ?= 21
CHART_VERSION_PATCH ?= 0

API_WITHOUT ?= $(API_VERSION_MAJOR).$(API_VERSION_MINOR).$(API_VERSION_PATCH)
API_VERSION ?= v$(API_WITHOUT)
CHART_VERSION_WITHOUT ?= $(CHART_VERSION_MAJOR).$(CHART_VERSION_MINOR).$(CHART_VERSION_PATCH)
CHART_VERSION ?= v$(CHART_VERSION_WITHOUT)
VERSION_PACKAGE = main

GO_LDFLAGS := '
GO_LDFLAGS += -X $(VERSION_PACKAGE).version=$(API_VERSION)
GO_LDFLAGS += -w -s # Drop debugging symbols.
GO_LDFLAGS += '

go-out:
	CGO_ENABLED=0 GOEXPERIMENT=loopvar GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags $(GO_LDFLAGS) -o ./sk8l .

version:
	@echo $(API_VERSION)

chart-version:
	@echo $(CHART_VERSION)

chart-version-wo:
	@echo $(if $(strip $(GITHUB_CHART_VERSION)),$(GITHUB_CHART_VERSION),$(CHART_VERSION_WITHOUT))


.PHONY: package-app-ci
package-app-ci: ## Package helm chart and upload
	helm package charts/sk8l \
        && mv sk8l*tgz charts/repo \
        && helm repo index charts/repo --url https://sk8l.io/charts


setup-certs: # setup-certs
	helm repo add jetstack https://charts.jetstack.io --force-update
	kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.4/cert-manager.crds.yaml
	helm upgrade --install \
	  cert-manager jetstack/cert-manager \
	  --namespace cert-manager \
	  --create-namespace=true \
	  --version v1.14.4 \
	  --set prometheus.enabled=false \
	  --set extraArgs={--feature-gates=AdditionalCertificateOutputFormats=true} \
	  --set webhook.extraArgs={--feature-gates=AdditionalCertificateOutputFormats=true} \
	  --set webhook.timeoutSeconds=4
	kubectl apply -f testdata/sk8l-cert-manager.yml
	helm upgrade --install trust-manager jetstack/trust-manager \
	  --install \
	  --namespace cert-manager \
	  --set app.trust.namespace=sk8l \
	  --wait
	kubectl apply -f testdata/sk8l-trust.yml

install-chart-ci: # install-chart-ci
	helm repo add sk8l https://sk8l.io/charts
	helm upgrade --install sk8l -f testdata/sk8l-values.yml --namespace sk8l \
	  --create-namespace=true \
	  --set namespace.create=false \
	  --set namespace.name=sk8l \
	  charts/sk8l

metrics-smoke-tests: # metrics-smoke-tests
	kubectl apply -f testdata/sk8l-cronjobs.yml -n sk8l > /dev/null
	kubectl apply -f testdata/sk8l-demo-job.yml -n sk8l > /dev/null
	kubectl wait -n sk8l --for=condition=ready pod -l app.kubernetes.io/pod=sk8l-ui --timeout=300s
	sleep 60
	./ci/collect_workload_info.sh sk8l > expected_output.txt
	sleep 18
	curl -k https://localhost:8590/metrics > current_state.txt
	./ci/check_strings_in_file.sh > job_output.txt

api-smoke-tests: # api-smoke-tests
	./ci/api_smoke_tests.sh

GITHUB_PR_IMAGE_TAG ?=''
update-config-files: # update-config-files
	./ci/update_config_files.sh $(GITHUB_PR_IMAGE_TAG)


helm-push:
	helm push charts/repo/sk8l-$(shell make chart-version-wo).tgz oci://ghcr.io/danroux/sk8l




