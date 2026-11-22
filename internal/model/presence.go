package model

import (
	"unicode"
	"unicode/utf8"
)

func ValidMonitorName(name string) bool {
	if utf8.RuneCountInString(name) > 48 {
		return false
	}
	for _, character := range name {
		if !unicode.IsPrint(character) {
			return false
		}
	}
	return true
}
