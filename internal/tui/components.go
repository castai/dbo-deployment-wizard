package tui

import (
	"strings"

	"github.com/samber/lo"

	"github.com/castai/dbo-deployment-wizard/internal/api"
)

// componentNames are the names the UI shows for the chart's
// components; the chart, the flags and the backend keep the internal
// names.
var componentNames = map[string]string{
	api.ComponentDBAgent: "Performance Advisor",
	api.ComponentDBProxy: "Database Proxy",
	api.ComponentPooling: "Connection Pooling",
}

// componentName is a component's display name; an unknown component
// shows its internal name.
func componentName(component string) string {
	if name, ok := componentNames[component]; ok {
		return name
	}

	return component
}

// componentNameList renders components as a comma-separated list of
// display names.
func componentNameList(components []string) string {
	return strings.Join(lo.Map(components, func(c string, _ int) string { return componentName(c) }), ", ")
}
