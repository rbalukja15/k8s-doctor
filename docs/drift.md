# Overlay drift

Drift compares two kustomize overlays of the same application and reports what differs beyond what is expected. It works for two environments (`overlays/staging` vs a prod tenant) and for two tenants in one environment (`tenant/acme` vs `tenant/globex`). Run it with `k8s-doctor drift --base <overlay> --target <overlay>` ([cli.md](cli.md#drift)).

## Matching

Objects are paired on their kind plus the name they have in their source file. The manifest collector records that name as `source.overlay.origins` in the snapshot, working back from the built name with the `namePrefix` and `nameSuffix` of every kustomization the overlay includes and the generator hashes on ConfigMaps and Secrets. The overlay's own Namespace object matches whatever it is called.

An object in one overlay only is a `drift-resource-missing` finding (medium) on the overlay that has it.

## Field differences

Matched objects are compared field by field. Paths follow the manifest layout down to the field the snapshot records, for example `spec.template.spec.containers[name=api].securityContext.runAsNonRoot`. List items with a name are addressed by name, so reordering is not drift. ConfigMap and Secret keys are compared by name only (`data.LOG_LEVEL`); values are never collected.

Before comparing, values are normalized:
- **Object names:** a reference to a renamed object (an HPA's `scaleTargetRef.name`, a ConfigMap or Secret volume's source) goes back to its source name, looked up by kind and name.
- **Namespace:** a value that is exactly the overlay's namespace becomes `{namespace}`.
- **Overlay name:** with the `hostnames` preset, the overlay's name becomes `{overlay}` where it is a whole part of a DNS label inside a hostname, so `acme.shop.example.com` and `api-acme.shop.example.com` normalize. A bare `staging`, or `staging` in a URL path or image repository, is a real value and still shows as drift.

Each remaining difference is a `drift-field` finding on the target object, with both values as evidence (`(unset)` when one side lacks the field):

| Severity | Applies to |
|---|---|
| high | security settings (`securityContext`, `privileged`, host namespaces, `hostPath`) |
| medium | env and config key differences |
| low | anything else |

## Expected differences

```yaml
drift:
  presets: [replicas, resources, imageTags, hostnames, hpaBounds]   # the default
  allowlist:
    - "ConfigMap:data.FEATURE_*"
    - "Deployment:spec.template.spec.containers[*].env[name=LOG_LEVEL]**"
```

Leaving `presets` out gives the default set above; `presets: []` turns them all off.

| Preset | Ignores |
|---|---|
| `replicas` | `spec.replicas` |
| `resources` | any `resources` field, including an empty one |
| `imageTags` | a container or init container image that differs only in tag or digest |
| `hostnames` | the overlay name inside values (see above) |
| `hpaBounds` | HPA `minReplicas` and `maxReplicas` |
| `ingressClassAndIssuer` | Ingress class and cert-manager issuer (from #43) |
| `ipAllowlists` | KongPlugin `config.allow` and `config.deny` (from #43) |

Allowlist entries are field-path globs, optionally limited to one kind with `Kind:`. `*` matches within one path segment and `**` across segments.

## Fixtures

`fixtures/drift/` holds labeled pairs of overlays, run by `TestDriftFixtures` with the default presets: an environment pair (staging vs `acme`, where only the prod HPA is drift), a clean tenant pair (`acme` vs `globex`), a drifted tenant (`umbrella`, no `runAsNonRoot` and an extra config key) and a near-miss tenant (`hooli`, which differs only where a preset expects it). The format is in [fixtures/README.md](../fixtures/README.md#drift-fixtures).

## Not covered yet

- Ingress, NetworkPolicy and KongPlugin are not in the snapshot until #43. They take part in matching and diffing once they are.
- The HTML report's drift section waits for the HTML report (#19), and per-target drift in the v2 report for #41.
- `make eval` is still a stub (#21); the drift fixtures are ready for it to score.
