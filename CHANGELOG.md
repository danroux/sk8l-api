## v0.17.0 (May 19, 2025)

ENHANCEMENTS:

* api: Add JobResponse.WithSidecarContainers [[GH-191](https://github.com/danroux/sk8l-api/issues/191)]
* ci/dependabot: docker:(deps): bump golang from 1.24.2 to 1.24.3 [[GH-193](https://github.com/danroux/sk8l-api/issues/193)]
* ci/dependabot: gha:(deps): bump docker/build-push-action from 6.15.0 to 6.16.0 [[GH-189](https://github.com/danroux/sk8l-api/issues/189)]

IMPROVEMENTS:

* gha/chart: Automate helm chart release with GHA [[GH-192](https://github.com/danroux/sk8l-api/issues/192)]
* go/deps: Replace github.com/ghodss/yaml with sigs.k8s.io/yaml [[GH-196](https://github.com/danroux/sk8l-api/issues/196)]

## v0.16.0 (April 24, 2025)

ENHANCEMENTS:

* ci/dependabot: docker:(deps): bump alpine from 3.20.0 to 3.20.2 [[GH-107](https://github.com/danroux/sk8l-api/issues/107)]
* ci/dependabot: docker:(deps): bump alpine from 3.20.2 to 3.21.3 [[GH-157](https://github.com/danroux/sk8l-api/issues/157)]
* ci/dependabot: docker:(deps): bump golang from 1.22.4 to 1.22.5 [[GH-96](https://github.com/danroux/sk8l-api/issues/96)]
* ci/dependabot: docker:(deps): bump golang from 1.22.5 to 1.24.1 [[GH-156](https://github.com/danroux/sk8l-api/issues/156)]
* ci/dependabot: gha:(deps): bump actions/checkout from 4.1.7 to 4.2.2 [[GH-151](https://github.com/danroux/sk8l-api/issues/151)]
* ci/dependabot: gha:(deps): bump actions/setup-go from 5.0.1 to 5.0.2 [[GH-98](https://github.com/danroux/sk8l-api/issues/98)]
* ci/dependabot: gha:(deps): bump actions/setup-go from 5.0.2 to 5.4.0 [[GH-165](https://github.com/danroux/sk8l-api/issues/165)]
* ci/dependabot: gha:(deps): bump actions/upload-artifact from 4.3.3 to 4.3.4 [[GH-97](https://github.com/danroux/sk8l-api/issues/97)]
* ci/dependabot: gha:(deps): bump actions/upload-artifact from 4.3.4 to 4.6.2 [[GH-164](https://github.com/danroux/sk8l-api/issues/164)]
* ci/dependabot: gha:(deps): bump azure/setup-helm from 4.1.0 to 4.3.0 [[GH-159](https://github.com/danroux/sk8l-api/issues/159)]
* ci/dependabot: gha:(deps): bump docker/build-push-action from 6.0.2 to 6.5.0 [[GH-105](https://github.com/danroux/sk8l-api/issues/105)]
* ci/dependabot: gha:(deps): bump docker/build-push-action from 6.5.0 to 6.15.0 [[GH-160](https://github.com/danroux/sk8l-api/issues/160)]
* ci/dependabot: gha:(deps): bump docker/login-action from 3.2.0 to 3.3.0 [[GH-104](https://github.com/danroux/sk8l-api/issues/104)]
* ci/dependabot: gha:(deps): bump docker/login-action from 3.3.0 to 3.4.0 [[GH-158](https://github.com/danroux/sk8l-api/issues/158)]
* ci/dependabot: gha:(deps): bump docker/setup-buildx-action from 3.3.0 to 3.5.0 [[GH-106](https://github.com/danroux/sk8l-api/issues/106)]
* ci/dependabot: gha:(deps): bump docker/setup-buildx-action from 3.5.0 to 3.10.0 [[GH-163](https://github.com/danroux/sk8l-api/issues/163)]
* ci/dependabot: gha:(deps): bump engineerd/setup-kind from 0.5.0 to 0.6.2 [[GH-162](https://github.com/danroux/sk8l-api/issues/162)]
* ci/dependabot: go:(deps): bump github.com/dgraph-io/badger/v4 from 4.2.0 to 4.7.0 [[GH-180](https://github.com/danroux/sk8l-api/issues/180)]
* ci/dependabot: go:(deps): bump github.com/prometheus/client_golang from 1.19.1 to 1.22.0 [[GH-171](https://github.com/danroux/sk8l-api/issues/171)]
* ci/dependabot: go:(deps): bump google.golang.org/grpc from 1.64.0 to 1.72.0 [[GH-181](https://github.com/danroux/sk8l-api/issues/181)]
* ci/dependabot: go:(deps): bump google.golang.org/protobuf from 1.34.1 to 1.36.6 [[GH-169](https://github.com/danroux/sk8l-api/issues/169)]
* ci/dependabot: go:(deps): bump k8s.io/client-go from 0.30.1 to 0.32.3 [[GH-170](https://github.com/danroux/sk8l-api/issues/170)]
* ci/k8s: Increase kind version, update supported versions and test against them.
* Add support && tests for k8s v1.30.4 && v1.31.0
  * Removed tests && end our support for no longer supported k8s versions
    - v1.26.x
    - v1.27.x
    - v1.28.x
[[GH-176](https://github.com/danroux/sk8l-api/issues/176)]
* go/deps: go:(deps): bump k8s.io/api from 0.30.1 to 0.30.3 [[GH-177](https://github.com/danroux/sk8l-api/issues/177)]
* go: Update go version && toolchain to 1.23.8 [[GH-179](https://github.com/danroux/sk8l-api/issues/179)]
* go:(deps): Update k8s.io packages && k8s.io/apimachinery to v0.30.11

- k8s.io/api
- k8s.io/apimachinery
- k8s.io/client-go [[GH-178](https://github.com/danroux/sk8l-api/issues/178)]
* k8s: Add support for k8s v1.32.x [[GH-182](https://github.com/danroux/sk8l-api/issues/182)]

IMPROVEMENTS:

* ci: Fix golangci-lint configuration, increase golangci version to 1.64.8. Rename deprecated linters and address issues raised now that the linters work again. [[GH-175](https://github.com/danroux/sk8l-api/issues/175)]
* gha/docker: No longer build/push/pull dev-pr version to/from Docker hub, instead export it in the OCI image layout format and load it into kind directly. [[GH-184](https://github.com/danroux/sk8l-api/issues/184)]
* gha: Improve grpcurl dependency caching [[GH-183](https://github.com/danroux/sk8l-api/issues/183)]

## v0.15.0 (June 21, 2024)

ENHANCEMENTS:

* Docker: Build Docker multi-platform images: linux/amd64,linux/arm{64,v7}
* ci/dependabot: docker:(deps): bump alpine from 3.19.1 to 3.20.0 [[GH-75](https://github.com/danroux/sk8l-api/issues/75)]
* ci/dependabot: docker:(deps): bump golang from 1.22.3 to 1.22.4 [[GH-77](https://github.com/danroux/sk8l-api/issues/77)]
* ci/dependabot: gha:(deps): bump actions/checkout from 4.1.4 to 4.1.6 [[GH-74](https://github.com/danroux/sk8l-api/issues/74)]
* ci/dependabot: gha:(deps): bump actions/checkout from 4.1.6 to 4.1.7 [[GH-81](https://github.com/danroux/sk8l-api/issues/81)]
* ci/dependabot: gha:(deps): bump docker/build-push-action from 5.3.0 to 6.0.2 [[GH-87](https://github.com/danroux/sk8l-api/issues/87)]
* ci/dependabot: gha:(deps): bump docker/login-action from 3.1.0 to 3.2.0 [[GH-76](https://github.com/danroux/sk8l-api/issues/76)]
* ci/dependabot: gha:(deps): bump golangci/golangci-lint-action from 6.0.0 to 6.0.1 [[GH-66](https://github.com/danroux/sk8l-api/issues/66)]

## v0.14.0 (May 16, 2024)

ENHANCEMENTS:

* ci/dependabot: docker:(deps): bump golang from 1.22.2 to 1.22.3 [[GH-65](https://github.com/danroux/sk8l-api/issues/65)]
* ci/dependabot: gha:(deps): bump golangci/golangci-lint-action from 5.1.0 to 6.0.0 [[GH-63](https://github.com/danroux/sk8l-api/issues/63)]
* ci/dependabot: go:(deps): bump github.com/prometheus/client_golang from 1.19.0 to 1.19.1 [[GH-67](https://github.com/danroux/sk8l-api/issues/67)]
* ci/dependabot: go:(deps): bump google.golang.org/grpc from 1.63.2 to 1.64.0 [[GH-72](https://github.com/danroux/sk8l-api/issues/72)]
* ci/dependabot: go:(deps): bump google.golang.org/protobuf from 1.34.0 to 1.34.1 [[GH-61](https://github.com/danroux/sk8l-api/issues/61)]
* ci/dependabot: go:(deps): bump k8s.io/api from 0.30.0 to 0.30.1 [[GH-70](https://github.com/danroux/sk8l-api/issues/70)]
* ci/dependabot: go:(deps): bump k8s.io/apimachinery from 0.30.0 to 0.30.1 [[GH-68](https://github.com/danroux/sk8l-api/issues/68)]
* ci/dependabot: go:(deps): bump k8s.io/client-go from 0.30.0 to 0.30.1 [[GH-71](https://github.com/danroux/sk8l-api/issues/71)]

DEPENDENCIES:

* Update `k8s.io/apimachinery` submodule => `v0.30.1` [[GH-72](https://github.com/danroux/sk8l-api/issues/72)]

## v0.13.0 (May 07, 2024)

ENHANCEMENTS:

* ci/dependabot: gha:(deps): bump actions/setup-go from 5.0.0 to 5.0.1 [[GH-59](https://github.com/danroux/sk8l-api/issues/59)]
* Update envoyproxy/envoy image to v1.30-latest

## v0.12.0 (May 02, 2024)

ENHANCEMENTS:

* api: Add sk8l.Cronjob/GetJobs to list jobs in the namespacee [[GH-52](https://github.com/danroux/sk8l-api/issues/52)]
* ci/dependabot: gha deps:(deps): bump actions/checkout from 4.1.2 to 4.1.4 [[GH-50](https://github.com/danroux/sk8l-api/issues/50)]
* ci/dependabot: gha:(deps): bump golangci/golangci-lint-action from 4.0.0 to 5.1.0 [[GH-57](https://github.com/danroux/sk8l-api/issues/57)]
* ci/dependabot: go deps:(deps): bump google.golang.org/protobuf from 1.33.0 to 1.34.0

go deps:(deps): bump google.golang.org/protobuf from 1.33.0 to 1.34.0 [[GH-56](https://github.com/danroux/sk8l-api/issues/56)]
* ci/dependabot: go deps:(deps): bump k8s.io/client-go from 0.29.3 to 0.30.0 [[GH-55](https://github.com/danroux/sk8l-api/issues/55)]

## v0.11.0 (April 24, 2024)

ENHANCEMENTS:

* ci/dependabot: gha deps:(deps): bump actions/upload-artifact from 4.3.1 to 4.3.3 [[GH-47](https://github.com/danroux/sk8l-api/issues/47)]
* docker/gha: Publish sk8l-api image to ghcr.io [[GH-46](https://github.com/danroux/sk8l-api/issues/46)]

IMPROVEMENTS:

* chart: Update README && Certificate secrets configuration [[GH-48](https://github.com/danroux/sk8l-api/issues/48)]
* gha/ci: Extend smoke tests [[GH-35](https://github.com/danroux/sk8l-api/issues/35)]

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
