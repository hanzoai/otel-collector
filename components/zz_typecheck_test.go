package components

import (
	"testing"

	"go.opentelemetry.io/collector/component"
)

// The pipelines in universe/infra/k8s name these. A build that does not
// register one of them fails to start.
func TestConfiguredComponentsAreRegistered(t *testing.T) {
	f, err := Components()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		kind string
		want []string
	}{
		{"extension", []string{"o11y_health_check"}},
		{"receiver", []string{"zap", "otlp", "filelog"}},
		{"processor", []string{"batch", "memory_limiter", "filter", "k8sattributes", "resource", "resourcedetection", "o11yspanmetrics"}},
		{"exporter", []string{"datastoretraces", "datastorelogsexporter", "o11ydatastoremetrics", "prometheus", "zap"}},
	} {
		for _, name := range c.want {
			var ok bool
			switch c.kind {
			case "extension":
				_, ok = f.Extensions[mustType(t, name)]
			case "receiver":
				_, ok = f.Receivers[mustType(t, name)]
			case "processor":
				_, ok = f.Processors[mustType(t, name)]
			case "exporter":
				_, ok = f.Exporters[mustType(t, name)]
			}
			if !ok {
				t.Errorf("%s %q is named by a pipeline but not registered", c.kind, name)
			}
		}
	}
}

func mustType(t *testing.T, s string) component.Type {
	t.Helper()
	typ, err := component.NewType(s)
	if err != nil {
		t.Fatalf("bad type %q: %v", s, err)
	}
	return typ
}
