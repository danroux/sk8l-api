# sk8l

This chart installs sk8l/skål(https://sk8l.io)

## Get Repository

```
helm repo add sk8l https://sk8l.io/charts
helm repo update
```

## Install chart

```
helm upgrade --install [RELEASE_NAME] sk8l/sk8l \
--set namespace=[NAMESPACE] \
--set serviceAccount.metadata.namespace=[NAMESPACE]
```

## Uninstall Chart

```
helm uninstall [RELEASE_NAME]
```

## Configuration

See . To see all configurable options with detailed comments, visit the chart's values.yaml, or run these configuration commands:

```
helm show values sk8l/sk8l
```

If you wish to consume the exported metrics with prometheus, you can point prometheus to port 8590(HTTPS).

## Kubernetes SD configurations

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

The commmunication between apps is encrypted so during installation server and ca certificates are created as placeholders and used as mounted volumes.

You should replace them as soon as possible and create your own.

### Labels

### Environment Variables

The frontend app calls the backend api on `https://localhost:9080`. You can change this by updating the ENV Variable `VUE_APP_SK8L_API_URL` on the `sk8l-web-configmap` configmap.

## Usage

After installation and once the apps are running, you should be able to navigate to https://localhost:8001 and see your cronjobs listed there.

## More Info

https://sk8l.io

This functionality is in beta and is subject to change. The code is provided as-is with no warranties.
