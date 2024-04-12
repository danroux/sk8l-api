## v0.10.0 (April 12, 2024)

IMPROVEMENTS:

* ci/k8s: Build and test againts matching apimachinery protos during testing on CI [[GH-33](https://github.com/danroux/sk8l-api/issues/33)]
* go: Improved go and envoy tls{MinVersion, MaxVersion} [[GH-33](https://github.com/danroux/sk8l-api/issues/33)]

BUG FIXES:

* Chart: [Remove a duplicate runAsNonRoot from the UI Deployment #17](https://github.com/danroux/sk8l-api/pull/17) by dbirks [[GH-18](https://github.com/danroux/sk8l-api/issues/18)]
* go: Fixed a bug on DashboardAnnotations when annotations.tmpl was missing [[GH-33](https://github.com/danroux/sk8l-api/issues/33)]

DEPENDENCIES:

* ci/dependabot: Configure dependabot version updates [[GH-20](https://github.com/danroux/sk8l-api/issues/20)]
* ci/dependabot: docker deps:(deps): bump alpine from 3.18.3 to 3.19.1 [[GH-22](https://github.com/danroux/sk8l-api/issues/22)]
* ci/dependabot: docker deps:(deps): bump golang from 1.22.0 to 1.22.2 [[GH-21](https://github.com/danroux/sk8l-api/issues/21)]
* ci/dependabot: gha deps:(deps): bump actions/checkout from 4.1.1 to 4.1.2 [[GH-30](https://github.com/danroux/sk8l-api/issues/30)]
* ci/dependabot: go deps:(deps): bump github.com/prometheus/client_golang from 1.17.0 to 1.19.0 [[GH-26](https://github.com/danroux/sk8l-api/issues/26)]
* gha/dependabot: Create .changelog entry on dependabot PRs [[GH-27](https://github.com/danroux/sk8l-api/issues/27)]
* gha/k8s: Build && push Docker dev image for testing on CI [[GH-29](https://github.com/danroux/sk8l-api/issues/29)]
* gha/k8s: Setup K8s pipeline in GHA [[GH-28](https://github.com/danroux/sk8l-api/issues/28)]
* golang: Setup checks related to golang in gha/ci [[GH-19](https://github.com/danroux/sk8l-api/issues/19)]
* deps/go: Update go dependencies: grpc 1.59.0 => 1.63.2 && client-go, apimachinery 0.27.12 => 0.29.3 [[GH-32](https://github.com/danroux/sk8l-api/issues/32)]

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
