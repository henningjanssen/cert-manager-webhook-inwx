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
              # Optional: override secret keys (default: username, password)
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

| Key | Default | Description |
|-----|---------|-------------|
| `groupName` | `acme.inwx.com` | Unique group name for the webhook API |
| `image.repository` | `ghcr.io/henningjanssen/cert-manager-webhook-inwx` | Docker image repository |
| `image.tag` | `""` (uses `appVersion`) | Docker image tag |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy |
| `replicaCount` | `1` | Number of webhook replicas |
| `certManager.namespace` | `cert-manager` | Namespace where cert-manager is installed |
| `certManager.serviceAccountName` | `cert-manager` | cert-manager service account name |
| `logLevel` | `2` | Webhook log verbosity (0–4) |
| `resources` | `{}` | Pod resource requests/limits |
| `nodeSelector` | `{}` | Node selector |
| `tolerations` | `[]` | Pod tolerations |
| `affinity` | `{}` | Pod affinity |

## Development

### Requirements

- Go 1.22+
- Docker

### Building locally

```bash
go mod tidy
go build -o webhook ./src/
```

### Building the Docker image

```bash
docker build -t cert-manager-webhook-inwx:dev .
```

### Running locally (requires TLS certs in docker/certs/)

```bash
cd docker
docker compose up
```

## License

MIT
