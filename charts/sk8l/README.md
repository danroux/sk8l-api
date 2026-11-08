# sk8l

This chart installs sk8l(skål)(https://sk8l.io). A cronjob/job monitoring and visualization web application in Kubernetes.

## Get Repository

```bash
helm repo add sk8l https://sk8l.io/charts
helm repo update
helm search repo sk8l
```

## Install chart

By default (`tls.enabled: false`), `sk8l` installs with zero external certificate prerequisites and serves the embedded UI, Connect/gRPC-Web/gRPC API (`h2c`), health probes, and Prometheus metrics on port `8590`:

```bash
helm upgrade --install [RELEASE_NAME] sk8l/sk8l \
  --namespace [NAMESPACE] \
  --create-namespace
```

## Uninstall Chart

```bash
helm uninstall [RELEASE_NAME] --namespace [NAMESPACE]
```

## Configuration

To see all configurable options with detailed comments, visit the chart's `values.yaml`, or run:

```bash
helm show values sk8l/sk8l
```

### Prometheus

If you wish to consume the exported metrics with Prometheus, point Prometheus to `/metrics` on port `8590` (`HTTP` by default, or `HTTPS` when `tls.enabled: true`).

#### Kubernetes SD configurations

You can also benefit from Prometheus Kubernetes SD and use the pod annotation `sk8l.io/api-scrape-port` to configure scraping jobs:

```yaml
kubernetes_sd_configs:
  - role: pod
relabel_configs:
  - source_labels: [__meta_kubernetes_pod_annotation_sk8l_io_api_scrape_port]
    action: keep
    regex: (\d{4})
  - source_labels: [__address__, __meta_kubernetes_pod_annotation_sk8l_io_api_scrape_port]
    action: replace
    regex: ([^:]+)(?::\d+)?;(\d+)
    replacement: $1:$2
    target_label: __address__
```

### Optional In-Pod TLS

By default `tls.enabled` is `false` (ideal when terminating TLS at an Ingress Controller, Gateway API, Service Mesh, or accessing locally via `kubectl port-forward`).

To enable end-to-end HTTPS/TLS directly on the `sk8l-api` pod:

1. Create a standard Kubernetes TLS secret in the target namespace:
   ```bash
   kubectl create secret tls -n [NAMESPACE] sk8l-server-cert-secret \
     --cert=path/to/tls.crt \
     --key=path/to/tls.key
   ```
2. Enable TLS in Helm values:
   ```bash
   helm upgrade --install [RELEASE_NAME] sk8l/sk8l \
     --namespace [NAMESPACE] \
     --set tls.enabled=true \
     --set tls.secretName=sk8l-server-cert-secret
   ```
   *(Optional: set `tls.caSecretName` if you also want to mount a separate CA bundle secret into `ClientCAs`).*

### Environment Variables

The web UI is embedded directly in `sk8l-api` and uses same-origin ConnectRPC/gRPC-Web requests on port `8590` by default. If needed, you can override `sk8lApi.apiURL` in `values.yaml`.

## Usage

After installation and once the `sk8l-api` pod is running, port-forward `svc/sk8l` on `8590:8590`:

```bash
kubectl -n [NAMESPACE] port-forward svc/sk8l 8590:8590
```

Navigate to `http://localhost:8590` (or `https://localhost:8590` when `tls.enabled: true`) to view your cronjobs and jobs.

## More Info

https://sk8l.io

This functionality is in beta and is subject to change. The code is provided as-is with no warranties.
