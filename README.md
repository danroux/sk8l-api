# sk8l(skål)

<p align="center">
  <picture>
    <img src="https://sk8l.io/charts/logo.png" alt="sk8l Icon" width="200" />
  </picture>
</p>

<h3 align="center">
  Easy Cronjob and Job workload visualization and monitoring in kubernetes
</h3>

<p align="center">
| <a href="https://sk8l.io"><b>Documentation & screenshots</b></a> | <a href="https://artifacthub.io/packages/helm/sk8l/sk8l"><b>helm chart</b></a> |
  <a href="https://artifacthub.io/packages/helm/sk8l/sk8l">
      <img src="https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/sk8l" alt="Artifact Hub">
  </a>
</p>

Monitor and view your cronjobs/job activity. Use the exported prometheus metrics to get alerts when your cronjob fails, takes longer than expected or does not start when it should.

- Get an overview of your cronjobs running in a namespace
- See which cronjobs run more often
- Get a quick glimpse of a cronjob, job or pod configuration via the UI
- Use the exported prometheus metrics:
  - Total registered cronjobs
  - Total completed cronjobs
  - Total cronjobs failures
  - Amount of current running cronjobs
  - Total completions of a cronjobs
  - Current duration of a running cronjob
  - Total failures of a cronjob
- Familiar UI, based on the open source Primer framework by github.
  - https://primer.style/
  - https://github.com/primer

## HELM Chart

https://artifacthub.io/packages/helm/sk8l/sk8l

```
helm repo add sk8l https://sk8l.io/charts
helm repo update

helm search repo sk8l

helm upgrade --install [RELEASE_NAME] sk8l/sk8l \
--namespace sk8l \
--create-namespace=true \
--set namespace.name=[NAMESPACE] \
--set serviceAccount.metadata.namespace.name=[NAMESPACE]
```

### Secrets

#### TLS

The communication between apps is encrypted.

To manually configure TLS, first create/retrieve a key & certificate pair. Then create TLS secrets in the namespace:

```
kubectl create secret tls -n NAMESPACE sk8l-server-cert-secret --cert=path/to/tls.cert --key=path/to/tls.key
kubectl create secret tls -n NAMESPACE sk8l-ca-root-cert-secret --cert=ca-cert.pem --key=ca-key.pem
```

## Supported Kubernetes versions

The Kubernetes community releases minor versions roughly every three months. These are the versions currently supported and tested against.

| Version       | Tested Version |
| ------------- | -------------- |
| v1.37         | v1.37.0        |
| v1.36         | v1.36.4        |
| v1.35         | v1.35.8        |
| v1.34         | v1.34.11       |

## Prometheus metrics

`sk8l` exports standard OpenMetrics Prometheus metrics with dimensional labels for cluster-wide and per-cronjob observability:

| Metric Name | Type | Labels | Description |
|:---|:---|:---|:---|
| `sk8l_cronjobs_count` | Gauge | `namespace`, `status` (`registered`, `running`, `failing`, `completed`) | Current count of cronjobs by namespace and status |
| `sk8l_cronjob_last_run_status` | Gauge | `namespace`, `cronjob`, `status` (`succeeded`, `failed`) | Status of the latest execution (1 if active, 0 otherwise) |
| `sk8l_cronjob_last_run_duration_seconds` | Gauge | `namespace`, `cronjob` | Duration in seconds of the latest execution |
| `sk8l_cronjob_active_jobs` | Gauge | `namespace`, `cronjob` | Number of currently active child jobs |
| `sk8l_cronjob_jobs_count` | Gauge | `namespace`, `cronjob`, `status` (`succeeded`, `failed`) | Retained child jobs count by completion status |
| `sk8l_cronjob_info` | Gauge | `namespace`, `cronjob`, `schedule`, `suspend` | CronJob metadata and configuration (value = 1) |

### Example PromQL Queries

* **Alert on any failing CronJob:**
  ```promql
  sk8l_cronjob_last_run_status{status="failed"} == 1
  ```
* **Cluster-wide failing CronJobs count:**
  ```promql
  sum(sk8l_cronjobs_count{status="failing"})
  ```
* **Top 5 slowest CronJobs:**
  ```promql
  topk(5, sk8l_cronjob_last_run_duration_seconds)
  ```

## Grafana Dashboard

`sk8l` provides an official, ready-to-import Grafana dashboard located in [`dashboards/sk8l-overview.json`](dashboards/sk8l-overview.json). It includes template variables for filtering by namespace and cronjob name, cluster health KPIs, state timelines, and execution duration trends.

Additionally, `sk8l-api` can generate dynamic dashboard JSON configurations via the `/api/dashboards` endpoint.

