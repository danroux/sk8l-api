## v0.22.0 (October 23, 2026)


FEATURES:

* go: Implement client-go informers store with indexed lookups, SetTransform memory stripping, and coalesced change notifications [[GH-325](https://github.com/danroux/sk8l-api/issues/325)]
* go: Switch storage backend from Badger to in-memory client-go informers and remove legacy collectors [[GH-326](https://github.com/danroux/sk8l-api/issues/326)]

ENHANCEMENTS:

* helm: Add RBAC clusterWide toggle, default to namespaced Role/RoleBinding, and dynamic namespace resolution [[GH-330](https://github.com/danroux/sk8l-api/issues/330)]

IMPROVEMENTS:

* docker: Optimize BuildKit cache mounts and exclude metadata from build context [[GH-329](https://github.com/danroux/sk8l-api/issues/329)]
* go: Abstract store operations behind Store interface and decouple gRPC streaming from Badger pub/sub [[GH-324](https://github.com/danroux/sk8l-api/issues/324)]
* go: Remove Badger dependency, unused logger adapter and chart storage volume; enforce `mnd` linter on all files [[GH-328](https://github.com/danroux/sk8l-api/issues/328)]

DEPENDENCIES:

* ci/dependabot: docker:(deps): bump alpine from 3.24.2 to 3.24.3 [[GH-327](https://github.com/danroux/sk8l-api/issues/327)]

## v0.21.0 (October 21, 2026)

BREAKING CHANGES:

* go: Standardize Prometheus metrics to OpenMetrics format with static metric names and dimensional labels, replacing legacy dynamic metric names [[GH-318](https://github.com/danroux/sk8l-api/issues/318)]

FEATURES:

* helm: Auto-provision Grafana overview dashboard via ConfigMap with discovery labels [[GH-321](https://github.com/danroux/sk8l-api/issues/321)]

IMPROVEMENTS:

* go: embed Grafana dashboard JSON and remove legacy template generator [[GH-320](https://github.com/danroux/sk8l-api/issues/320)]

DEPENDENCIES:

* ci/dependabot: docker:(deps): bump golang from 1.27.1 to 1.27.2 [[GH-319](https://github.com/danroux/sk8l-api/issues/319)]

## v0.20.0 (October 16, 2026)

FEATURES:

* go: Add runtime-configurable log level via LOG_LEVEL environment variable [[GH-298](https://github.com/danroux/sk8l-api/issues/298)]

ENHANCEMENTS:

* ci/dependabot: docker:(deps): bump alpine from 3.24.1 to 3.24.2 [[GH-305](https://github.com/danroux/sk8l-api/issues/305)]
* ci/dependabot: docker:(deps): bump golang from 1.26.4 to 1.26.5 [[GH-290](https://github.com/danroux/sk8l-api/issues/290)]
* ci/dependabot: docker:(deps): bump golang from 1.26.5 to 1.27.1 [[GH-303](https://github.com/danroux/sk8l-api/issues/303)]
* ci/dependabot: gha:(deps): bump helm/kind-action from 1.12.0 to 1.14.0 [[GH-289](https://github.com/danroux/sk8l-api/issues/289)]
* ci/dependabot: gha:(deps): bump helm/kind-action from 1.14.0 to 1.15.0 [[GH-309](https://github.com/danroux/sk8l-api/issues/309)]
* ci/dependabot: go:(deps): bump github.com/dgraph-io/badger/v4 from 4.9.5 to 4.9.6 [[GH-291](https://github.com/danroux/sk8l-api/issues/291)]
* ci/dependabot: go:(deps): bump google.golang.org/grpc from 1.83.0 to 1.83.1 [[GH-293](https://github.com/danroux/sk8l-api/issues/293)]
* ci/dependabot: go:(deps): bump google.golang.org/grpc from 1.83.1 to 1.83.2 [[GH-301](https://github.com/danroux/sk8l-api/issues/301)]
* ci/dependabot: go:(deps): bump google.golang.org/grpc from 1.83.2 to 1.84.0 [[GH-304](https://github.com/danroux/sk8l-api/issues/304)]
* ci/dependabot: go:(deps): bump google.golang.org/protobuf from 1.36.12-0.20260120151049-f2248ac996af to 1.36.12 [[GH-292](https://github.com/danroux/sk8l-api/issues/292)]
* ci/dependabot: go:(deps): bump k8s.io/client-go from 0.36.3 to 0.36.4 [[GH-296](https://github.com/danroux/sk8l-api/issues/296)]
* ci/dependabot: go:(deps): bump k8s.io/client-go from 0.36.4 to 0.37.0 [[GH-302](https://github.com/danroux/sk8l-api/issues/302)]
* ci: Add on-demand preview environment workflow deploying ephemeral namespaces via ArgoCD [[GH-314](https://github.com/danroux/sk8l-api/issues/314)]

IMPROVEMENTS:

* ci/k8s: Upgrade KinD to v0.33.0 and update test matrix to Kubernetes v1.34–v1.37 (dropping EOL v1.31–v1.33) [[GH-307](https://github.com/danroux/sk8l-api/issues/307)]
* ci: Streamline changelog checker workflow into single job and remove skip-ci [[GH-316](https://github.com/danroux/sk8l-api/issues/316)]
* go: Implement real gRPC health checks backed by Badger DB liveness probe and fix Watch streaming to conform to the gRPC Health Checking Protocol specification [[GH-299](https://github.com/danroux/sk8l-api/issues/299)]
* go: Replace time.Sleep polling in GetCronjobs, GetCronjob, GetCronjobPods, and GetJobs with event-driven gRPC streaming via Badger db.Subscribe, eliminating redundant sends, update latency, and context cancellation blindness [[GH-308](https://github.com/danroux/sk8l-api/issues/308)]

DEPENDENCIES:

* ci/dependabot: go:(deps): bump github.com/prometheus/client_golang from 1.24.1 to 1.25.0 [[GH-315](https://github.com/danroux/sk8l-api/issues/315)]
* ci/dependabot: go:(deps): bump k8s.io/client-go from 0.37.0 to 0.37.1 [[GH-311](https://github.com/danroux/sk8l-api/issues/311)]

## v0.19.0 (August 26, 2026)

ENHANCEMENTS:

* ci/dependabot: docker:(deps): bump alpine from 3.22.1 to 3.22.2 [[GH-230](https://github.com/danroux/sk8l-api/issues/230)]
* ci/dependabot: docker:(deps): bump alpine from 3.22.2 to 3.24.1 [[GH-270](https://github.com/danroux/sk8l-api/issues/270)]
* ci/dependabot: docker:(deps): bump golang from 1.25.0 to 1.25.1 [[GH-226](https://github.com/danroux/sk8l-api/issues/226)]
* ci/dependabot: docker:(deps): bump golang from 1.26.0 to 1.26.4 [[GH-271](https://github.com/danroux/sk8l-api/issues/271)]
* ci/dependabot: gha:(deps): bump actions/cache from 4 to 5 [[GH-261](https://github.com/danroux/sk8l-api/issues/261)]
* ci/dependabot: gha:(deps): bump actions/checkout from 5.0.0 to 5.0.1 [[GH-233](https://github.com/danroux/sk8l-api/issues/233)]
* ci/dependabot: gha:(deps): bump actions/checkout from 6.0.2 to 6.0.3 [[GH-264](https://github.com/danroux/sk8l-api/issues/264)]
* ci/dependabot: gha:(deps): bump actions/setup-go from 5.5.0 to 6.0.0 [[GH-224](https://github.com/danroux/sk8l-api/issues/224)]
* ci/dependabot: gha:(deps): bump actions/setup-go from 6.0.0 to 6.4.0 [[GH-265](https://github.com/danroux/sk8l-api/issues/265)]
* ci/dependabot: gha:(deps): bump actions/upload-artifact from 4.6.2 to 5.0.0 [[GH-231](https://github.com/danroux/sk8l-api/issues/231)]
* ci/dependabot: gha:(deps): bump azure/setup-helm from 4.3.1 to 5.0.0 [[GH-263](https://github.com/danroux/sk8l-api/issues/263)]
* ci/dependabot: gha:(deps): bump docker/build-push-action from 6.18.0 to 7.2.0 [[GH-266](https://github.com/danroux/sk8l-api/issues/266)]
* ci/dependabot: gha:(deps): bump docker/login-action from 3.5.0 to 3.7.0 [[GH-229](https://github.com/danroux/sk8l-api/issues/229)]
* ci/dependabot: gha:(deps): bump docker/login-action from 3.7.0 to 4.2.0 [[GH-262](https://github.com/danroux/sk8l-api/issues/262)]
* ci/dependabot: gha:(deps): bump docker/setup-buildx-action from 3.11.1 to 4.1.0 [[GH-268](https://github.com/danroux/sk8l-api/issues/268)]
* ci/dependabot: gha:(deps): bump golangci/golangci-lint-action from 8.0.0 to 9.0.0 [[GH-232](https://github.com/danroux/sk8l-api/issues/232)]
* ci/dependabot: gha:(deps): bump golangci/golangci-lint-action from 9.2.0 to 9.2.1 [[GH-269](https://github.com/danroux/sk8l-api/issues/269)]
* ci/dependabot: gha:(deps): bump reproducible-containers/buildkit-cache-dance from 3.2.0 to 3.3.0 [[GH-209](https://github.com/danroux/sk8l-api/issues/209)]
* ci/dependabot: gha:(deps): bump reproducible-containers/buildkit-cache-dance from 3.3.0 to 3.4.0 [[GH-267](https://github.com/danroux/sk8l-api/issues/267)]
* ci/dependabot: go:(deps): bump github.com/dgraph-io/badger/v4 from 4.8.0 to 4.9.1 [[GH-235](https://github.com/danroux/sk8l-api/issues/235)]
* ci/dependabot: go:(deps): bump github.com/dgraph-io/badger/v4 from 4.9.1 to 4.9.2 [[GH-260](https://github.com/danroux/sk8l-api/issues/260)]
* ci/dependabot: go:(deps): bump github.com/dgraph-io/badger/v4 from 4.9.2 to 4.9.5 [[GH-275](https://github.com/danroux/sk8l-api/issues/275)]
* ci/dependabot: go:(deps): bump github.com/prometheus/client_golang from 1.22.0 to 1.23.0 [[GH-215](https://github.com/danroux/sk8l-api/issues/215)]
* ci/dependabot: go:(deps): bump github.com/prometheus/client_golang from 1.23.0 to 1.23.2 [[GH-225](https://github.com/danroux/sk8l-api/issues/225)]
* ci/dependabot: go:(deps): bump github.com/prometheus/client_golang from 1.23.2 to 1.24.1 [[GH-278](https://github.com/danroux/sk8l-api/issues/278)]
* ci/dependabot: go:(deps): bump github.com/rs/zerolog from 1.34.0 to 1.35.0 [[GH-249](https://github.com/danroux/sk8l-api/issues/249)]
* ci/dependabot: go:(deps): bump github.com/rs/zerolog from 1.35.0 to 1.35.1 [[GH-254](https://github.com/danroux/sk8l-api/issues/254)]
* ci/dependabot: go:(deps): bump google.golang.org/grpc from 1.72.2 to 1.75.0 [[GH-206](https://github.com/danroux/sk8l-api/issues/206)]
* ci/dependabot: go:(deps): bump google.golang.org/grpc from 1.75.0 to 1.79.1 [[GH-228](https://github.com/danroux/sk8l-api/issues/228)]
* ci/dependabot: go:(deps): bump google.golang.org/grpc from 1.79.1 to 1.79.2 [[GH-238](https://github.com/danroux/sk8l-api/issues/238)]
* ci/dependabot: go:(deps): bump google.golang.org/grpc from 1.79.2 to 1.79.3 [[GH-244](https://github.com/danroux/sk8l-api/issues/244)]
* ci/dependabot: go:(deps): bump google.golang.org/grpc from 1.79.3 to 1.80.0 [[GH-250](https://github.com/danroux/sk8l-api/issues/250)]
* ci/dependabot: go:(deps): bump google.golang.org/grpc from 1.80.0 to 1.81.0 [[GH-255](https://github.com/danroux/sk8l-api/issues/255)]
* ci/dependabot: go:(deps): bump google.golang.org/grpc from 1.81.0 to 1.81.1 [[GH-259](https://github.com/danroux/sk8l-api/issues/259)]
* ci/dependabot: go:(deps): bump google.golang.org/grpc from 1.81.1 to 1.83.0 [[GH-277](https://github.com/danroux/sk8l-api/issues/277)]
* ci/dependabot: go:(deps): bump google.golang.org/protobuf from 1.36.8 to 1.36.9 [[GH-227](https://github.com/danroux/sk8l-api/issues/227)]
* ci/dependabot: go:(deps): bump k8s.io/client-go from 0.33.1 to 0.33.4 [[GH-211](https://github.com/danroux/sk8l-api/issues/211)]
* ci/dependabot: go:(deps): bump k8s.io/client-go from 0.33.4 to 0.34.0 [[GH-223](https://github.com/danroux/sk8l-api/issues/223)]
* ci/dependabot: go:(deps): bump k8s.io/client-go from 0.34.0 to 0.35.2 [[GH-234](https://github.com/danroux/sk8l-api/issues/234)]
* ci/dependabot: go:(deps): bump k8s.io/client-go from 0.35.2 to 0.35.3 [[GH-246](https://github.com/danroux/sk8l-api/issues/246)]
* ci/dependabot: go:(deps): bump k8s.io/client-go from 0.35.3 to 0.36.0 [[GH-251](https://github.com/danroux/sk8l-api/issues/251)]
* ci/dependabot: go:(deps): bump k8s.io/client-go from 0.36.0 to 0.36.1 [[GH-256](https://github.com/danroux/sk8l-api/issues/256)]
* ci/dependabot: go:(deps): bump k8s.io/client-go from 0.36.1 to 0.36.3 [[GH-276](https://github.com/danroux/sk8l-api/issues/276)]
* ci/dependabot: go:(deps): bump sigs.k8s.io/yaml from 1.4.0 to 1.5.0 [[GH-213](https://github.com/danroux/sk8l-api/issues/213)]
* deps/k8s: bump protos/{apimachinery,api} to 0.34.5 [[GH-237](https://github.com/danroux/sk8l-api/issues/237)]

IMPROVEMENTS:

* ci/k8s: Upgrade setup-kind GitHub action from engineerd/setup-kind to helm/kind-action@v1.12.0 [[GH-272](https://github.com/danroux/sk8l-api/issues/272)]
* ci: fix false-positive "new field" reports in check_k8s_mapper_drift.sh by stopping go doc output parsing at the struct closing brace, eliminating receiver type names (e.g. EnvVar, JobSpec, PodStatus) from being treated as upstream fields [[GH-279](https://github.com/danroux/sk8l-api/issues/279)]
* go/deps: bump badger/v4 4.7.0 => 4.8.0 protobuf 1.36.6 => 1.36.8

- go:(deps): bump google.golang.org/protobuf from 1.36.6 to 1.36.8
- go:(deps): bump github.com/dgraph-io/badger/v4 from 4.7.0 to 4.8.0 dependencies [[GH-221](https://github.com/danroux/sk8l-api/issues/221)]
* go: Improve error handling, resilience, and test coverage across k8s client, store, watchers, and metrics collection [[GH-274](https://github.com/danroux/sk8l-api/issues/274)]
* k8s/protos: Add podIps to PodStatusResponse [[GH-239](https://github.com/danroux/sk8l-api/issues/239)]
* protos: Add StartingDeadlineSeconds && k8s mapper drift script and CI job [[GH-240](https://github.com/danroux/sk8l-api/issues/240)]
* refactor: Extract internal/dashboard package [[GH-241](https://github.com/danroux/sk8l-api/issues/241)]
* refactor: Extract internal/k8s package

Thread ctx through k8s.ClientInterface && fix watcher shutdown [[GH-243](https://github.com/danroux/sk8l-api/issues/243)]
* refactor: Extract internal/mapper package and update callers [[GH-242](https://github.com/danroux/sk8l-api/issues/242)]

BUG FIXES:

* go: Fix concurrent slice data races in GetCronjobs and allAndRunningJobsAnPods
ci: Add -race flag to Go test workflow to enforce race detection on CI [[GH-273](https://github.com/danroux/sk8l-api/issues/273)]

## v0.18.0 (August 21, 2025)

ENHANCEMENTS:

* ci/dependabot: docker:(deps): bump alpine from 3.21.3 to 3.22.1 [[GH-204](https://github.com/danroux/sk8l-api/issues/204)]
* ci/dependabot: docker:(deps): bump golang from 1.24.3 to 1.25.0 [[GH-207](https://github.com/danroux/sk8l-api/issues/207)]
* ci/dependabot: gha:(deps): bump actions/checkout from 4.2.2 to 5.0.0 [[GH-218](https://github.com/danroux/sk8l-api/issues/218)]
* ci/dependabot: gha:(deps): bump actions/setup-go from 5.4.0 to 5.5.0 [[GH-194](https://github.com/danroux/sk8l-api/issues/194)]
* ci/dependabot: gha:(deps): bump azure/setup-helm from 4.3.0 to 4.3.1 [[GH-219](https://github.com/danroux/sk8l-api/issues/219)]
* ci/dependabot: gha:(deps): bump docker/build-push-action from 6.16.0 to 6.18.0 [[GH-199](https://github.com/danroux/sk8l-api/issues/199)]
* ci/dependabot: gha:(deps): bump docker/login-action from 3.4.0 to 3.5.0 [[GH-216](https://github.com/danroux/sk8l-api/issues/216)]
* ci/dependabot: gha:(deps): bump docker/setup-buildx-action from 3.10.0 to 3.11.0 [[GH-208](https://github.com/danroux/sk8l-api/issues/208)]
* ci/dependabot: gha:(deps): bump golangci/golangci-lint-action from 6.0.1 to 8.0.0 [[GH-190](https://github.com/danroux/sk8l-api/issues/190)]
* ci/dependabot: go:(deps): bump google.golang.org/grpc from 1.72.0 to 1.72.1 [[GH-195](https://github.com/danroux/sk8l-api/issues/195)]
* ci/dependabot: go:(deps): bump google.golang.org/grpc from 1.72.1 to 1.72.2 [[GH-203](https://github.com/danroux/sk8l-api/issues/203)]
* ci/dependabot: go:(deps): bump k8s.io/client-go from 0.32.3 to 0.33.1 [[GH-185](https://github.com/danroux/sk8l-api/issues/185)]

IMPROVEMENTS:

* misc: Improve logging, Improve dashboard layout and panel design, go refactorings: still a work in progress

- Logging: Use github.com/rs/zerolog to generate json structured logs
- Dashboards: Update dependencies
- Dashboards: Add bargauge && status-history panels
- Dashboards: Use grafana templating features to generate less panels and repeat them per cronjob
- Dashboards: Add namespace/cronjobs names variables to enable filtering the panels
- go: Add functions to generate panels cronjobsTotalsTimeseries, totalsBarGaugePanel, allStatusHistory, allStateTimelines
- go: No longer use go's default mux handler
- go: improve servers setup/shutdown [[GH-205](https://github.com/danroux/sk8l-api/issues/205)]

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