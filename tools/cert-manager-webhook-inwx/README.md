# cert-manager-webhook-inwx

A [cert-manager](https://cert-manager.io/) ACME DNS01 webhook solver for [INWX](https://www.inwx.com/).

> One tool in a [moon](https://moonrepo.dev) monorepo — see the
> [root README](../../README.md) for the shared build/publish pipeline.

## Prerequisites

- Kubernetes 1.24+
- cert-manager 1.13+
- Helm 3.x

## Installation

```bash
helm install cert-manager-webhook-inwx \
  oci://ghcr.io/henningjanssen/charts/cert-manager-webhook-inwx \
  --namespace cert-manager \
  --create-namespace
```

Without `--version` this installs the newest stable release. Pin one with
`--version x.y.z`.

## Configuration

### 1. Create a Kubernetes secret with your INWX credentials

```bash
kubectl create secret generic inwx-credentials \
  --namespace cert-manager \
  --from-literal=username=<YOUR_INWX_USERNAME> \
  --from-literal=password=<YOUR_INWX_PASSWORD>
```

The secret must be in the cert-manager namespace for a `ClusterIssuer`. For an `Issuer` it must be in the `Issuer`'s namespace, and that namespace must be listed in the `credentialNamespaces` Helm value unless it is the release namespace.

#### Accounts with two-factor authentication

If the INWX account uses 2FA, add its base32 TOTP secret (the text form of the setup QR code) to the secret:

```bash
kubectl create secret generic inwx-credentials \
  --namespace cert-manager \
  --from-literal=username=<YOUR_INWX_USERNAME> \
  --from-literal=password=<YOUR_INWX_PASSWORD> \
  --from-literal=totpSecret=<YOUR_BASE32_TOTP_SECRET>
```

INWX accepts each code only once, so the webhook does at most one 2FA login per 30 seconds and asks cert-manager to retry when more are waiting. Issuing many certificates at once is slower on such an account, and replicas may collide and retry.

### 2. Create a ClusterIssuer referencing the webhook

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-inwx
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: user@example.com
    privateKeySecretRef:
      name: letsencrypt-account-key
    solvers:
      - dns01:
          webhook:
            groupName: acme.inwx.com
            solverName: inwx
            config:
              secretName: inwx-credentials
              # Optional: override secret keys (defaults: "username", "password", "totpSecret")
              # usernameKey: user
              # passwordKey: pass
              # totpSecretKey: totp
```

### 3. Issue a certificate

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-tls
  namespace: default
spec:
  secretName: example-tls
  dnsNames:
    - example.com
    - "*.example.com"
  issuerRef:
    name: letsencrypt-inwx
    kind: ClusterIssuer
```

## Helm values

### Core

| Key | Default | Description |
|-----|---------|-------------|
| `groupName` | `acme.inwx.com` | Unique group name for the webhook API |
| `replicaCount` | `1` | Number of webhook replicas |
| `logLevel` | `2` | Log verbosity: 0 = errors only, 4 = debug |

### Image

| Key | Default | Description |
|-----|---------|-------------|
| `image.repository` | `ghcr.io/henningjanssen/cert-manager-webhook-inwx` | Image repository |
| `image.tag` | `""` | Image tag — defaults to chart `appVersion` |
| `image.digest` | `""` | Image digest (e.g. `sha256:…`). Takes precedence over `tag` when set |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy |

### cert-manager integration

| Key | Default | Description |
|-----|---------|-------------|
| `certManager.namespace` | `cert-manager` | Namespace where cert-manager is installed |
| `certManager.serviceAccountName` | `cert-manager` | cert-manager controller service account name |

### TLS / PKI

| Key | Default | Description |
|-----|---------|-------------|
| `pki.caDuration` | `43800h` | Self-signed CA lifetime; keep it well above `pki.duration` |
| `pki.duration` | `8760h` | Webhook TLS certificate lifetime |
| `pki.renewBefore` | `720h` | Renewal window before expiry |
| `pki.algorithm` | `ECDSA` | Key algorithm (`ECDSA` or `RSA`) |
| `pki.size` | `256` | Key size (256/384 for ECDSA; 2048/4096 for RSA) |

### Scaling & availability

| Key | Default | Description |
|-----|---------|-------------|
| `autoscaling.enabled` | `false` | Enable HPA (ignores `replicaCount` when true) |
| `autoscaling.minReplicas` | `1` | HPA minimum replicas |
| `autoscaling.maxReplicas` | `5` | HPA maximum replicas |
| `autoscaling.targetCPUUtilizationPercentage` | `80` | HPA CPU target (%) |
| `autoscaling.targetMemoryUtilizationPercentage` | `""` | HPA memory target (%) |
| `podDisruptionBudget.enabled` | `false` | Enable PodDisruptionBudget |
| `podDisruptionBudget.minAvailable` | `1` | Minimum available pods |

### Observability

| Key | Default | Description |
|-----|---------|-------------|
| `metrics.serviceMonitor.enabled` | `false` | Create a Prometheus Operator `ServiceMonitor` |
| `metrics.podMonitor.enabled` | `false` | Create a Prometheus Operator `PodMonitor` |
| `otel.enabled` | `false` | Inject `OTEL_EXPORTER_OTLP_ENDPOINT` into the container |
| `otel.endpoint` | `""` | OTLP/gRPC collector URL, e.g. `http://otel-collector.monitoring.svc:4317` (`https://` for TLS) |
| `otel.serviceName` | `""` | Value for `OTEL_SERVICE_NAME` (defaults to chart fullname) |

### Networking

| Key | Default | Description |
|-----|---------|-------------|
| `service.type` | `ClusterIP` | Service type |
| `service.port` | `443` | Service port |
| `networkPolicy.enabled` | `false` | Create a `NetworkPolicy` restricting ingress/egress |
| `networkPolicy.extraEgress` | `[]` | Extra egress rules; add the API server port if it is not 443 (often 6443) and the OTLP port for tracing |

### Probes

| Key | Default | Description |
|-----|---------|-------------|
| `startupProbe` | see values | Probe checked until first success; gives the webhook up to 120 s to start before liveness kicks in |
| `livenessProbe` | see values | Active after startup probe succeeds |
| `readinessProbe` | see values | Active after startup probe succeeds |

### Scheduling

| Key | Default | Description |
|-----|---------|-------------|
| `nodeSelector` | `{}` | Node selector |
| `tolerations` | `[]` | Pod tolerations |
| `affinity` | `{}` | Pod affinity |
| `topologySpreadConstraints` | `[]` | Topology spread constraints |
| `priorityClassName` | `""` | Priority class name |

### Advanced

| Key | Default | Description |
|-----|---------|-------------|
| `credentialNamespaces` | `[]` | Extra namespaces whose credential secrets the webhook may read (for an `Issuer` elsewhere) |
| `credentialSecretNames` | `[]` | Restrict the webhook to these credential secret names; empty allows every secret in those namespaces |
| `extraEnv` | `[]` | Extra environment variables |
| `extraVolumes` | `[]` | Extra volumes |
| `extraVolumeMounts` | `[]` | Extra volume mounts |
| `podAnnotations` | `{}` | Pod annotations |
| `podLabels` | `{}` | Pod labels |
| `serviceAccount.create` | `true` | Create a ServiceAccount |
| `serviceAccount.annotations` | `{}` | ServiceAccount annotations |
| `terminationGracePeriodSeconds` | `30` | Graceful termination period |

## Troubleshooting

### Certificate stuck in pending

1. Check webhook logs: `kubectl logs -n cert-manager -l app.kubernetes.io/name=cert-manager-webhook-inwx`
2. Describe the failing `CertificateRequest`: `kubectl describe certificaterequest -n <namespace> <name>`
3. Verify the secret exists and contains the right keys:
   ```bash
   kubectl get secret inwx-credentials -n cert-manager -o jsonpath='{.data.username}' | base64 -d
   ```

### "secret not found" error

The credential secret must be in the same namespace as the `Issuer`. For a `ClusterIssuer`, cert-manager reads secrets from the cert-manager namespace — make sure the secret is created there.

### DNS record not cleaned up after challenge

cert-manager calls `CleanUp` after every challenge regardless of outcome. If a record remains, check the webhook logs for errors from the INWX API and verify the credentials have permission to delete records in that zone.

### Webhook TLS certificate not issued

The webhook bootstraps its own TLS via cert-manager. If the `APIService` is not ready, cert-manager itself may not be running yet. Check:

```bash
kubectl get certificate -n cert-manager
kubectl describe apiservice v1alpha1.acme.inwx.com
```

## Development

Tasks and the release process are in the [root README](../../README.md).

To test against the real INWX sandbox, set `INWX_SANDBOX_USERNAME`, `INWX_SANDBOX_PASSWORD`, `INWX_SANDBOX_ZONE` (and `INWX_SANDBOX_TOTP_SECRET` for 2FA), then run `go test -run TestInwxSandbox ./src/`. It is skipped otherwise.

## License

MIT
