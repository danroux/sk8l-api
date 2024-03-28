# https://gist.github.com/mpneuried/0594963ad38e68917ef189b4e6a269db

# HELP
# This will output the help for each task
# thanks to https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
.PHONY: help

help: ## This help.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Bump these on release
VERSION_MAJOR ?= 0
VERSION_MINOR ?= 9
VERSION_PATCH ?= 0

CHART_VERSION_MAJOR ?= 0
CHART_VERSION_MINOR ?= 10
CHART_VERSION_PATCH ?= 1

WITHOUT ?= $(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_PATCH)
VERSION ?= v$(WITHOUT)
CHART_VERSION_WITHOUT ?= $(CHART_VERSION_MAJOR).$(CHART_VERSION_MINOR).$(CHART_VERSION_PATCH)
CHART_VERSION ?= v$(CHART_VERSION_WITHOUT)
VERSION_PACKAGE = main

GO_LDFLAGS := '
GO_LDFLAGS += -X $(VERSION_PACKAGE).version=$(VERSION)
GO_LDFLAGS += -w -s # Drop debugging symbols.
GO_LDFLAGS += '

go-out:
	CGO_ENABLED=0 GOEXPERIMENT=loopvar GOOS=linux go build -ldflags $(GO_LDFLAGS) -o ./sk8l .

version:
	@echo $(VERSION)

