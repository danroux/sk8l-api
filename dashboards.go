// Sk8l
package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
)

type DataSource struct {
	UID, Type string
}

type Target struct {
	DataSource         *DataSource
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
	GridPos    *GridPos `json:"gridPos"`
	DataSource *DataSource
	Options    *Option
	Override   *Override
	Title      string
	Type       string
	Targets    []*Target
}

var (
	dataSource = &DataSource{
		Type: "prometheus",
		UID:  "${DS_PROMETHEUS}",
	}

	durationRe              = regexp.MustCompile(`duration_seconds$`)
	failureMetricRe         = regexp.MustCompile(`failure_total$`)
	failingCronjobsMetricRe = regexp.MustCompile(`failing_cronjobs_total$`)

	totalMetricNames = []string{
		registeredCronjobsOpts.Name,
		completedCronjobsOpts.Name,
		runningCronjobsOpts.Name,
		failingCronjobsOpts.Name,
	}

	totalStatNames = []string{
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
			GridPos: &GridPos{
				H: 1,
				W: 24,
				X: 0,
				Y: 0,
			},
			Targets: make([]*Target, 0),
		},
		{
			Title:      "completed / registered / failed cronjobs totals",
			DataSource: dataSource,
			GridPos: &GridPos{
				H: 8,
				W: 12,
				X: 0,
				Y: 1,
			},
			Targets: totalsMetrics,
			Options: &Option{
				Calcs: "last",
			},
		},
		totalsBarGaugePanel(),
	}

	individualPanels := make([]Panel, 0)
	individualPanelsGenerator := func(key, value any) bool {
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

		keyName, ok := key.(string)

		if !ok {
			log.Error().
				Str("component", "dashboards").
				Str("operation", "generatePanels").
				Msg("key.(string)")
		}

		i := len(individualPanels)

		var rowI, rowM, failureY uint16

		rowI = uint16((i + 1) * 9)
		rowM = rowI + 1
		failureY = rowM + 8

		row = Panel{
			Type:  "row",
			Title: keyName,
			GridPos: &GridPos{
				H: 1,
				W: 24,
				X: 0,
				Y: rowI,
			},
			Targets:    make([]*Target, 0),
			DataSource: dataSource,
		}

		individualPanels = append(individualPanels, row)

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
					failureMetricName = metricName
				}
				target = &Target{
					Expr:         metricName,
					LegendFormat: "{{__name__}}",
					DataSource:   dataSource,
				}
				cronjobTotals = append(cronjobTotals, target)
			}
		}

		a := fmt.Sprintf("%s: completion / failure totals", keyName)
		b := fmt.Sprintf("%s: jobs duration", keyName)
		c := fmt.Sprintf("%s: state timeline", keyName)

		if len(cronjobTotals) > 0 {
			individualPanels = append(individualPanels, Panel{
				Title:      a,
				DataSource: dataSource,
				GridPos: &GridPos{
					H: 8,
					W: 12,
					X: 0,
					Y: rowM,
				},
				Targets: cronjobTotals,
				Options: &Option{
					Calcs: "last",
				},
			})
		}

		if len(cronjobDurations) > 0 {
			individualPanels = append(individualPanels, Panel{
				Title:      b,
				DataSource: dataSource,
				GridPos: &GridPos{
					H: 8,
					W: 12,
					X: 12,
					Y: rowM,
				},
				Targets: cronjobDurations,
				Options: &Option{
					Calcs: "max",
				},
			})
		}

		if failureMetricName != "" {
			failureTargets := []*Target{
				&Target{
					Expr:         failureMetricName,
					LegendFormat: "failure total", // {{ __name__ }}
					DataSource:   dataSource,
				},
			}

			individualPanels = append(individualPanels, Panel{
				Title:      c,
				Type:       "state-timeline",
				DataSource: dataSource,
				GridPos: &GridPos{
					H: 8,
					W: 12,
					X: 0,
					Y: failureY,
				},
				Targets: failureTargets,
				Options: &Option{
					Calcs: "last",
				},
			})
		}

		return true
	}

	metricsNamesMap.Range(individualPanelsGenerator)
	panels = append(panels, individualPanels...)

	return panels
}

// func totalsStatPanel() Panel {
//      var totalsStatsTargets = make([]*Target, 0, len(totalStatNames))
//      for _, totalStatName := range totalStatNames {
//              legendFmt := "{{__name__}}"
//              if failingCronjobsMetricRe.MatchString(totalStatName) {
//                      legendFmt = "failing cronjobs"
//              }

//              t := &Target{
//                      Expr:         fmt.Sprintf("%s_%s", MetricPrefix, totalStatName),
//                      LegendFormat: legendFmt,
//                      DataSource:   dataSource,
//              }
//              totalsStatsTargets = append(totalsStatsTargets, t)
//      }

//      return Panel{
//              Type:       "stat",
//              Title:      fmt.Sprintf("sk8l: %s totals", K8Namespace),
//              DataSource: dataSource,
//              GridPos: &GridPos{
//                      H: 8,
//                      W: 12,
//                      X: 12,
//                      Y: 1,
//              },
//              Targets: totalsStatsTargets,
//              Options: &Option{
//                      Calcs: "lastNotNull",
//              },
//              Override: &Override{
//                      ID:      "byName",
//                      Options: "failing cronjobs",
//              },
//      }
// }

func totalsBarGaugePanel() Panel {
	var totalsTargets = make([]*Target, 0, len(totalMetricNames))
	for _, totalMetricName := range totalMetricNames {
		// legendFmt := "{{__name__}}"
		// if failingCronjobsMetricRe.MatchString(totalMetricName) {
		//      legendFmt = "failing cronjobs"
		// }
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
		GridPos: &GridPos{
			H: 8,
			W: 12,
			X: 12,
			Y: 1,
		},
		Targets: totalsTargets,
		Options: &Option{
			Calcs: "lastNotNull",
		},
		Override: &Override{
			ID:      "byName",
			Options: "failing cronjobs",
		},
	}
}
