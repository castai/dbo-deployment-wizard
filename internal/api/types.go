package api

import "time"

// Credentials sources a component's database credentials from exactly
// one of: an existing Secret, or a username/password pair. The
// referenced Secret takes precedence over the pair; when only the
// pair is set, the install creates a Secret from it and passes that
// ref to helm instead.
type Credentials struct {
	SecretName string
	Username   string
	Password   string
}

// Provided reports whether the credentials are sourced from either
// the referenced Secret or a complete username/password pair.
func (c Credentials) Provided() bool {
	return c.SecretName != "" || (c.Username != "" && c.Password != "")
}

// HelmChartVersion describes one published version of a chart.
type HelmChartVersion struct {
	Number  string
	Created time.Time
}

// Component names match the chart's subcharts.
const (
	ComponentDBProxy = "db-proxy"
	ComponentDBAgent = "db-agent"
	ComponentPooling = "pooling"
)

// AllComponents is the curated list the wizard offers. The chart's
// db-optimizer branch is not supported.
var AllComponents = []string{
	ComponentDBAgent,
	ComponentDBProxy,
	ComponentPooling,
}
