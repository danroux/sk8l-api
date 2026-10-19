// Package dashboard provides types and functions for generating Grafana dashboard
// configurations from sk8l Prometheus metrics. It produces panel definitions
// that can be imported into Grafana via the sk8l API.
package dashboard

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

type DataSource struct {
	UID, Type string
}

type Target struct {
	DataSource         DataSource
	Expr, LegendFormat string
}

type GridPos struct {
	H uint16 `json:"h"`
	W uint16 `json:"w"`
	X uint16 `json:"x"`
	Y uint16 `json:"y"`
}

type Option struct {
	Calcs string
}

type Override struct {
	ID      string `json:"id"`
	Options string `json:"options"`
}

type Panel struct {
	DataSource DataSource
	Override   Override
	Options    Option
	Title      string
	Type       string
	Repeat     string
	Targets    []*Target
	GridPos    GridPos `json:"gridPos"`
}

var (
	dataSource = DataSource{
		Type: "prometheus",
		UID:  "${DS_PROMETHEUS}",
	}

	failureMetricRe = regexp.MustCompile(`failure_total$`)
)

type Generator struct {
	MetricPrefix string
	Namespace    string
	Statuses     []string
}

func NewGenerator(metricPrefix, namespace string, statuses []string) *Generator {
	return &Generator{
		MetricPrefix: metricPrefix,
		Namespace:    namespace,
		Statuses:     statuses,
	}
}

func (g *Generator) GeneratePanels(cronjobsMap *sync.Map) []Panel {
	var totalsMetrics = make([]*Target, 0, len(g.Statuses))
	for _, status := range g.Statuses {
		t := &Target{
			Expr:         fmt.Sprintf(`sk8l_cronjobs_count{namespace="%s", status="%s"}`, g.Namespace, status),
			LegendFormat: status,
			DataSource:   dataSource,
		}
		totalsMetrics = append(totalsMetrics, t)
	}

	panels := []Panel{
		{
			Type:       "row",
			Title:      fmt.Sprintf("sk8l: %s overview", g.Namespace),
			DataSource: dataSource,
			GridPos: GridPos{
				X: 0,
				Y: 0,
				H: 1,
				W: 24,
			},
			Targets: make([]*Target, 0),
		},
		g.cronjobsTotalsTimeseries(totalsMetrics),
		g.totalsBarGaugePanel(),
		g.allStateTimelines(cronjobsMap),
		g.allStatusHistory(cronjobsMap),
	}

	cronJobRowPanels := g.generateCronJobRowPanels(cronjobsMap)
	panels = append(panels, cronJobRowPanels...)

	return panels
}

func (g *Generator) generateCronJobRowPanels(cronjobsMap *sync.Map) []Panel {
	cronJobRowPanels := make([]Panel, 0)
	cronjobsMap.Range(g.individualPanelsGenerator(&cronJobRowPanels))
	return cronJobRowPanels
}

func (g *Generator) individualPanelsGenerator(cronJobRowPanels *[]Panel) func(key, value any) bool {
	return func(key, value any) bool {
		if len(*cronJobRowPanels) > 0 {
			return true
		}

		i := len(*cronJobRowPanels)
		var rowY, panelY, failureY uint16
		rowY = uint16((i + 1) * 10)
		panelY = rowY + 1
		failureY = panelY + 8

		row := Panel{
			Type:  "row",
			Title: "${cronjob}",
			GridPos: GridPos{
				X: 0,
				Y: rowY,
				H: 1,
				W: 24,
			},
			Targets:    make([]*Target, 0),
			DataSource: dataSource,
			Repeat:     "cronjob",
		}
		*cronJobRowPanels = append(*cronJobRowPanels, row)

		// 1. Completion / Failure totals panel
		cronjobTotals := []*Target{
			{
				Expr:         fmt.Sprintf(`sk8l_cronjob_jobs_count{namespace="%s", cronjob=~"${cronjob}", status="succeeded"}`, g.Namespace),
				LegendFormat: "succeeded",
				DataSource:   dataSource,
			},
			{
				Expr:         fmt.Sprintf(`sk8l_cronjob_jobs_count{namespace="%s", cronjob=~"${cronjob}", status="failed"}`, g.Namespace),
				LegendFormat: "failed",
				DataSource:   dataSource,
			},
		}

		*cronJobRowPanels = append(*cronJobRowPanels, Panel{
			Title:      "${cronjob}: completion / failure totals",
			DataSource: dataSource,
			GridPos:    GridPos{X: 0, Y: panelY, H: 8, W: 12},
			Targets:    cronjobTotals,
			Options:    Option{Calcs: "last"},
		})

		// 2. Job duration panel
		cronjobDurations := []*Target{
			{
				Expr:         fmt.Sprintf(`sk8l_cronjob_last_run_duration_seconds{namespace="%s", cronjob=~"${cronjob}"}`, g.Namespace),
				LegendFormat: "duration (s)",
				DataSource:   dataSource,
			},
		}

		*cronJobRowPanels = append(*cronJobRowPanels, Panel{
			Title:      "${cronjob}: jobs duration",
			DataSource: dataSource,
			GridPos:    GridPos{X: 12, Y: panelY, H: 8, W: 12},
			Targets:    cronjobDurations,
			Options:    Option{Calcs: "max"},
		})

		// 3. State timeline panel
		*cronJobRowPanels = append(*cronJobRowPanels, Panel{
			Title:      "${cronjob}: state timeline",
			Type:       "state-timeline",
			DataSource: dataSource,
			GridPos:    GridPos{X: 0, Y: failureY, H: 8, W: 12},
			Targets: []*Target{
				{
					Expr:         fmt.Sprintf(`sk8l_cronjob_last_run_status{namespace="%s", cronjob=~"${cronjob}", status="failed"}`, g.Namespace),
					LegendFormat: "failure status",
					DataSource:   dataSource,
				},
			},
			Options: Option{Calcs: "last"},
		})

		return true
	}
}

func (g *Generator) cronjobsTotalsTimeseries(totalsMetrics []*Target) Panel {
	return Panel{
		Title:      "completed / registered / failed cronjobs totals",
		Type:       "timeseries",
		DataSource: dataSource,
		GridPos:    GridPos{X: 0, Y: 1, H: 8, W: 12},
		Targets:    totalsMetrics,
		Options:    Option{Calcs: "last"},
	}
}

func (g *Generator) totalsBarGaugePanel() Panel {
	totalsTargets := make([]*Target, 0, len(g.Statuses))
	for _, status := range g.Statuses {
		legendFmt := strings.ReplaceAll(status, "_", " ")
		totalsTargets = append(totalsTargets, &Target{
			Expr:         fmt.Sprintf(`sk8l_cronjobs_count{namespace="%s", status="%s"}`, g.Namespace, status),
			LegendFormat: legendFmt,
			DataSource:   dataSource,
		})
	}

	return Panel{
		Type:       "bargauge",
		Title:      fmt.Sprintf("sk8l: %s totals", g.Namespace),
		DataSource: dataSource,
		GridPos:    GridPos{X: 12, Y: 1, H: 8, W: 12},
		Targets:    totalsTargets,
		Options:    Option{Calcs: "lastNotNull"},
		Override:   Override{ID: "byName", Options: "failing"},
	}
}

func (g *Generator) allStatusHistory(cronjobsMap *sync.Map) Panel {
	failureTargets := g.collectStatusTargets(cronjobsMap)
	return Panel{
		Title:      "status history",
		Type:       "status-history",
		DataSource: dataSource,
		GridPos:    GridPos{X: 12, Y: 9, H: 8, W: 12},
		Targets:    failureTargets,
	}
}

func (g *Generator) allStateTimelines(cronjobsMap *sync.Map) Panel {
	failureTargets := g.collectStatusTargets(cronjobsMap)
	return Panel{
		Title:      "status timeline",
		Type:       "state-timeline",
		DataSource: dataSource,
		GridPos:    GridPos{X: 0, Y: 9, H: 8, W: 12},
		Targets:    failureTargets,
		Options:    Option{Calcs: "last"},
	}
}

func (g *Generator) collectStatusTargets(cronjobsMap *sync.Map) []*Target {
	targets := make([]*Target, 0)
	if cronjobsMap == nil {
		return targets
	}

	cronjobsMap.Range(func(key, _ any) bool {
		cronjobName, ok := key.(string)
		if !ok {
			return true
		}
		targets = append(targets, &Target{
			Expr:         fmt.Sprintf(`sk8l_cronjob_last_run_status{namespace="%s", cronjob="%s", status="failed"}`, g.Namespace, cronjobName),
			LegendFormat: cronjobName,
			DataSource:   dataSource,
		})
		return true
	})

	if len(targets) == 0 {
		targets = append(targets, &Target{
			Expr:         fmt.Sprintf(`sk8l_cronjob_last_run_status{namespace="%s", status="failed"}`, g.Namespace),
			LegendFormat: "{{cronjob}}",
			DataSource:   dataSource,
		})
	}

	return targets
}

func failureLegendFmt(metricName string) string {
	shorterLegend := strings.TrimSuffix(failureMetricRe.String(), "$")
	legendFmt := strings.TrimSuffix(metricName, fmt.Sprintf("_%s", shorterLegend))
	return legendFmt
}
