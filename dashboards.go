package main

import (
	"fmt"
	"regexp"
)

type DataSource struct {
	Type, Uid string
}

type Target struct {
	Expr, LegendFormat string
	DataSource         *DataSource
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
	Id      string `json:"id"`
	Options string `json:"options"`
}

type Panel struct {
	Title      string
	Targets    []*Target
	DataSource *DataSource
	Type       string
	GridPos    *GridPos `json:"gridPos"`
	Options    *Option
	Override   *Override
}

var (
	dataSource = &DataSource{
		Type: "prometheus",
		Uid:  "${DS_PROMETHEUS}",
	}

	durationRe              = regexp.MustCompile(`duration_seconds$`)
	failureMetricRe         = regexp.MustCompile(`failure_total$`)
	failingCronjobsMetricRe = regexp.MustCompile(`failing_cronjobs_total$`)

	totalMetricNames = []string{
		failingCronjobsOpts.Name,
		runningCronjobsOpts.Name,
		completedCronjobsOpts.Name,
		registeredCronjobsOpts.Name,
	}

	totalStatNames = []string{
		completedCronjobsOpts.Name,
		runningCronjobsOpts.Name,
		failingCronjobsOpts.Name,
	}
)

func generatePanels() []Panel {
	var totalsMetrics = []*Target{}
	var totalsStats = []*Target{}
	for _, totalMetricName := range totalMetricNames {
		t := &Target{
			Expr:         fmt.Sprintf("%s_%s", METRIC_PREFIX, totalMetricName),
			LegendFormat: "{{__name__}}",
			DataSource:   dataSource,
		}
		totalsMetrics = append(totalsMetrics, t)
	}

	for _, totalStatName := range totalStatNames {
		legendFmt := "{{__name__}}"
		if failingCronjobsMetricRe.MatchString(totalStatName) {
			legendFmt = "failing cronjobs"
		}

		t := &Target{
			Expr:         fmt.Sprintf("%s_%s", METRIC_PREFIX, totalStatName),
			LegendFormat: legendFmt,
			DataSource:   dataSource,
		}
		totalsStats = append(totalsStats, t)
	}

	var panels = []Panel{
		Panel{
			Type:       "row",
			Title:      fmt.Sprintf("sk8l: %s overview", K8_NAMESPACE),
			DataSource: dataSource,
			GridPos: &GridPos{
				H: 1,
				W: 24,
				X: 0,
				Y: 0,
			},
			Targets: make([]*Target, 0),
		},
		Panel{
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
		Panel{
			Type:       "stat",
			Title:      fmt.Sprintf("sk8l: %s totals", K8_NAMESPACE),
			DataSource: dataSource,
			GridPos: &GridPos{
				H: 8,
				W: 12,
				X: 12,
				Y: 1,
			},
			Targets: totalsStats,
			Options: &Option{
				Calcs: "lastNotNull",
			},
			Override: &Override{
				Id:      "byName",
				Options: "failing cronjobs",
			},
		},
	}

	individualPanels := []Panel{}

	individualPanelsGenerator := func(key, value any) bool {
		var row Panel
		var target = &Target{}
		var failureMetricName string
		var cronjobDurations = make([]*Target, 0)
		var cronjobTotals = make([]*Target, 0)
		metricNames := value.([]string)

		keyName := key.(string)

		i := len(individualPanels)

		var rowI, rowM, failureY uint16

		rowI = uint16((i + 1) * 9)
		rowM = uint16(rowI + 1)
		failureY = uint16(rowM + 8)

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
			fTarget := &Target{
				Expr:         failureMetricName,
				LegendFormat: "{{__name__}}",
				DataSource:   dataSource,
			}

			failureTargets := []*Target{
				fTarget,
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
