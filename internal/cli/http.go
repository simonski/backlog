package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"harness.engineering/internal/local"
	"harness.engineering/internal/model"
	"harness.engineering/internal/version"
	"harness.engineering/internal/wire"
)

func publish(ctx context.Context, address string, report model.Report, diagnostics io.Writer) (model.Result, error) {
	var result model.Result
	if err := local.ValidateAddress(address); err != nil {
		return result, err
	}
	client := &http.Client{
		Transport: &http.Transport{},
		Timeout:   4 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer client.CloseIdleConnections()
	base := "http://" + address
	var advertised wire.ServerVersion
	versionErr := request(ctx, client, http.MethodGet, base+"/api/version", nil, &advertised)
	if versionErr == nil && version.Valid(advertised.Version) {
		if warning := version.Warning(version.Client, advertised.Version); warning != "" {
			fmt.Fprintln(diagnostics, warning)
		}
	} else {
		advertised.Version = ""
	}

	payload, err := json.Marshal(report)
	if err != nil {
		return result, err
	}
	if len(payload) > wire.MaxReportBytes {
		return result, errors.New("report exceeds 1 MiB")
	}
	err = request(ctx, client, http.MethodPost, base+"/api/reports", payload, &result)
	if err != nil && advertised.Version == "" {
		err = fmt.Errorf("%w (server version unavailable)", err)
	}
	return result, err
}

func request(ctx context.Context, client *http.Client, method, url string, payload []byte, target any) error {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body := io.LimitReader(response.Body, 64<<10)
	if response.StatusCode != http.StatusOK {
		var failure wire.Failure
		if err := json.NewDecoder(body).Decode(&failure); err == nil && failure.Error != "" {
			return fmt.Errorf("%s: %s", response.Status, failure.Error)
		}
		return errors.New(response.Status)
	}
	if err := json.NewDecoder(body).Decode(target); err != nil {
		return fmt.Errorf("invalid server response: %w", err)
	}
	return nil
}
