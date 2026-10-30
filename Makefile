# https://gist.github.com/mpneuried/0594963ad38e68917ef189b4e6a269db

# HELP
# This will output the help for each task
# thanks to https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
.PHONY: help

help: ## This help.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Bump these on release
APP_VERSION ?= 0.25.0

API_VERSION_MAJOR ?= 0
API_VERSION_MINOR ?= 22
API_VERSION_PATCH ?= 0

CHART_VERSION_MAJOR ?= 0
CHART_VERSION_MINOR ?= 26
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

CHART_VER_WO ?= $(if $(strip $(GITHUB_CHART_VERSION)),$(GITHUB_CHART_VERSION),$(CHART_VERSION_WITHOUT))

chart-version-wo:
	@echo $(CHART_VER_WO)


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
	kubectl rollout status deployment/trust-manager -n cert-manager --timeout=120s
	kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=trust-manager -n cert-manager --timeout=120s
	for i in $$(seq 1 10); do kubectl apply -f testdata/sk8l-trust.yml && break || sleep 3; done

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
	./ci/collect_workload_info.sh sk8l > expected_output.txt
	@echo "Waiting for cronjob executions and metrics to become available..."
	@passed=0; \
	for i in $$(seq 1 45); do \
	  curl -k -s https://localhost:8590/metrics > current_state.txt 2>/dev/null || true; \
	  if ./ci/check_strings_in_file.sh > job_output.txt 2>&1; then \
	    echo "Smoke tests passed in $${i} polling attempt(s)."; \
	    passed=1; \
	    break; \
	  fi; \
	  sleep 2; \
	done; \
	if [ $$passed -ne 1 ]; then \
	  echo "Error: Smoke tests timed out waiting for metrics."; \
	  ./ci/check_strings_in_file.sh; \
	  exit 1; \
	fi

api-smoke-tests: # api-smoke-tests
	./ci/api_smoke_tests.sh

GITHUB_PR_IMAGE_TAG ?=''
update-config-files: # update-config-files
	./ci/update_config_files.sh $(GITHUB_PR_IMAGE_TAG)


helm-push:
	helm push charts/repo/sk8l-$(CHART_VER_WO).tgz oci://ghcr.io/danroux/sk8l

UI_VERSION ?= v0.19.0

create-tag: ## Create release git tag
	git tag -a $(API_VERSION) -m $(API_VERSION)

bump-chart-version: ## Bump Helm chart version in Chart.yaml
	@echo 'Bump chart version to $(CHART_VERSION_WITHOUT)'
	yq -i '.version = "$(CHART_VERSION_WITHOUT)"' charts/sk8l/Chart.yaml

bump-chart-appVersion: ## Bump Helm chart appVersion in Chart.yaml
	@echo 'Bump chart appVersion to $(APP_VERSION)'
	yq -i '.appVersion = "$(APP_VERSION)"' charts/sk8l/Chart.yaml

bump-image-versions: ## Bump image tags in deployment templates and values.yaml
	sed -i.bak -E 's/.Values.sk8lUi.imageTag \| default "[^"]+"/.Values.sk8lUi.imageTag | default "$(UI_VERSION)"/g' charts/sk8l/templates/ui-deployment.yaml && rm -f charts/sk8l/templates/ui-deployment.yaml.bak
	sed -i.bak -E 's/.Values.sk8lApi.imageTag \| default "[^"]+"/.Values.sk8lApi.imageTag | default "$(API_VERSION)"/g' charts/sk8l/templates/deployment.yaml && rm -f charts/sk8l/templates/deployment.yaml.bak
	yq -i '.sk8lApi.imageTag = "$(API_VERSION)"' charts/sk8l/values.yaml
	yq -i '.sk8lUi.imageTag = "$(UI_VERSION)"' charts/sk8l/values.yaml
	sed -i.bak -E 's#(image: danroux/sk8l-api:)[^ \t\r\n]+#\1$(API_VERSION)#g' charts/sk8l/Chart.yaml && rm -f charts/sk8l/Chart.yaml.bak
	sed -i.bak -E 's#(image: danroux/sk8l-ui:)[^ \t\r\n]+#\1$(UI_VERSION)#g' charts/sk8l/Chart.yaml && rm -f charts/sk8l/Chart.yaml.bak

bump-version: ## Bump all version strings across chart and templates
	@echo 'Bumping all version references...'
	$(MAKE) bump-image-versions
	$(MAKE) bump-chart-version
	$(MAKE) bump-chart-appVersion

update-chart-changes: ## Extract latest CHANGELOG section into Chart.yaml artifacthub.io/changes
	./ci/parse_changelog.sh CHANGELOG.md charts/sk8l/Chart.yaml

LAST_RELEASE_TAG ?= $(shell git describe --tags $$(git rev-list --tags --max-count=1) 2>/dev/null || echo "v0.0.0")
THIS_RELEASE_TAG ?= HEAD

changelog: ## Generate release changelog from .changelog/ entries
	@echo "Generating changelog for $(THIS_RELEASE_TAG) from $(LAST_RELEASE_TAG)..."
	@changelog-build -last-release $(LAST_RELEASE_TAG) \
		-entries-dir .changelog/ \
		-changelog-template .changelog/changelog.tmpl \
		-note-template .changelog/note.tmpl \
		-this-release $(THIS_RELEASE_TAG) | ./ci/clean_changelog.sh

prepare-release: ## Prepare local release branch with bumped versions and changelog
	git checkout -b release-$(API_VERSION)
	$(MAKE) bump-version
	cp dashboards/sk8l-overview.json charts/sk8l/dashboards/sk8l-overview.json
	$(MAKE) update-chart-changes

print-release-msg: ## Print release commit message format
	@echo "Release $(API_VERSION) - Chart.version $(CHART_VERSION)"

current-versions: ## Display current configured versions
	@echo "API Version:   $(API_VERSION)"
	@echo "Chart Version: $(CHART_VERSION)"
	@echo "App Version:   v$(APP_VERSION)"
	@echo "UI Version:    $(UI_VERSION)"

sk8lGhcrImages:
	yq -i '.sk8lApi.image = "ghcr.io/danroux/sk8l-api"' charts/sk8l/values-local.yaml
	yq -i '.sk8lApi.imageTag = "pre"' charts/sk8l/values-local.yaml
	yq -i '.sk8lUi.image = "ghcr.io/danroux/sk8l-ui"' charts/sk8l/values-local.yaml
	yq -i '.sk8lUi.imageTag = "dev"' charts/sk8l/values-local.yaml

sk8lLocalImages:
	yq -i '.sk8lApi.image = "localhost:5001/sk8l-api"' charts/sk8l/values-local.yaml \
	&& yq -i '.sk8lUi.image = "localhost:5001/sk8l-ui"' charts/sk8l/values-local.yaml

sk8lImages:
	yq -i '.sk8lApi.image = "danroux/sk8l-api"' charts/sk8l/values-local.yaml \
	&& yq -i '.sk8lUi.image = "danroux/sk8l-ui"' charts/sk8l/values-local.yaml
	yq -i '.sk8lApi.imageTag = "$(API_VERSION)"' charts/sk8l/values-local.yaml
	yq -i ".sk8lUi.imageTag = \"$(UI_VERSION)\"" charts/sk8l/values-local.yaml
