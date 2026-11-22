package wire

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
)

const MaxReportBytes = 1 << 20
const MonitorUsernameHeader = "Backlog-Username"

func ReadJSON(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("expected exactly one JSON value")
	}
	return nil
}

type ServerVersion struct {
	Version        string `json:"server_version"`
	Protocol       int    `json:"protocol_version"`
	ProtocolSemver string `json:"protocol_semver,omitempty"`
}

// NetworkProtocol uses the semantic version when present and maps older integer versions to a major release.
func (version ServerVersion) NetworkProtocol() string {
	if version.ProtocolSemver != "" {
		return version.ProtocolSemver
	}
	if version.Protocol > 0 {
		return strconv.Itoa(version.Protocol) + ".0.0"
	}
	return ""
}

type StopResult struct {
	PID int `json:"pid"`
}

type ClientHeartbeat struct {
	Username string `json:"username"`
}

type Failure struct {
	Error string `json:"error"`
}
