package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"atlas/internal/app"

	"github.com/gorilla/mux"
)

// corsMiddleware handles Cross-Origin Resource Sharing (CORS)
func corsMiddleware() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set CORS headers
			origin := r.Header.Get("Origin")
			if origin == "" {
				origin = "*"
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token, X-Requested-With")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "86400")

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// loggingMiddleware logs all HTTP requests
func loggingMiddleware(logger *slog.Logger) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			logger.Info("HTTP Request",
				"method", r.Method,
				"path", r.URL.Path,
				"remote", r.RemoteAddr,
			)
			next.ServeHTTP(w, r)
			logger.Info("HTTP Response",
				"method", r.Method,
				"path", r.URL.Path,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

func SetupRoutes(application *app.App) *mux.Router {
	r := mux.NewRouter()
	// Add middleware (order matters!)
	// 1. CORS - must be first to handle preflight requests
	r.Use(corsMiddleware())
	// 2. Session - sets atlas_session cookie for user identification
	r.Use(sessionMiddleware())
	// 3. Logging - logs all requests
	r.Use(loggingMiddleware(application.Logger))
	// 4. Rate limiting - uses atlas_session cookie (per-user, not per-IP)
	r.Use(rateLimitMiddleware())
	// Serve static files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./ui"))))
	r.HandleFunc("/", serveIndex)
	// Health check endpoints
	r.HandleFunc("/healthz", healthCheck(application)).Methods("GET", "HEAD")
	r.HandleFunc("/readyz", readinessCheck(application)).Methods("GET", "HEAD")

	// API routes
	r.HandleFunc("/api/cluster", getClusterInfo(application)).Methods("GET")
	r.HandleFunc("/api/pvpvc/{namespace}", getPVPVC(application)).Methods("GET")
	r.HandleFunc("/api/resources/{namespace}", getAllResources(application)).Methods("GET")
	r.HandleFunc("/api/resource/{type}/{namespace}/{name}", getResourceDetails(application)).Methods("GET")
	r.HandleFunc("/api/ingresses/{namespace}", getIngresses(application)).Methods("GET")
	r.HandleFunc("/api/services/{namespace}", getServices(application)).Methods("GET")
	r.HandleFunc("/api/pods/{namespace}", getPods(application)).Methods("GET")
	r.HandleFunc("/api/deployments/{namespace}", getDeployments(application)).Methods("GET")
	r.HandleFunc("/api/health/{namespace}", getHealth(application)).Methods("GET")
	r.HandleFunc("/api/releases/{namespace}", getReleases(application)).Methods("GET")
	r.HandleFunc("/api/releases/{namespace}/{deployment}/history", getDeploymentHistory(application)).Methods("GET")
	r.HandleFunc("/api/crds", getCRDs(application)).Methods("GET")
	r.HandleFunc("/api/configmaps/{namespace}", getConfigMaps(application)).Methods("GET")
	r.HandleFunc("/api/secrets/{namespace}", getSecrets(application)).Methods("GET")
	r.HandleFunc("/api/relationships/{namespace}", getResourceRelationships(application)).Methods("GET")
	r.HandleFunc("/api/cronjobs/{namespace}", getCronJobsAndJobs(application)).Methods("GET")
	r.HandleFunc("/api/statefulsets/{namespace}", getStatefulSets(application)).Methods("GET")
	r.HandleFunc("/api/daemonsets/{namespace}", getDaemonSets(application)).Methods("GET")
	r.HandleFunc("/api/jobs/{namespace}", getJobs(application)).Methods("GET")
	r.HandleFunc("/api/endpoints/{namespace}", getEndpoints(application)).Methods("GET")
	r.HandleFunc("/api/storageclasses", getStorageClasses(application)).Methods("GET")
	r.HandleFunc("/api/hpas/{namespace}", getHPAs(application)).Methods("GET")
	r.HandleFunc("/api/pdbs/{namespace}", getPDBs(application)).Methods("GET")
	// Note: Cache stats available at /api/cache/stats for monitoring
	r.HandleFunc("/api/cache/stats", getCacheStats(application)).Methods("GET")

	// Multi-cluster management endpoints
	r.HandleFunc("/api/clusters", getClustersHandler(application)).Methods("GET")
	r.HandleFunc("/api/cluster/current", getCurrentClusterHandler(application)).Methods("GET")
	r.HandleFunc("/api/cluster/switch", switchClusterHandler(application)).Methods("POST")
	r.HandleFunc("/api/cluster/{id}", getClusterInfoHandler(application)).Methods("GET")
	r.HandleFunc("/api/clusters/health", getClusterHealthHandler(application)).Methods("GET")

	return r
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./ui/index.html")
}
