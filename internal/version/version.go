package version

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	Client   = "0.12.1"
	Server   = "0.11.1"
	Protocol = 1
)

func core(value string) ([3]int, bool) {
	var parsed [3]int
	core, _, _ := strings.Cut(value, "+")
	core, _, _ = strings.Cut(core, "-")
	parts := strings.Split(core, ".")
	if len(parts) != len(parsed) {
		return parsed, false
	}
	for i, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 || strconv.Itoa(number) != part {
			return parsed, false
		}
		parsed[i] = number
	}
	return parsed, true
}

func Valid(value string) bool {
	_, ok := core(value)
	return ok
}

func Older(version, than string) bool {
	a, aOK := core(version)
	b, bOK := core(than)
	return aOK && bOK && slices.Compare(a[:], b[:]) < 0
}

// Warning describes a major or minor application version mismatch.
func Warning(client, server string) string {
	c, clientOK := core(client)
	s, serverOK := core(server)
	if !clientOK || !serverOK || (c[0] == s[0] && c[1] == s[1]) {
		return ""
	}
	older := "client"
	if c[0] > s[0] || (c[0] == s[0] && c[1] > s[1]) {
		older = "server"
	}
	return fmt.Sprintf("version mismatch: CLI %s, server %s; upgrade the %s", client, server, older)
}
