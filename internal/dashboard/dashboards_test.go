package dashboard

import (
	"sync"
	"testing"
)

func TestNewGenerator(t *testing.T) {
	gen := NewGenerator("sk8l_default", "default", []string{"registered", "completed"})
	if gen.MetricPrefix != "sk8l_default" {
		t.Errorf("expected MetricPrefix sk8l_default, got %s", gen.MetricPrefix)
	}
	if gen.Namespace != "default" {
		t.Errorf("expected Namespace default, got %s", gen.Namespace)
	}
	if len(gen.Statuses) != 2 {
		t.Errorf("expected 2 Statuses, got %d", len(gen.Statuses))
	}
}

func TestGeneratePanels_EmptyMap(t *testing.T) {
	totalMetrics := []string{
		"registered",
		"completed",
		"running",
		"failing",
	}
	gen := NewGenerator("sk8l_prod", "prod", totalMetrics)
	m := &sync.Map{}

	panels := gen.GeneratePanels(m)
	if len(panels) < 5 {
		t.Errorf("expected at least 5 default overview panels, got %d", len(panels))
	}

	if panels[0].Type != "row" || panels[0].Title != "sk8l: prod overview" {
		t.Errorf("unexpected first panel: %+v", panels[0])
	}
}

func TestGeneratePanels_WithMetrics(t *testing.T) {
	totalMetrics := []string{
		"registered",
		"completed",
	}
	gen := NewGenerator("sk8l_staging", "staging", totalMetrics)
	m := &sync.Map{}
	m.Store("my_cronjob", []string{
		"sk8l_staging_my_cronjob_completion_total",
		"sk8l_staging_my_cronjob_failure_total",
		"sk8l_staging_my_cronjob_duration_seconds",
	})

	panels := gen.GeneratePanels(m)
	if len(panels) <= 5 {
		t.Errorf("expected more than 5 panels with cronjob metrics, got %d", len(panels))
	}

	// Verify failure legend format
	legend := failureLegendFmt("sk8l_staging_my_cronjob_failure_total")
	if legend != "sk8l_staging_my_cronjob" {
		t.Errorf("expected legend sk8l_staging_my_cronjob, got %q", legend)
	}
}
