# cert-manager-webhook-inwx

A [cert-manager](https://cert-manager.io/) ACME DNS01 webhook solver for [INWX](https://www.inwx.com/).

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

## Configuration

### 1. Create a Kubernetes secret with your INWX credentials

```bash
kubectl create secret generic inwx-credentials \
  --namespace cert-manager \
  --from-literal=username=<YOUR_INWX_USERNAME> \
  --from-literal=password=<YOUR_INWX_PASSWORD>
```

The secret must exist in the same namespace as your `Issuer`, or in the cert-manager namespace for `ClusterIssuer`.

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
              # Optional: override secret keys (defaults: "username", "password")
              # usernameKey: user
              # passwordKey: pass
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
| `metrics.enabled` | `false` | Expose `/metrics` and `/healthz` on a dedicated HTTP port |
| `metrics.port` | `8080` | Metrics server port |
| `metrics.serviceMonitor.enabled` | `false` | Create a Prometheus Operator `ServiceMonitor` |
| `metrics.podMonitor.enabled` | `false` | Create a Prometheus Operator `PodMonitor` |
| `otel.enabled` | `false` | Inject `OTEL_EXPORTER_OTLP_ENDPOINT` into the container |
| `otel.endpoint` | `""` | OTLP/gRPC collector endpoint, e.g. `otel-collector.monitoring.svc:4317` |
| `otel.serviceName` | `""` | Value for `OTEL_SERVICE_NAME` (defaults to chart fullname) |

### Networking

| Key | Default | Description |
|-----|---------|-------------|
| `service.type` | `ClusterIP` | Service type |
| `service.port` | `443` | Service port |
| `networkPolicy.enabled` | `false` | Create a `NetworkPolicy` restricting ingress/egress |
| `ingress.enabled` | `false` | Create a Kubernetes `Ingress` for the metrics endpoint |
| `ingress.className` | `""` | `ingressClassName` |
| `ingress.annotations` | `{}` | Ingress annotations |
| `ingress.hosts` | see values | Host rules (default path: `/metrics`) |
| `ingress.tls` | `[]` | TLS configuration |
| `httpRoute.enabled` | `false` | Create a Gateway API `HTTPRoute` for the metrics endpoint |
| `httpRoute.parentRefs` | `[]` | Gateway references (required when `httpRoute.enabled: true`) |
| `httpRoute.hostnames` | `[]` | Hostnames to match |

### Probes

| Key | Default | Description |
|-----|---------|-------------|
| `startupProbe` | see values | Probe checked until first success; gives the webhook up to 60 s to start before liveness kicks in |
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
   kubectl get secret inwx-credentials -n cert-manager -o jsonpath='{.data}' | base64 -d
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

### Requirements

- Go 1.25+
- Docker
- [Task](https://taskfile.dev) (optional but recommended)

### Common tasks

```bash
task build          # compile binary → bin/webhook
task test           # run tests with race detector + coverage
task lint           # golangci-lint
task ci             # full local pipeline: tidy, lint, test, vulncheck, helm lint
task helm:lint      # lint and dry-run render the Helm chart
task docker:build   # build Docker image (TAG=dev)
```

See `Taskfile.yml` for all available tasks.

### Building manually

```bash
go mod tidy
go build -o bin/webhook ./src/
```

```bash
docker build -t cert-manager-webhook-inwx:dev .
```

## License

MIT
