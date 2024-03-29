# sk8l

sk8l/skål

Monitor your cronjobs activity. Use the exported prometheus metrics to get alerts when your cronjob fails, takes longer than expected or does not start when it should.

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

## Screenshots

https://sk8l.io/

### HELM Chart

https://artifacthub.io/packages/helm/sk8l/sk8l

```
helm repo add sk8l https://sk8l.io/charts
helm repo update

helm search repo sk8l

helm upgrade --install [RELEASE_NAME] sk8l/sk8l \
--set namespace.name=[NAMESPACE] \
--set serviceAccount.metadata.namespace.name=[NAMESPACE]
```

### Prometheus metrics

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

### Grafana Dashboard

sk8l can generate an annotations json configuration based on the current configured cronjobs on kubernetes that can be copy/pasted and imported into Grafana to create a dashboard.
