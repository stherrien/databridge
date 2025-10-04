# DataBridge Kubernetes Deployment Guide

Complete guide for deploying DataBridge on Kubernetes with high availability, autoscaling, and monitoring.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Architecture](#architecture)
- [Deployment Options](#deployment-options)
- [Configuration](#configuration)
- [Scaling](#scaling)
- [Monitoring](#monitoring)
- [Troubleshooting](#troubleshooting)

## Prerequisites

### Required Tools

```bash
# kubectl (1.24+)
kubectl version --client

# Kubernetes cluster (1.24+)
kubectl cluster-info

# Optional: kustomize
kustomize version

# Optional: helm
helm version
```

### Cluster Requirements

- **Kubernetes**: 1.24 or later
- **Storage Class**: Dynamic provisioning support
- **LoadBalancer**: For external access (or Ingress Controller)
- **Resources**: 3+ nodes with 2 CPU and 4GB RAM each

## Quick Start

### Deploy DataBridge

```bash
# Create namespace
kubectl apply -f namespace.yaml

# Create ConfigMap and Secret
kubectl apply -f configmap.yaml
kubectl apply -f secret.yaml

# Deploy StatefulSet
kubectl apply -f statefulset.yaml

# Create Services
kubectl apply -f service.yaml

# Check deployment
kubectl get pods -n databridge
kubectl get svc -n databridge
```

### Access DataBridge

```bash
# Port forward to local machine
kubectl port-forward -n databridge svc/databridge 8080:8080

# Or get LoadBalancer IP
kubectl get svc databridge -n databridge

# Access API
curl http://localhost:8080/api/health
```

## Architecture

### Components

```
┌─────────────────────────────────────────┐
│          LoadBalancer / Ingress         │
└─────────────────┬───────────────────────┘
                  │
┌─────────────────▼───────────────────────┐
│            Service (ClusterIP)          │
└─────────────────┬───────────────────────┘
                  │
     ┌────────────┼────────────┐
     │            │            │
┌────▼───┐   ┌───▼────┐  ┌───▼────┐
│ Pod-0  │   │ Pod-1  │  │ Pod-2  │
│ (Raft) │◄──┤ (Raft) ├──┤ (Raft) │
└────┬───┘   └───┬────┘  └───┬────┘
     │           │            │
┌────▼───┐   ┌───▼────┐  ┌───▼────┐
│  PVC-0 │   │  PVC-1 │  │  PVC-2 │
└────────┘   └────────┘  └────────┘
```

### StatefulSet

DataBridge uses a StatefulSet for:
- **Stable Network IDs**: Consistent pod naming (databridge-0, databridge-1, ...)
- **Persistent Storage**: Dedicated PVCs for each pod
- **Ordered Deployment**: Sequential pod creation/termination
- **Raft Consensus**: Reliable cluster membership

### Services

- **databridge**: LoadBalancer for external access
- **databridge-headless**: Cluster-internal communication for Raft

## Deployment Options

### Option 1: kubectl apply

```bash
# Deploy all resources
kubectl apply -f namespace.yaml
kubectl apply -f configmap.yaml
kubectl apply -f secret.yaml
kubectl apply -f statefulset.yaml
kubectl apply -f service.yaml
kubectl apply -f ingress.yaml
kubectl apply -f hpa.yaml
kubectl apply -f pdb.yaml
```

### Option 2: Kustomize

```bash
# Deploy using kustomize
kubectl apply -k deploy/kubernetes/

# Or build and apply
kustomize build deploy/kubernetes/ | kubectl apply -f -
```

### Option 3: Helm (if available)

```bash
# Install Helm chart
helm install databridge ./deploy/helm/databridge \
  --namespace databridge \
  --create-namespace \
  --values values.yaml
```

## Configuration

### ConfigMap

Edit `configmap.yaml` to configure application settings:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: databridge-config
  namespace: databridge
data:
  LOG_LEVEL: "info"
  API_PORT: "8080"
  DATA_DIR: "/app/data"
  CLUSTER_ENABLED: "true"
  # Add custom configuration
  MAX_CONCURRENT_TASKS: "100"
  QUEUE_MAX_SIZE: "10000"
```

### Secret

Edit `secret.yaml` for sensitive data:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: databridge-secret
  namespace: databridge
type: Opaque
stringData:
  DB_PASSWORD: "secure-password"
  API_KEY: "your-api-key"
  JWT_SECRET: "your-jwt-secret"
```

### Resource Limits

Adjust in `statefulset.yaml`:

```yaml
resources:
  requests:
    cpu: 500m      # 0.5 CPU cores
    memory: 512Mi  # 512 MB
  limits:
    cpu: 2000m     # 2 CPU cores
    memory: 2Gi    # 2 GB
```

### Storage

Configure PVC size in `statefulset.yaml`:

```yaml
volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: [ "ReadWriteOnce" ]
      storageClassName: fast-ssd  # Your storage class
      resources:
        requests:
          storage: 50Gi  # Adjust size
```

### Ingress

Update `ingress.yaml` with your domain:

```yaml
spec:
  tls:
  - hosts:
    - databridge.yourdomain.com
    secretName: databridge-tls
  rules:
  - host: databridge.yourdomain.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: databridge
            port:
              number: 8080
```

## Scaling

### Manual Scaling

```bash
# Scale to 5 replicas
kubectl scale statefulset databridge -n databridge --replicas=5

# Verify
kubectl get pods -n databridge
```

### Horizontal Pod Autoscaler (HPA)

Deploy HPA for automatic scaling:

```bash
# Apply HPA
kubectl apply -f hpa.yaml

# Check HPA status
kubectl get hpa -n databridge

# Describe HPA
kubectl describe hpa databridge -n databridge
```

HPA configuration (`hpa.yaml`):
- **Min Replicas**: 3
- **Max Replicas**: 10
- **CPU Target**: 70%
- **Memory Target**: 80%

### Vertical Pod Autoscaler (VPA)

For automatic resource adjustment:

```yaml
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: databridge
  namespace: databridge
spec:
  targetRef:
    apiVersion: apps/v1
    kind: StatefulSet
    name: databridge
  updatePolicy:
    updateMode: "Auto"
```

## Monitoring

### Prometheus Integration

Deploy ServiceMonitor for Prometheus Operator:

```bash
# Apply ServiceMonitor
kubectl apply -f servicemonitor.yaml

# Verify metrics
kubectl port-forward -n databridge svc/databridge 8080:8080
curl http://localhost:8080/api/metrics
```

### Grafana Dashboard

Import DataBridge dashboard:

1. Access Grafana
2. Import dashboard from `deploy/grafana/dashboards/databridge.json`
3. Select Prometheus datasource

### Health Checks

DataBridge includes three probe types:

1. **Startup Probe**: Initial startup (max 150s)
2. **Readiness Probe**: Ready for traffic
3. **Liveness Probe**: Still alive

```bash
# Check pod health
kubectl describe pod databridge-0 -n databridge

# Manual health check
kubectl exec -it databridge-0 -n databridge -- \
  curl localhost:8080/api/health
```

### Logs

```bash
# View logs for specific pod
kubectl logs -f databridge-0 -n databridge

# View logs for all pods
kubectl logs -f -l app=databridge -n databridge

# Previous container logs (after restart)
kubectl logs databridge-0 -n databridge --previous

# Stream logs with stern (if installed)
stern databridge -n databridge
```

## High Availability

### Pod Disruption Budget

PDB ensures minimum availability during disruptions:

```bash
# Apply PDB
kubectl apply -f pdb.yaml

# Check PDB
kubectl get pdb -n databridge
```

Configuration (`pdb.yaml`):
- **minAvailable**: 2 (always keep 2 pods running)

### Anti-Affinity

Add to StatefulSet for pod distribution:

```yaml
spec:
  template:
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app
                  operator: In
                  values:
                  - databridge
              topologyKey: kubernetes.io/hostname
```

### Multi-Zone Deployment

```yaml
spec:
  template:
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
              - key: app
                operator: In
                values:
                - databridge
            topologyKey: topology.kubernetes.io/zone
```

## Backup and Recovery

### Backup PVCs

```bash
# Create VolumeSnapshot
kubectl apply -f - <<EOF
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: databridge-snapshot-$(date +%Y%m%d-%H%M%S)
  namespace: databridge
spec:
  volumeSnapshotClassName: csi-snapclass
  source:
    persistentVolumeClaimName: data-databridge-0
EOF
```

### Restore from Snapshot

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: data-databridge-0-restored
  namespace: databridge
spec:
  dataSource:
    name: databridge-snapshot-20240101-120000
    kind: VolumeSnapshot
    apiGroup: snapshot.storage.k8s.io
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
```

## Updates and Rollbacks

### Rolling Update

StatefulSet updates pods sequentially:

```bash
# Update image
kubectl set image statefulset/databridge \
  databridge=databridge:v1.1.0 \
  -n databridge

# Watch update
kubectl rollout status statefulset/databridge -n databridge

# Check update history
kubectl rollout history statefulset/databridge -n databridge
```

### Rollback

```bash
# Rollback to previous version
kubectl rollout undo statefulset/databridge -n databridge

# Rollback to specific revision
kubectl rollout undo statefulset/databridge -n databridge --to-revision=2
```

## Troubleshooting

### Pod Not Starting

```bash
# Check pod status
kubectl get pods -n databridge

# Describe pod for events
kubectl describe pod databridge-0 -n databridge

# Check logs
kubectl logs databridge-0 -n databridge

# Common issues:
# 1. Image pull failures - check image name/tag
# 2. Resource limits - check node capacity
# 3. PVC binding - check storage class
# 4. ConfigMap/Secret missing - verify resources exist
```

### Cluster Connection Issues

```bash
# Check Raft connectivity
kubectl exec -it databridge-0 -n databridge -- \
  curl http://databridge-1.databridge-headless.databridge.svc.cluster.local:8080/api/cluster/status

# Check DNS resolution
kubectl exec -it databridge-0 -n databridge -- \
  nslookup databridge-headless.databridge.svc.cluster.local

# Check network policies
kubectl get networkpolicies -n databridge

# Verify service endpoints
kubectl get endpoints databridge-headless -n databridge
```

### Performance Issues

```bash
# Check resource usage
kubectl top pods -n databridge

# Check node capacity
kubectl describe nodes

# Check PVC performance
kubectl describe pvc -n databridge

# Enable debug logging
kubectl set env statefulset/databridge \
  LOG_LEVEL=debug -n databridge
```

### Storage Issues

```bash
# Check PVC status
kubectl get pvc -n databridge

# Describe PVC
kubectl describe pvc data-databridge-0 -n databridge

# Check PV
kubectl get pv

# Verify storage class
kubectl get storageclass
```

## Security Best Practices

### RBAC

Create ServiceAccount with minimal permissions:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: databridge
  namespace: databridge
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: databridge
  namespace: databridge
rules:
- apiGroups: [""]
  resources: ["pods", "services"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: databridge
  namespace: databridge
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: databridge
subjects:
- kind: ServiceAccount
  name: databridge
  namespace: databridge
```

### Network Policies

Restrict traffic:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: databridge
  namespace: databridge
spec:
  podSelector:
    matchLabels:
      app: databridge
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: databridge
    ports:
    - protocol: TCP
      port: 8080
    - protocol: TCP
      port: 9000
  egress:
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 53
    - protocol: UDP
      port: 53
```

### Security Context

Already configured in `statefulset.yaml`:
- **runAsUser**: 1000 (non-root)
- **runAsGroup**: 1000
- **fsGroup**: 1000
- **readOnlyRootFilesystem**: Optional

## Production Checklist

- [ ] Update image tag to specific version
- [ ] Configure appropriate resource limits
- [ ] Set up persistent storage with backups
- [ ] Enable HPA for autoscaling
- [ ] Configure PDB for high availability
- [ ] Set up monitoring with Prometheus/Grafana
- [ ] Configure ingress with TLS
- [ ] Implement network policies
- [ ] Create RBAC policies
- [ ] Test backup and restore procedures
- [ ] Document runbooks for common issues
- [ ] Set up alerting for critical events

## Additional Resources

- [Kubernetes Documentation](https://kubernetes.io/docs/)
- [StatefulSet Guide](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/)
- [Prometheus Operator](https://prometheus-operator.dev/)
- [Cert-Manager](https://cert-manager.io/)
