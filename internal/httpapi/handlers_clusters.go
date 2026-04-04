package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"atlas/internal/app"

	"github.com/gorilla/mux"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

// ============================================
// SINGLE CLUSTER INFO (Legacy)
// ============================================

// getClusterInfo returns basic cluster information including namespaces.
// Used in single-cluster mode. For multi-cluster, use getClusterInfoHandler.
// GET /api/cluster
func getClusterInfo(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		namespaces, err := application.K8sClient.Clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
		if err != nil {
			application.Logger.Error("Failed to list namespaces", "error", err)
			http.Error(w, "Failed to retrieve cluster information", http.StatusInternalServerError)
			return
		}

		var nsList []string
		for _, ns := range namespaces.Items {
			nsList = append(nsList, ns.Name)
		}

		config, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
		if err != nil {
			application.Logger.Warn("Failed to load kubeconfig", "error", err)
		}

		currentContext := ""
		clusterName := ""
		if config != nil {
			currentContext = config.CurrentContext
			if ctx, ok := config.Contexts[currentContext]; ok {
				clusterName = ctx.Cluster
			}
		}

		application.Logger.Info("Cluster info retrieved",
			"cluster", clusterName,
			"context", currentContext,
			"namespace_count", len(nsList))

		json.NewEncoder(w).Encode(map[string]interface{}{
			"cluster_name": clusterName,
			"context_name": currentContext,
			"namespaces":   nsList,
		})
	}
}

// ============================================
// MULTI-CLUSTER MANAGEMENT
// ============================================

// getClustersHandler returns a list of all available clusters.
// GET /api/clusters
func getClustersHandler(app *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if app.ClusterManager == nil {
			http.Error(w, "Multi-cluster mode not enabled", http.StatusNotImplemented)
			return
		}

		clusters := app.ClusterManager.ListClusters()

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(clusters); err != nil {
			app.Logger.Error("Failed to encode clusters", slog.Any("error", err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}
}

// getCurrentClusterHandler returns the currently selected cluster for the user.
// GET /api/cluster/current
func getCurrentClusterHandler(app *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if app.ClusterManager == nil {
			// Single cluster mode - return the single cluster info
			response := map[string]string{
				"cluster_id": "default",
				"mode":       "single-cluster",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}

		// Get user ID from session/auth - for now use a default
		// In production, extract from JWT or session
		userID := getUserID(r)

		clusterID, ok := app.ClusterManager.GetUserCluster(userID)
		if !ok {
			clusterID = app.ClusterManager.GetDefaultCluster()
		}

		response := map[string]string{
			"cluster_id": clusterID,
			"mode":       "multi-cluster",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// switchClusterHandler switches the active cluster for the current user.
// POST /api/cluster/switch
func switchClusterHandler(app *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if app.ClusterManager == nil {
			http.Error(w, "Multi-cluster mode not enabled", http.StatusNotImplemented)
			return
		}

		var req struct {
			ClusterID string `json:"cluster_id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.ClusterID == "" {
			http.Error(w, "cluster_id is required", http.StatusBadRequest)
			return
		}

		// Verify cluster exists
		_, err := app.ClusterManager.GetCluster(req.ClusterID)
		if err != nil {
			http.Error(w, "Cluster not found", http.StatusNotFound)
			return
		}

		// Get user ID from session/auth
		userID := getUserID(r)

		// Set user's cluster
		if err := app.ClusterManager.SetUserCluster(userID, req.ClusterID); err != nil {
			app.Logger.Error("Failed to set user cluster",
				slog.String("user_id", userID),
				slog.String("cluster_id", req.ClusterID),
				slog.Any("error", err))
			http.Error(w, "Failed to switch cluster", http.StatusInternalServerError)
			return
		}

		app.Logger.Info("Cluster switched",
			slog.String("user_id", userID),
			slog.String("cluster_id", req.ClusterID))

		response := map[string]string{
			"status":     "success",
			"cluster_id": req.ClusterID,
			"message":    "Cluster switched successfully",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// getClusterHealthHandler returns health status of all clusters.
// GET /api/clusters/health
func getClusterHealthHandler(app *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if app.ClusterManager == nil {
			http.Error(w, "Multi-cluster mode not enabled", http.StatusNotImplemented)
			return
		}

		healthResults := app.ClusterManager.CheckAllHealth()

		results := make(map[string]map[string]interface{})
		for clusterID, err := range healthResults {
			status := "healthy"
			errorMsg := ""
			if err != nil {
				status = "unhealthy"
				errorMsg = err.Error()
			}

			results[clusterID] = map[string]interface{}{
				"status": status,
				"error":  errorMsg,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

// getClusterInfoHandler returns detailed information about a specific cluster.
// GET /api/cluster/{id}
func getClusterInfoHandler(app *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if app.ClusterManager == nil {
			http.Error(w, "Multi-cluster mode not enabled", http.StatusNotImplemented)
			return
		}

		vars := mux.Vars(r)
		clusterID := vars["id"]

		if clusterID == "" {
			http.Error(w, "cluster_id is required", http.StatusBadRequest)
			return
		}

		info, err := app.ClusterManager.GetClusterInfo(clusterID)
		if err != nil {
			http.Error(w, "Cluster not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
	}
}

// getCacheStatsHandler returns cache statistics.
// GET /api/cache/stats
func getCacheStatsHandler(app *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats := app.Cache.Stats()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}
}

// getUserID extracts user ID from request.
// In production, this would decode JWT or session token.
// For now, use a simple header or default value.
func getUserID(r *http.Request) string {
	// Try to get from header
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		return userID
	}

	// Try to get from cookie
	if cookie, err := r.Cookie("user_id"); err == nil {
		return cookie.Value
	}

	// Default to IP address as user identifier
	return r.RemoteAddr
}
