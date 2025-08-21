// Sk8l
package main

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

	totalMetricNames = []string{
		registeredCronjobsOpts.Name,
		completedCronjobsOpts.Name,
		runningCronjobsOpts.Name,
		failingCronjobsOpts.Name,
	}
)

func generatePanels() []Panel {
	var totalsMetrics = make([]*Target, 0, len(totalMetricNames))
	for _, totalMetricName := range totalMetricNames {
		t := &Target{
			Expr:         fmt.Sprintf("%s_%s", MetricPrefix, totalMetricName),
			LegendFormat: "{{__name__}}",
			DataSource:   dataSource,
		}
		totalsMetrics = append(totalsMetrics, t)
	}

	var panels = []Panel{
		{
			Type:       "row",
			Title:      fmt.Sprintf("sk8l: %s overview", K8Namespace),
			DataSource: dataSource,
			GridPos: GridPos{
				X: 0,
				Y: 0,
				H: 1,
				W: 24,
			},
			Targets: make([]*Target, 0),
		},
		cronjobsTotalsTimeseries(totalsMetrics),
		totalsBarGaugePanel(),
		allStateTimelines(),
		allStatusHistory(),
	}

	cronJobRowPanels := generateCronJobRowPanels(metricsNamesMap)
	panels = append(panels, cronJobRowPanels...)

	return panels
}

func generateCronJobRowPanels(metricsNames *sync.Map) []Panel {
	cronJobRowPanels := make([]Panel, 0)
	metricsNames.Range(individualPanelsGenerator(&cronJobRowPanels))
	return cronJobRowPanels
}

func individualPanelsGenerator(cronJobRowPanels *[]Panel) func(key, value any) bool {
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

		var row Panel
		var target *Target
		var failureMetricName string
		var cronjobDurations = make([]*Target, 0)
		var cronjobTotals = make([]*Target, 0)

		metricNames, ok := value.([]string)
		if !ok {
			log.Error().
				Str("component", "dashboards").
				Str("operation", "generatePanels").
				Msg("value.([]string)")
		}

		_, ok = key.(string)
		if !ok {
			log.Error().
				Str("component", "dashboards").
				Str("operation", "generatePanels").
				Msg("key.(string)")
		}

		i := len(*cronJobRowPanels)

		var rowY, panelY, failureY uint16

		rowY = uint16((i + 1) * 10)
		panelY = rowY + 1
		failureY = panelY + 8
		emptyRowTargets := make([]*Target, 0)

		row = Panel{
			Type:  "row",
			Title: "${cronjob}",
			GridPos: GridPos{
				X: 0,
				Y: rowY,
				H: 1,
				W: 24,
			},
			Targets:    emptyRowTargets,
			DataSource: dataSource,
			Repeat:     "cronjob",
		}
		*cronJobRowPanels = append(*cronJobRowPanels, row)

		for _, metricName := range metricNames {
			if durationRe.MatchString(metricName) {
				target = &Target{
					Expr:         metricName,
					LegendFormat: "{{job_name}}",
					DataSource:   dataSource,
				}
				cronjobDurations = append(cronjobDurations, target)
			} else {
				if failureMetricRe.MatchString(metricName) {
					metricName = "sk8l_${namespace}_${cronjob}_failure_total"
					failureMetricName = metricName
				} else {
					metricName = "sk8l_${namespace}_${cronjob}_completion_total"
				}
				target = &Target{
					Expr:         metricName,
					LegendFormat: "{{__name__}}",
					DataSource:   dataSource,
				}
				cronjobTotals = append(cronjobTotals, target)
			}
		}

		a := "${cronjob}: completion / failure totals"
		b := "${cronjob}: jobs duration"
		c := "${cronjob}: state timeline"

		if len(cronjobTotals) > 0 {
			cronjobTotalsPanel := Panel{
				Title:      a,
				DataSource: dataSource,
				GridPos: GridPos{
					X: 0,
					Y: panelY,
					H: 8,
					W: 12,
				},
				Targets: cronjobTotals,
				Options: Option{
					Calcs: "last",
				},
			}
			*cronJobRowPanels = append(*cronJobRowPanels, cronjobTotalsPanel)
		}

		if len(cronjobDurations) > 0 {
			cronjobDurationsPanel := Panel{
				Title:      b,
				DataSource: dataSource,
				GridPos: GridPos{
					X: 12,
					Y: panelY,
					H: 8,
					W: 12,
				},
				Targets: cronjobDurations,
				Options: Option{
					Calcs: "max",
				},
			}
			*cronJobRowPanels = append(*cronJobRowPanels, cronjobDurationsPanel)
		}

		if failureMetricName != "" {
			failureTargets := []*Target{
				{
					Expr:         failureMetricName,
					LegendFormat: "failure total",
					DataSource:   dataSource,
				},
			}

			cronjobStateTimelinePanel := Panel{
				Title:      c,
				Type:       "state-timeline",
				DataSource: dataSource,
				GridPos: GridPos{
					X: 0,
					Y: failureY,
					H: 8,
					W: 12,
				},
				Targets: failureTargets,
				Options: Option{
					Calcs: "last",
				},
			}
			*cronJobRowPanels = append(*cronJobRowPanels, cronjobStateTimelinePanel)
		}

		return true
	}
}

func cronjobsTotalsTimeseries(totalsMetrics []*Target) Panel {
	return Panel{
		Title:      "completed / registered / failed cronjobs totals",
		Type:       "timeseries",
		DataSource: dataSource,
		GridPos: GridPos{
			X: 0,
			Y: 1,
			H: 8,
			W: 12,
		},
		Targets: totalsMetrics,
		Options: Option{
			Calcs: "last",
		},
	}
}

func totalsBarGaugePanel() Panel {
	var totalsTargets = make([]*Target, 0, len(totalMetricNames))
	for _, totalMetricName := range totalMetricNames {
		legendFmt := strings.TrimSuffix(totalMetricName, "_total")
		legendFmt = strings.ReplaceAll(legendFmt, "_", " ")

		t := &Target{
			Expr:         fmt.Sprintf("%s_%s", MetricPrefix, totalMetricName),
			LegendFormat: legendFmt,
			DataSource:   dataSource,
		}
		totalsTargets = append(totalsTargets, t)
	}

	return Panel{
		Type:       "bargauge",
		Title:      fmt.Sprintf("sk8l: %s totals", K8Namespace),
		DataSource: dataSource,
		GridPos: GridPos{
			X: 12,
			Y: 1,
			H: 8,
			W: 12,
		},
		Targets: totalsTargets,
		Options: Option{
			Calcs: "lastNotNull",
		},
		Override: Override{
			ID:      "byName",
			Options: "failing cronjobs",
		},
	}
}

func allStatusHistory() Panel {
	failureTargets := make([]*Target, 0)

	metricsNamesMap.Range(func(key, value any) bool {
		metricNames, ok := value.([]string)
		if !ok {
			log.Error().
				Str("component", "dashboards").
				Str("operation", "allStatusHistory").
				Msg("value.([]string)")
		}

		for _, metricName := range metricNames {
			if failureMetricRe.MatchString(metricName) {
				failureTarget := &Target{
					Expr:         metricName,
					LegendFormat: failureLegendFmt(metricName), // {{__name__}}
					DataSource:   dataSource,
				}
				failureTargets = append(failureTargets, failureTarget)
			}
		}
		return true
	})

	return Panel{
		Title:      "status history",
		Type:       "status-history",
		DataSource: dataSource,
		GridPos: GridPos{
			X: 12,
			Y: 9,
			H: 8,
			W: 12,
		},
		Targets: failureTargets,
	}
}

func allStateTimelines() Panel {
	failureTargets := make([]*Target, 0)

	metricsNamesMap.Range(func(key, value any) bool {
		metricNames, ok := value.([]string)
		if !ok {
			log.Error().
				Str("component", "dashboards").
				Str("operation", "allStateTimelines").
				Msg("value.([]string)")
		}

		for _, metricName := range metricNames {
			if failureMetricRe.MatchString(metricName) {
				failureTarget := &Target{
					Expr:         metricName,
					LegendFormat: failureLegendFmt(metricName), // {{__name__}}
					DataSource:   dataSource,
				}
				failureTargets = append(failureTargets, failureTarget)
			}
		}
		return true
	})

	return Panel{
		Title:      "status timeline",
		Type:       "state-timeline",
		DataSource: dataSource,
		GridPos: GridPos{
			X: 0,
			Y: 9,
			H: 8,
			W: 12,
		},
		Targets: failureTargets,
		Options: Option{
			Calcs: "last",
		},
	}
}

func failureLegendFmt(metricName string) string {
	legendFmt := strings.TrimPrefix(metricName, fmt.Sprintf("%s_", MetricPrefix))
	shorterLegend := strings.TrimSuffix(failureMetricRe.String(), "$")
	legendFmt = strings.TrimSuffix(legendFmt, fmt.Sprintf("_%s", shorterLegend))
	return legendFmt
}
