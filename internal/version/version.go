package version

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	Client         = "0.16.0"
	Server         = "0.16.0"
	Protocol       = "1.0.0"
	LegacyProtocol = 1
	clientUpgrade  = "upgrade the client (run brew upgrade simonski/backlog/backlog or run make install from an updated checkout)"
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
	advice := clientUpgrade
	if c[0] > s[0] || (c[0] == s[0] && c[1] > s[1]) {
		advice = "upgrade the server"
	}
	return fmt.Sprintf("version mismatch: CLI %s, server %s; %s", client, server, advice)
}

// CheckProtocol reports minor-version differences and rejects incompatible major versions.
func CheckProtocol(server string) (string, error) {
	clientVersion, _ := core(Protocol)
	serverVersion, valid := core(server)
	if !valid || strings.ContainsAny(server, "+-") || serverVersion[0] == 0 {
		return "", fmt.Errorf("server protocol version unavailable or invalid (%q)", server)
	}
	if clientVersion[0] == serverVersion[0] && clientVersion[1] == serverVersion[1] {
		return "", nil
	}
	advice := clientUpgrade
	if slices.Compare(clientVersion[:], serverVersion[:]) > 0 {
		advice = "upgrade the server"
	}
	detail := fmt.Sprintf("CLI protocol %s, server protocol %s; %s", Protocol, server, advice)
	if clientVersion[0] != serverVersion[0] {
		return "", fmt.Errorf("incompatible network protocol: %s", detail)
	}
	return "network protocol version mismatch: " + detail, nil
}
