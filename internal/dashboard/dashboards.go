// Package dashboard provides types and functions for generating Grafana dashboard
// configurations from sk8l Prometheus metrics. It produces panel definitions
// that can be imported into Grafana via the sk8l API.
package dashboard

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
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

	durationRe      = regexp.MustCompile(`duration_seconds$`)
	failureMetricRe = regexp.MustCompile(`failure_total$`)
)

type Generator struct {
	MetricPrefix     string
	Namespace        string
	TotalMetricNames []string
}

func NewGenerator(metricPrefix, namespace string, totalMetricNames []string) *Generator {
	return &Generator{
		MetricPrefix:     metricPrefix,
		Namespace:        namespace,
		TotalMetricNames: totalMetricNames,
	}
}

func (g *Generator) GeneratePanels(metricsNames *sync.Map) []Panel {
	var totalsMetrics = make([]*Target, 0, len(g.TotalMetricNames))
	for _, totalMetricName := range g.TotalMetricNames {
		t := &Target{
			Expr:         fmt.Sprintf("%s_%s", g.MetricPrefix, totalMetricName),
			LegendFormat: "{{__name__}}",
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
		g.allStateTimelines(metricsNames),
		g.allStatusHistory(metricsNames),
	}

	cronJobRowPanels := g.generateCronJobRowPanels(metricsNames)
	panels = append(panels, cronJobRowPanels...)

	return panels
}

func (g *Generator) generateCronJobRowPanels(metricsNames *sync.Map) []Panel {
	cronJobRowPanels := make([]Panel, 0)
	metricsNames.Range(g.individualPanelsGenerator(&cronJobRowPanels))
	return cronJobRowPanels
}

func (g *Generator) individualPanelsGenerator(cronJobRowPanels *[]Panel) func(key, value any) bool {
	return func(key, value any) bool {
		if len(*cronJobRowPanels) > 0 {
			log.Info().
				Str(
					"individualPanelsGenerator",
					fmt.Sprintf("%d", len(*cronJobRowPanels)),
				).
				Msg("metricName")
			return true
		}

		metricNames, ok := value.([]string)
		if !ok {
			log.Error().
				Str("component", "dashboard").
				Str("operation", "individualPanelsGenerator").
				Msg("value.([]string) type assertion failed")
			return false
		}

		cronjobName, ok := key.(string)
		if !ok {
			log.Error().
				Str("component", "dashboard").
				Str("operation", "individualPanelsGenerator").
				Msg("key.(string) type assertion failed")
			return false
		}

		i := len(*cronJobRowPanels)
		var rowY, panelY, failureY uint16
		rowY = uint16((i + 1) * 10)
		panelY = rowY + 1
		failureY = panelY + 8

		row := Panel{
			Type:  "row",
			Title: cronjobName,
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

		var failureMetricName string
		cronjobDurations := make([]*Target, 0)
		cronjobTotals := make([]*Target, 0)

		for _, metricName := range metricNames {
			if durationRe.MatchString(metricName) {
				cronjobDurations = append(cronjobDurations, &Target{
					Expr:         metricName,
					LegendFormat: "{{job_name}}",
					DataSource:   dataSource,
				})
			} else {
				if failureMetricRe.MatchString(metricName) {
					metricName = "sk8l_${namespace}_${cronjob}_failure_total"
					failureMetricName = metricName
				} else {
					metricName = "sk8l_${namespace}_${cronjob}_completion_total"
				}
				cronjobTotals = append(cronjobTotals, &Target{
					Expr:         metricName,
					LegendFormat: "{{__name__}}",
					DataSource:   dataSource,
				})
			}
		}

		if len(cronjobTotals) > 0 {
			*cronJobRowPanels = append(*cronJobRowPanels, Panel{
				Title:      "${cronjob}: completion / failure totals",
				DataSource: dataSource,
				GridPos:    GridPos{X: 0, Y: panelY, H: 8, W: 12},
				Targets:    cronjobTotals,
				Options:    Option{Calcs: "last"},
			})
		}

		if len(cronjobDurations) > 0 {
			*cronJobRowPanels = append(*cronJobRowPanels, Panel{
				Title:      "${cronjob}: jobs duration",
				DataSource: dataSource,
				GridPos:    GridPos{X: 12, Y: panelY, H: 8, W: 12},
				Targets:    cronjobDurations,
				Options:    Option{Calcs: "max"},
			})
		}

		if failureMetricName != "" {
			*cronJobRowPanels = append(*cronJobRowPanels, Panel{
				Title:      "${cronjob}: state timeline",
				Type:       "state-timeline",
				DataSource: dataSource,
				GridPos:    GridPos{X: 0, Y: failureY, H: 8, W: 12},
				Targets: []*Target{
					{
						Expr:         failureMetricName,
						LegendFormat: "failure total",
						DataSource:   dataSource,
					},
				},
				Options: Option{Calcs: "last"},
			})
		}

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
	totalsTargets := make([]*Target, 0, len(g.TotalMetricNames))
	for _, totalMetricName := range g.TotalMetricNames {
		legendFmt := strings.TrimSuffix(totalMetricName, "_total")
		legendFmt = strings.ReplaceAll(legendFmt, "_", " ")
		totalsTargets = append(totalsTargets, &Target{
			Expr:         fmt.Sprintf("%s_%s", g.MetricPrefix, totalMetricName),
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
		Override:   Override{ID: "byName", Options: "failing cronjobs"},
	}
}

func (g *Generator) allStatusHistory(metricsNames *sync.Map) Panel {
	failureTargets := collectFailureTargets(metricsNames)
	return Panel{
		Title:      "status history",
		Type:       "status-history",
		DataSource: dataSource,
		GridPos:    GridPos{X: 12, Y: 9, H: 8, W: 12},
		Targets:    failureTargets,
	}
}

func (g *Generator) allStateTimelines(metricsNames *sync.Map) Panel {
	failureTargets := collectFailureTargets(metricsNames)
	return Panel{
		Title:      "status timeline",
		Type:       "state-timeline",
		DataSource: dataSource,
		GridPos:    GridPos{X: 0, Y: 9, H: 8, W: 12},
		Targets:    failureTargets,
		Options:    Option{Calcs: "last"},
	}
}

func collectFailureTargets(metricsNames *sync.Map) []*Target {
	failureTargets := make([]*Target, 0)
	metricsNames.Range(func(key, value any) bool {
		metricNames, ok := value.([]string)
		if !ok {
			log.Error().
				Str("component", "dashboard").
				Str("operation", "collectFailureTargets").
				Msg("value.([]string) type assertion failed")
			return false
		}
		for _, metricName := range metricNames {
			if failureMetricRe.MatchString(metricName) {
				failureTargets = append(failureTargets, &Target{
					Expr:         metricName,
					LegendFormat: failureLegendFmt(metricName),
					DataSource:   dataSource,
				})
			}
		}
		return true
	})
	return failureTargets
}

func failureLegendFmt(metricName string) string {
	// MetricPrefix is not available here — callers should pre-strip if needed.
	// This trims the failure suffix to produce a short legend label.
	shorterLegend := strings.TrimSuffix(failureMetricRe.String(), "$")
	legendFmt := strings.TrimSuffix(metricName, fmt.Sprintf("_%s", shorterLegend))
	return legendFmt
}
