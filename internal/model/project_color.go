package model

import "fmt"

type ProjectColorPreset struct {
	Key   string
	Name  string
	Value string
}

var ProjectColorPresets = []ProjectColorPreset{
	{"rose", "Rose", "hsl(344, 88%, 83%)"},
	{"peach", "Peach", "hsl(23, 88%, 82%)"},
	{"butter", "Butter", "hsl(46, 88%, 81%)"},
	{"lime", "Lime", "hsl(82, 88%, 81%)"},
	{"mint", "Mint", "hsl(150, 88%, 82%)"},
	{"sky", "Sky", "hsl(197, 88%, 82%)"},
	{"periwinkle", "Periwinkle", "hsl(228, 88%, 85%)"},
	{"lavender", "Lavender", "hsl(273, 88%, 85%)"},
}

func (project Project) Color() string {
	for _, preset := range ProjectColorPresets {
		if preset.Key == project.ColorPreset {
			return preset.Value
		}
	}
	index := project.ID - 1
	// Space automatic colors around the hue circle with a distinct saturation from presets.
	hue := (155 + (index%360)*137) % 360
	saturation := 56 + (index/360)%24
	lightness := 74 + (index/(360*24))%14
	return fmt.Sprintf("hsl(%d, %d%%, %d%%)", hue, saturation, lightness)
}
