package workflow

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

func getRunDetailHandler(re *core.RequestEvent) error {
	detail, err := buildRunDetail(re.App, re.Request.PathValue("id"))
	if err != nil {
		return re.JSON(http.StatusNotFound, map[string]string{"error": "run not found"})
	}
	return re.JSON(http.StatusOK, detail)
}
