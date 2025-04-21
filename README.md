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

The commmunication between apps is encrypted.

To manually configure TLS, first create/retrieve a key & certificate pair. Then create TLS secrets in the namespace:

```
kubectl create secret tls -n NAMESPACE sk8l-server-cert-secret --cert=path/to/tls.cert --key=path/to/tls.key
kubectl create secret tls -n NAMESPACE sk8l-ca-root-cert-secret --cert=ca-cert.pem --key=ca-key.pem
```

## Supported Kubernetes versions

The Kubernetes community releases minor versions roughly every three months. These are the versions currently supported and tested against.

| Version       | Tested Version |
| ------------- | ----------------- |
| v1.31         | v1.31.0           |
| v1.30         | v1.30.4           |
| v1.29         | v1.29.8           |

## Prometheus metrics

sk8l collects and publishes aggregated metrics for all the configured cronjobs on a namespace and also metrics per each single cronjob.

|                       Name                       |              Description              |
|:------------------------------------------------:|:-------------------------------------:|
| sk8l_[NAMESPACE]_registered_cronjobs_total       | Total registered cronjobs             |
| sk8l_[NAMESPACE]_completed_cronjobs_total        | Total completed cronjobs              |
| sk8l_[NAMESPACE]_failing_cronjobs_total          | Total cronjobs failures               |
| sk8l_[NAMESPACE]_running_cronjobs_total          | Amount of current running cronjobs    |
| sk8l_[NAMESPACE]_[CRONJOB_NAME]_completion_total | Total completions of a cronjobs       |
| sk8l_[NAMESPACE]_[CRONJOB_NAME]_duration_seconds | Current duration of a running cronjob |
| sk8l_[NAMESPACE]_[CRONJOB_NAME]_failure_total    | Total failures of a cronjob           |

## Grafana Dashboard

sk8l can generate an annotations json configuration based on the current configured cronjobs on kubernetes that can be copy/pasted and imported into Grafana to create a dashboard.
