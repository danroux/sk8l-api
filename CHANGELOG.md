## v0.9.0 (March 19, 2024)

SECURITY:

* Upgrade `google.golang.org/protobuf` => `v1.33.0` to remove [CWE-835](https://cwe.mitre.org/data/definitions/835.html) / [CVE-2024-24786](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2024-24786) vulnerability. [[GH-14](https://github.com/danroux/sk8l-api/issues/14)]

IMPROVEMENTS:

* Reduce calls to the k8s api by improving how cronjobs are collected [[GH-14](https://github.com/danroux/sk8l-api/issues/14)]

DEPENDENCIES:

* - Update `k8s.io/apimachinery` submodule => `v0.27.12` [[GH-14](https://github.com/danroux/sk8l-api/issues/14)]
* - Update go dependencies:
  - `google.golang.org/protobuf` => `v1.33.0`
  - `k8s.io/api` => `v0.27.12`
  - `k8s.io/apimachinery` => `v0.27.12`
  - `k8s.io/client-go` => `v0.27.12`
* - Remove `github.com/golang/protobuf` [[GH-14](https://github.com/danroux/sk8l-api/issues/14)]

## v0.8.0 (February 29, 2024)

ENHANCEMENTS:

* grafana: Generate annotations.json based on cronjob definitions that can be copy-pasted to create a base Grafana Dashboard. [[GH-12](https://github.com/danroux/sk8l-api/issues/12)]

## v0.7.0 (February 17, 2024)

IMPROVEMENTS:

* chart: Update README && value field [[GH-10](https://github.com/danroux/sk8l-api/issues/10)]

NOTES:

* chart: Release chart v.0.8.0 [[GH-11](https://github.com/danroux/sk8l-api/issues/11)]

## v0.6.0 (February 16, 2024)

ENHANCEMENTS:

* chart: Rename env variables in sk8l-ui-configmap to work with vite [[GH-8](https://github.com/danroux/sk8l-api/issues/8)]

IMPROVEMENTS:

* Mark cronjobs/jobs/pods as failed when containers errored at init because of configuration errors. [[GH-7](https://github.com/danroux/sk8l-api/issues/7)]

## v0.5.0 (February 01, 2024)

SECURITY:

* security: Add pod/container securityContext && networkPolicies [[GH-5](https://github.com/danroux/sk8l-api/issues/5)]

ENHANCEMENTS:

* Docker: Increase go image version 1.21.3->1.21.6 [[GH-5](https://github.com/danroux/sk8l-api/issues/5)]

IMPROVEMENTS:

* chart: Split api/ui deployments && service and overall cleaned up chart files [[GH-5](https://github.com/danroux/sk8l-api/issues/5)]

## 0.4.0 (Dec 3, 2023)

ENHANCEMENT:

* Set up CHANGELOG && .changelog [[GH-2](https://github.com/danroux/sk8l-api/issues/2)]
* Set up release-notes generation on CI [[GH-2](https://github.com/danroux/sk8l-api/issues/2)]
* Set up version check on CI that tests that the new tag version matches the helm appVersion on tag creation [[GH-2](https://github.com/danroux/sk8l-api/issues/2)]
