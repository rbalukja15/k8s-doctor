# Overlay drift

Drift compares two kustomize overlays of the same application and reports what differs beyond what is expected. It works for two environments (`overlays/staging` vs a prod tenant) and for two tenants in one environment (`tenant/acme` vs `tenant/globex`). The command that runs it is #30.

## Matching

Objects are paired on their kind plus the name they have in their source file. The manifest collector records that name as `source.overlay.origins` in the snapshot, working back from the built name with the `namePrefix` and `nameSuffix` of every kustomization the overlay includes and the generator hashes on ConfigMaps and Secrets. The overlay's own Namespace object matches whatever it is called.

An object in one overlay only is a `drift-resource-missing` finding (medium) on the overlay that has it.

## Field differences

Matched objects are compared field by field. Paths follow the manifest layout down to the field the snapshot records, for example `spec.template.spec.containers[name=api].securityContext.runAsNonRoot`. List items with a name are addressed by name, so reordering is not drift. ConfigMap and Secret keys are compared by name only (`data.LOG_LEVEL`); values are never collected.

Before comparing, values are normalized:
- **Object names:** a reference to a renamed object, such as an HPA's `scaleTargetRef.name`, goes back to its source name.
- **Namespace:** the overlay's namespace becomes `{namespace}`.
- **Overlay name:** with the `hostnames` preset, the overlay's name in any value becomes `{overlay}`. So `acme.shop.example.com` and `globex.shop.example.com` are equal, but a different domain or path is not.

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
| `resources` | any `resources` field |
| `imageTags` | an image that differs only in tag or digest |
| `hostnames` | the overlay name inside values (see above) |
| `hpaBounds` | HPA `minReplicas` and `maxReplicas` |
| `ingressClassAndIssuer` | Ingress class and cert-manager issuer (from #43) |
| `ipAllowlists` | KongPlugin `config.allow` and `config.deny` (from #43) |

Allowlist entries are field-path globs, optionally limited to one kind with `Kind:`. `*` matches within one path segment and `**` across segments.

## Not covered yet

Ingress, NetworkPolicy and KongPlugin are not in the snapshot until #43. They take part in matching and diffing once they are.
