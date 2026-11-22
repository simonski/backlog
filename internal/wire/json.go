package wire

import (
	"encoding/json"
	"errors"
	"io"
)

const MaxReportBytes = 1 << 20

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
	Version  string `json:"server_version"`
	Protocol int    `json:"protocol_version"`
}

type StopResult struct {
	PID int `json:"pid"`
}

type Failure struct {
	Error string `json:"error"`
}
