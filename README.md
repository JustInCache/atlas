# Atlas - Kubernetes Cluster Dashboard 

**Atlas** is a fast, lightweight, read-only Kubernetes monitoring dashboard for SREs and developers. Named after the Titan who holds up the celestial sphere, Atlas provides a comprehensive view of your entire Kubernetes infrastructure.

![Go Version](https://img.shields.io/badge/Go-1.21-00ADD8?logo=go)
![Kubernetes](https://img.shields.io/badge/Kubernetes-0.28-326CE5?logo=kubernetes)

## ✨ Key Features

- **Read-Only & Safe** - Zero write operations, perfect for production monitoring
- **High Performance** - Optimized for 50+ concurrent users with intelligent caching
- **Multi-Cluster Support** - Manage and switch between multiple Kubernetes clusters (with Redis)
- **Comprehensive Views** - Pods, Deployments, Services, Ingresses, ConfigMaps, Secrets, PV/PVC, CRDs
- **Resource Relationships** - Track dependencies and connections between resources
- **Health Dashboard** - Real-time cluster health with node monitoring and events
- **Modern UI** - Dark theme with collapsible sections and responsive design

## �️ Architecture Flow

Atlas follows a lightweight, read-only architecture optimized for performance and scalability:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              USER BROWSER                               │
│  ┌──────────────┐  ┌──────────────┐   ┌──────────────┐                  │
│  │   Dashboard  │  │   Explorer   │   │ Detail Panel │                  │
│  │   (Vue.js)   │  │ (Vanilla JS) │   │  (Dynamic)   │                  │
│  └──────┬───────┘  └──────┬───────┘   └──────┬───────┘                  │
│         │                 │                  │                          │
│         └─────────────────┴──────────────────┘                          │
│                           │                                             │
│                    Auto-Refresh (30s-180s)                              │
│                    Priority-Based Polling                               │
└───────────────────────────┼─────────────────────────────────────────────┘
                            │
                     HTTP REST API
                            │
┌───────────────────────────▼──────────────────────────────────────────────┐
│                        GO HTTP SERVER (Port 8080)                        │
│  ┌──────────────────────────────────────────────────────────────────┐    │
│  │                      Gorilla Mux Router                          │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐               │    │
│  │  │   /api/pods │  │ /api/deploy │  │/api/services│   +14 more    │    │
│  │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘               │    │
│  └─────────┼─────────────────┼─────────────────┼────────────────────┘    │
│            │                 │                 │                         │
│  ┌─────────▼─────────────────▼─────────────────▼────────────────────┐    │
│  │                     HTTP API Handlers                            │    │
│  │   • Rate Limiting (100 req/s per user)                           │    │
│  │   • Session Management (Cookie-based)                            │    │
│  │   • GetOrFetch Pattern (Cache-first).                             │    │
│  └─────────┬────────────────────────────────────────────────────────┘    │
└────────────┼─────────────────────────────────────────────────────────────┘
             │
             │ Cache Lookup
             ▼
┌────────────────────────────────────────────────────────────────────────┐
│                         CACHING LAYER                                  │
│  ┌──────────────────────────────────────────────────────────────┐      │
│  │   In-Memory Cache (sync.RWMutex + singleflight)               │      │
│  │   • TTL: 5-60s (resource-dependent)                          │      │
│  │   • Cache Hit Rate: 70-80%                                   │      │
│  │   • ResourceVersion Tracking                                 │      │
│  │   • Request Deduplication (~95% reduction)                   │      │
│  └──────────────────────────────────────────────────────────────┘      │
│                                                                        │
│  ┌──────────────────────────────────────────────────────────────┐      │
│  │   Redis Cache (Multi-Cluster Mode)                           │      │
│  │   • Shared cache across Atlas instances                      │      │
│  │   • Cluster-scoped namespacing                               │      │
│  │   • Automatic failover to in-memory                          │      │
│  └──────────────────────────────────────────────────────────────┘      │
└────────────┬───────────────────────────────────────────────────────────┘
             │
             │ Cache Miss → Fetch from K8s
             ▼
┌────────────────────────────────────────────────────────────────────────┐
│                     KUBERNETES CLIENT-GO                               │
│  ┌──────────────────────────────────────────────────────────────┐      │
│  │   Connection Pool (200 max idle, 50 QPS burst 100)           │      │
│  │   • API Server Load Balancing                                │      │
│  │   • Automatic Retry with Backoff                             │      │
│  │   • Read-Only Operations (List/Get only)                     │      │
│  └──────────────────────────────────────────────────────────────┘      │
└────────────┬───────────────────────────────────────────────────────────┘
             │
             │ GET/LIST API Calls
             ▼
┌────────────────────────────────────────────────────────────────────────┐
│                    KUBERNETES API SERVER                               │
│  ┌──────────────────────────────────────────────────────────────┐      │
│  │   Core Resources: Pods, Services, Nodes, Events              │      │
│  │   Apps: Deployments, StatefulSets, DaemonSets                │      │
│  │   Batch: Jobs, CronJobs                                      │      │
│  │   Networking: Ingresses, Endpoints                           │      │
│  │   Storage: PVs, PVCs, StorageClasses                         │      │
│  │   Config: ConfigMaps, Secrets                                │      │
│  │   Autoscaling: HPAs, PDBs                                    │      │
│  │   Custom: CRDs                                               │      │
│  └──────────────────────────────────────────────────────────────┘      │
└────────────────────────────────────────────────────────────────────────┘
```

### Request Flow Example

1. **User Action**: User opens "Pods" tab in browser
2. **API Request**: `GET /api/pods/default` sent to Go server
3. **Cache Check**: Handler checks cache with key `cluster:pods:default`
4. **Cache Hit**: Return cached data (TTL: 30s for pods)
5. **Cache Miss**: 
   - Singleflight ensures only 1 K8s API call for concurrent requests
   - Fetch from Kubernetes API: `client.CoreV1().Pods("default").List()`
   - Store in cache with ResourceVersion tracking
   - Return data to browser
6. **Auto-Refresh**: Browser polls every 30s (HIGH priority for pods)
7. **Next Request**: Served from cache (70%+ hit rate)

### Performance Characteristics

| Component | Metric | Value |
|-----------|--------|-------|
| **Frontend** | Initial Load | < 2s (critical data) |
| **Frontend** | Full Dashboard | 2-3s |
| **Backend** | Cache Hit Response | < 10ms |
| **Backend** | Cache Miss Response | 50-200ms |
| **Backend** | Concurrent Users | 50-100 (optimized) |
| **Caching** | Hit Rate | 70-80% |
| **Caching** | Request Deduplication | ~95% reduction |
| **K8s API** | Calls Per Second | 1-2 req/s (with cache) |

## �🌐 Multi-Cluster Support (New!)

Atlas now supports multi-cluster deployments with Redis-backed caching:

- **Switch between clusters** from the UI dropdown
- **Shared cache** across multiple Atlas instances
- **Horizontal scaling** with Redis
- **Per-cluster namespacing** for cache isolation


## Namespace Specific Dashboard 
<img width="1725" height="852" alt="image" src="https://github.com/user-attachments/assets/13489745-c10e-48bc-ba65-a6b5e44696aa" />

## Resources Relationship Explorer
<img width="1725" height="852" alt="image" src="https://github.com/user-attachments/assets/eb960935-93f3-4830-b7ff-06e791381b89" />

## Release View 
<img width="1725" height="852" alt="image" src="https://github.com/user-attachments/assets/8ddc65ae-7053-41af-99a5-e6eec767abdb" />

## Easy Navigation between resources.
<img width="1725" height="852" alt="image" src="https://github.com/user-attachments/assets/d91b4147-502c-4a8a-8de8-d93f425cc38a" />



## 🚀 Quick Start

### Prerequisites
- Go 1.21+
- Access to Kubernetes cluster
- `kubectl` configured

### Installation

\`\`\`bash
git clone https://github.com/Fanatic-zer0/atlas.git
cd atlas
make build
./bin/atlas
\`\`\`

Access dashboard at `http://localhost:8080`

### Configuration

**Environment Variables:**
- `PORT` - Server port (default: 8080)
- `KUBECONFIG` - Path to kubeconfig (default: ~/.kube/config)

## 📡 Main API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/cluster` | Cluster information |
| `GET /api/health/{namespace}` | Health dashboard with nodes & events |
| `GET /api/resources/{namespace}` | All resources with filtering |
| `GET /api/ingresses/{namespace}` | Ingresses with Kong plugins |
| `GET /api/services/{namespace}` | Services with endpoints |
| `GET /api/pods/{namespace}` | Pods with status |
| `GET /api/deployments/{namespace}` | Deployments with replicas |
| `GET /api/configmaps/{namespace}` | ConfigMaps |
| `GET /api/secrets/{namespace}` | Secrets metadata |
| `GET /api/pvpvc/{namespace}` | PV/PVC with pod usage |
| `GET /api/crds` | Custom Resource Definitions |
| `GET /api/cache/stats` | Cache statistics |
| `POST /api/cache/clear` | Clear cache |

## 🎨 UI Tabs

| Tab | Features |
|-----|----------|
| **Health** | Resource summary, pod/deployment/service health, cluster events |
| **Cluster** | Nodes with system info, conditions, and addresses |
| **Resources** | Unified view with type filtering and search |
| **Ingresses** | LoadBalancer IPs, Kong plugins, routing rules |
| **Services** | Endpoints, ports, selectors |
| **Pods** | Status, containers, restarts, IP, node |
| **Deployments** | Replicas, images, resources |
| **ConfigMaps/Secrets** | Keys, usage tracking |
| **PV/PVC** | Storage types, capacity, pod usage |
| **CRDs** | Versions, scope, categories |

## 🐳 Docker Deployment

**Quick setup:**

```bash
# Start Redis
docker run -d --name atlas-redis -p 6379:6379 redis:7-alpine

# Run Atlas with multi-cluster mode
export CACHE_TYPE=redis
export REDIS_ADDR=localhost:6379
export MULTI_CLUSTER=true
./bin/atlas
```

### Build

```bash
# Single-arch (current platform)
make docker-build

# Multi-arch (linux/amd64 + linux/arm64) — requires a registry
make docker-buildx IMAGE=yourrepo/atlas:latest

# Multi-arch local test build (current arch only, no registry needed)
make docker-buildx-load
```


### Run with a kubeconfig file

Mount your local kubeconfig into the container and point `KUBECONFIG` at it:

```bash
make docker-run
```

Which expands to:

```bash
docker run -p 8080:8080 \
  -v ~/.kube/config:/home/appuser/.kube/config:ro \
  -e KUBECONFIG=/home/appuser/.kube/config \
  atlas:latest
```

> **Tip:** If your kubeconfig references local certificate files (e.g. `certificate-authority: /Users/you/...`), use embedded credentials instead (`certificate-authority-data` in base64). EKS, GKE, and AKS kubeconfigs typically do this already.

---

### 3. Authentication

Atlas has **no built-in authentication**. For multi-user access, place an authenticating reverse proxy in front of it. Two common options:

#### Option A — oauth2-proxy (SSO via Google / GitHub / OIDC)

```yaml
# Add to your Ingress annotations (nginx example)
nginx.ingress.kubernetes.io/auth-url: "https://oauth2-proxy.monitoring.svc/oauth2/auth"
nginx.ingress.kubernetes.io/auth-signin: "https://atlas.example.com/oauth2/start?rd=$escaped_request_uri"
```

Deploy [oauth2-proxy](https://github.com/oauth2-proxy/oauth2-proxy) as a sidecar or separate deployment, configured for your identity provider (Google Workspace, GitHub Org, Okta, etc.).

#### Option B — Basic auth via Ingress

```bash
# Create htpasswd secret
htpasswd -c auth admin
kubectl create secret generic atlas-basic-auth --from-file=auth -n monitoring
```

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: atlas
  namespace: monitoring
  annotations:
    nginx.ingress.kubernetes.io/auth-type: basic
    nginx.ingress.kubernetes.io/auth-secret: atlas-basic-auth
    nginx.ingress.kubernetes.io/auth-realm: "Atlas — Kubernetes Dashboard"
spec:
  rules:
  - host: atlas.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: atlas
            port:
              number: 80
```

> For production, Option A (SSO) is strongly preferred — it ties access to your existing identity provider and supports audit logging.

## 🛡️ Security & RBAC

**Read-Only Operations:**
- Only `List()` and `Get()` operations
- No `Create`, `Update`, `Delete`, or `Patch`

**Minimal RBAC:**

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: atlas-viewer
rules:
- apiGroups: ["", "apps", "batch", "networking.k8s.io", "apiextensions.k8s.io"]
  resources: ["*"]
  verbs: ["get", "list"]
```

## 📈 Performance

| Concurrent Users | Response Time | Notes |
|-----------------|---------------|-------|
| 10-30 | <100ms | Excellent |
| 30-50 | 100-200ms | Good |
| 50-70 | 200-300ms | Acceptable |

**Optimizations:**
- ResourceVersion-based change detection (50-70% fewer API calls)
- 30-second intelligent caching
- Concurrent resource fetching (3-6x faster)
- Connection pool tuning (200 max idle, 50 QPS, 100 burst)

## 🔧 Development

```bash
make deps      # Install dependencies
make build     # Build binary
make run       # Run locally
make test      # Run tests
make fmt       # Format code
```

## 📝 License

MIT License

---

**Made with ❤️ for SREs and Platform Engineers**

*Atlas - Supporting your entire Kubernetes world.*
