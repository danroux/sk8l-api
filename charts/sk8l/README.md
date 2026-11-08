# sk8l

This chart installs sk8l(skål)(https://sk8l.io). A cronjob/job monitoring and visualization web application in Kubernetes.

## Get Repository

```
helm repo add sk8l https://sk8l.io/charts
helm repo update
helm search repo sk8l
```

## Install chart

```bash
helm upgrade --install [RELEASE_NAME] sk8l/sk8l \
  --namespace [NAMESPACE] \
  --create-namespace
```

## Uninstall Chart

```
helm uninstall [RELEASE_NAME]
```

## Configuration

To see all configurable options with detailed comments, visit the chart's values.yaml, or run these configuration commands:

```
helm show values sk8l/sk8l
```

### Prometheus

If you wish to consume the exported metrics with prometheus, you can point prometheus to port 8590(HTTPS).

#### Kubernetes SD configurations

You can also benefit from prometheus support of Kubernetes SD and use the pod's label `sk8l.io/api-scrape-port` to configure the scrapping jobs:

```
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

### Secrets

#### TLS

The commmunication between apps is encrypted.

To manually configure TLS, first create/retrieve a key & certificate pair. Then create TLS secrets in the namespace:

```
kubectl create secret tls -n NAMESPACE sk8l-server-cert-secret --cert=path/to/tls.cert --key=path/to/tls.key
kubectl create secret tls -n NAMESPACE sk8l-ca-root-cert-secret --cert=ca-cert.pem --key=ca-key.pem
```

### Labels

### Environment Variables

The web UI is embedded directly in `sk8l-api` and uses same-origin ConnectRPC/gRPC-Web requests on port `8590` by default. If needed, you can override `sk8lApi.apiURL` in `values.yaml`.

## Usage

After installation and once the `sk8l-api` pod is running, port-forward `svc/sk8l` on `8590:8590` and navigate to https://localhost:8590 to view your cronjobs and jobs.

## More Info

https://sk8l.io

This functionality is in beta and is subject to change. The code is provided as-is with no warranties.
