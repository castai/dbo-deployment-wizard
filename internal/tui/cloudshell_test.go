package tui

import "testing"

func TestBrowserCloudShell(t *testing.T) {
	tests := map[string]struct {
		env  map[string]string
		want bool
	}{
		"aws cloudshell":         {map[string]string{"AWS_EXECUTION_ENV": "CloudShell"}, true},
		"google cloud shell":     {map[string]string{"CLOUD_SHELL": "true"}, true},
		"azure cloud shell":      {map[string]string{"AZUREPS_HOST_ENVIRONMENT": "cloud-shell/2.0.0"}, true},
		"aws outside cloudshell": {map[string]string{"AWS_EXECUTION_ENV": "ExecutionEnv"}, false},
		"native terminal":        {nil, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			for _, k := range []string{"AWS_EXECUTION_ENV", "CLOUD_SHELL", "AZUREPS_HOST_ENVIRONMENT"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if got := browserCloudShell(); got != tt.want {
				t.Fatalf("browserCloudShell() = %v, want %v", got, tt.want)
			}
		})
	}
}
