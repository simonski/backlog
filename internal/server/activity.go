package server

import (
	"fmt"
	"strings"
	"time"

	"harness.engineering/internal/model"
)

const defaultActivityRange = "4w"

type activityRange struct {
	key    string
	months int
	days   int
	step   int
}

var standardActivityRange = activityRange{key: defaultActivityRange, days: 28, step: 1}

var activityRanges = []activityRange{
	{key: "1y", months: 12, step: 7},
	{key: "6m", months: 6, step: 7},
	{key: "3m", months: 3, step: 7},
	{key: "1m", months: 1, step: 1},
	standardActivityRange,
	{key: "2w", days: 14, step: 1},
	{key: "1w", days: 7, step: 1},
}

func findActivityRange(key string) activityRange {
	for _, preset := range activityRanges {
		if preset.key == key {
			return preset
		}
	}
	return standardActivityRange
}

func (preset activityRange) first(today time.Time) time.Time {
	if preset.months == 0 {
		return today.AddDate(0, 0, 1-preset.days)
	}
	month := time.Date(today.Year(), today.Month()-time.Month(preset.months), 1, 0, 0, 0, 0, today.Location())
	day := min(today.Day(), month.AddDate(0, 1, -1).Day())
	return month.AddDate(0, 0, day)
}

func (preset activityRange) available(today time.Time, oldest string) bool {
	return preset.months == 0 || (oldest != "" && oldest <= preset.first(today).Format(time.DateOnly))
}

type activityOption struct {
	Key      string
	Selected bool
	Disabled bool
}

type activityDate struct {
	Date  string
	Label string
	Tick  string
	Month string
}

type activityCount struct {
	Status string
	Count  int
}

type activityCell struct {
	Date   string
	Label  string
	Counts []activityCount
}

type activityRow struct {
	Project model.Project
	Cells   []activityCell
}

type activityChart struct {
	Dates   []activityDate
	Rows    []activityRow
	Options []activityOption
	Grain   string
}

func makeActivityChart(today time.Time, selected activityRange, oldest string, projects []model.Project, activity []model.PeriodActivity) activityChart {
	chart := activityChart{Grain: "day"}
	if selected.step == 7 {
		chart.Grain = "week"
	}
	for _, preset := range activityRanges {
		chart.Options = append(chart.Options, activityOption{preset.key, preset.key == selected.key, !preset.available(today, oldest)})
	}
	first := selected.first(today)
	for day := first; !day.After(today); day = day.AddDate(0, 0, selected.step) {
		end := day.AddDate(0, 0, selected.step-1)
		if end.After(today) {
			end = today
		}
		label := day.Format("2 January 2006")
		if end.After(day) {
			label = day.Format("2 Jan 2006") + " – " + end.Format("2 Jan 2006")
		}
		date := activityDate{Date: day.Format(time.DateOnly), Label: label}
		chart.Dates = append(chart.Dates, date)
	}
	last := len(chart.Dates) - 1
	tickEvery := max(2, (last+3)/4)
	for i := range chart.Dates {
		if (i%tickEvery != 0 || i == last-1) && i != last {
			continue
		}
		day := first.AddDate(0, 0, i*selected.step)
		chart.Dates[i].Tick = day.Format("2")
		chart.Dates[i].Month = day.Format("Jan")
	}
	type dayKey struct {
		project int64
		date    string
	}
	counts := make(map[dayKey]map[string]int)
	for _, entry := range activity {
		key := dayKey{entry.ProjectID, entry.Start}
		if counts[key] == nil {
			counts[key] = make(map[string]int)
		}
		counts[key][entry.Status] = entry.Count
	}
	statuses := []struct{ value, label string }{
		{model.Blocked, "blocked"}, {model.Todo, "to do"}, {model.Doing, "doing"},
		{model.Done, "done"}, {model.Rejected, "rejected"},
	}
	for _, project := range projects {
		row := activityRow{Project: project}
		for _, date := range chart.Dates {
			cell := activityCell{Date: date.Date}
			var labels []string
			for _, status := range statuses {
				if count := counts[dayKey{project.ID, date.Date}][status.value]; count > 0 {
					cell.Counts = append(cell.Counts, activityCount{status.value, count})
					labels = append(labels, fmt.Sprintf("%d %s", count, status.label))
				}
			}
			if len(labels) == 0 {
				labels = []string{"no recorded activity"}
			}
			cell.Label = fmt.Sprintf("%s · %s · %s", project.Name(), date.Label, strings.Join(labels, ", "))
			row.Cells = append(row.Cells, cell)
		}
		chart.Rows = append(chart.Rows, row)
	}
	return chart
}
