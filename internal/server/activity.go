package server

import (
	"fmt"
	"strings"
	"time"

	"harness.engineering/internal/model"
)

const activityDays = 30

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
	Dates []activityDate
	Rows  []activityRow
}

func makeActivityChart(first time.Time, projects []model.Project, activity []model.DailyActivity) activityChart {
	var chart activityChart
	for i := range activityDays {
		day := first.AddDate(0, 0, i)
		date := activityDate{Date: day.Format(time.DateOnly), Label: day.Format("2 January 2006")}
		if (i%7 == 0 && i < activityDays-2) || i == activityDays-1 {
			date.Tick = day.Format("2")
			date.Month = day.Format("Jan")
		}
		chart.Dates = append(chart.Dates, date)
	}
	type dayKey struct {
		project int64
		date    string
	}
	counts := make(map[dayKey]map[string]int)
	for _, entry := range activity {
		key := dayKey{entry.ProjectID, entry.Day}
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

func projectColor(id int64) string {
	index := id - 1
	// Space consecutive projects around the hue circle and vary the pastel
	// saturation and lightness as the hue sequence repeats.
	hue := (155 + (index%360)*137) % 360
	saturation := 56 + (index/360)%24
	lightness := 74 + (index/(360*24))%14
	return fmt.Sprintf("hsl(%d, %d%%, %d%%)", hue, saturation, lightness)
}
