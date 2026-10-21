# Rules

Run `k8s-doctor rules` for the list built into your binary.

| ID | Category | Default severity | Modes | Flags |
|---|---|---|---|---|
| `resources-requests-missing` | resources | medium | manifest, cluster | A container with no CPU or memory request. A limit counts as a request, since Kubernetes copies it. |
| `resources-memory-limit-missing` | resources | medium | manifest, cluster | A container with no memory limit. CPU limits are not required. |
| `probe-readiness-missing` | probes | medium | manifest, cluster | A Deployment, StatefulSet or DaemonSet container with no readiness probe. Jobs and CronJobs are skipped. |
| `probe-liveness-missing` | probes | low | manifest, cluster | Same scope, no liveness probe. |
| `image-tag-mutable` | images | medium | manifest, cluster | An image with `:latest` or no tag, unless pinned by digest. A registry port is not mistaken for a tag. |
| `image-pull-policy-stale` | images | high | manifest, cluster | A mutable image with an explicit `IfNotPresent` or `Never` pull policy. |
| `deployment-single-replica` | availability | medium | manifest, cluster | A Deployment with one replica (explicit or defaulted), unless an HPA keeps `minReplicas >= 2`. Scaled-to-zero Deployments and StatefulSets are skipped. |

## Configuring rules

Put a `.k8s-doctor.yaml` in the working directory, or pass `--config`:

```yaml
rules:
  probe-liveness-missing:
    enabled: false
  image-tag-mutable:
    severity: high
```

Severity overrides do not change finding IDs.
