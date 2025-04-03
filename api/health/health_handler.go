package health

import (
	"net/http"
	"time"

	"ctoup.com/coreapp/internal/version"
	"ctoup.com/coreapp/pkg/core/db"
	"github.com/gin-gonic/gin"
)

type HealthCheckResponse struct {
	Status  string                  `json:"status"`
	Version string                  `json:"version"`
	Notes   []string                `json:"notes,omitempty"`
	Output  string                  `json:"output,omitempty"`
	Checks  map[string]CheckDetails `json:"checks,omitempty"`
}

type CheckDetails struct {
	ComponentType string    `json:"componentType"`
	ComponentName string    `json:"componentName,omitempty"`
	Status        string    `json:"status"`
	Time          time.Time `json:"time"`
	Output        string    `json:"output,omitempty"`
}

type HealthHandler struct {
	store *db.Store
}

// AddHealth implements api.ServerInterface.
func (exh *HealthHandler) GetHealthCheck(c *gin.Context) {
	response := HealthCheckResponse{
		Status:  "pass",
		Version: version.Version,
		Notes:   []string{"health check"},
		Checks:  make(map[string]CheckDetails),
	}

	// Example check for database
	dbCheck := CheckDetails{
		ComponentType: "database",
		ComponentName: "main-db",
		Status:        "pass",
		Time:          time.Now(),
	}
	response.Checks["database"] = dbCheck

	// Example check for external API
	apiCheck := CheckDetails{
		ComponentType: "external",
		ComponentName: "weather-api",
		Status:        "pass",
		Time:          time.Now(),
	}
	response.Checks["external-api"] = apiCheck

	// If any check fails, set the overall status to fail
	for _, check := range response.Checks {
		if check.Status != "pass" {
			response.Status = "fail"
			break
		}
	}

	if response.Status == "pass" {
		c.JSON(http.StatusOK, response)
	} else {
		c.JSON(http.StatusServiceUnavailable, response)
	}
}

func NewHealthHandler(store *db.Store) *HealthHandler {
	return &HealthHandler{store: store}
}
