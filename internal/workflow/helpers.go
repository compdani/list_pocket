package workflow

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func asString(value any, fallback string) string {
	text := strings.TrimSpace(fmt.Sprintf("%v", value))
	if text == "" || text == "<nil>" {
		return fallback
	}
	return text
}

func createWorkflowUUID() string {
	return uuid.NewString()
}
