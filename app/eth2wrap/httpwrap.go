// Copyright © 2022 Obol Labs Inc.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of  MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along with
// this program.  If not, see <http://www.gnu.org/licenses/>.

package eth2wrap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	eth2http "github.com/attestantio/go-eth2-client/http"
	eth2p0 "github.com/attestantio/go-eth2-client/spec/phase0"

	"github.com/obolnetwork/charon/app/errors"
	"github.com/obolnetwork/charon/app/z"
	"github.com/obolnetwork/charon/eth2util/eth2exp"
)

// BlockAttestationsProvider is the interface for providing attestations included in blocks.
// It is a standard beacon API endpoint not implemented by eth2client.
// See https://ethereum.github.io/beacon-APIs/#/Beacon/getBlockAttestations.
type BlockAttestationsProvider interface {
	BlockAttestations(ctx context.Context, stateID string) ([]*eth2p0.Attestation, error)
}

// NewHTTPAdapterForT returns a http adaptor for testing non-eth2service methods as it is nil.
func NewHTTPAdapterForT(_ *testing.T, address string, timeout time.Duration) *httpAdapter {
	return &httpAdapter{
		address: address,
		timeout: timeout,
	}
}

// httpAdapter implements Client by wrapping eth2http.Service adding the experimental interfaces not present in go-eth2-client.
type httpAdapter struct {
	*eth2http.Service
	address string
	timeout time.Duration
}

type submitBeaconCommitteeSelectionsJSON struct {
	Data []*eth2exp.BeaconCommitteeSelection `json:"data"`
}

type submitSyncCommitteeSelectionsJSON struct {
	Data []*eth2exp.SyncCommitteeSelection `json:"data"`
}

type attestationsJSON struct {
	Data []*eth2p0.Attestation `json:"data"`
}

// AggregateBeaconCommitteeSelections implements eth2exp.BeaconCommitteeSelectionAggregator.
func (h *httpAdapter) AggregateBeaconCommitteeSelections(ctx context.Context, selections []*eth2exp.BeaconCommitteeSelection) ([]*eth2exp.BeaconCommitteeSelection, error) {
	reqBody, err := json.Marshal(selections)
	if err != nil {
		return nil, errors.Wrap(err, "marshal submit beacon committee selections")
	}

	respBody, err := httpPost(ctx, h.address, "/eth/v1/validator/beacon_committee_selections", bytes.NewReader(reqBody), h.timeout)
	if err != nil {
		return nil, errors.Wrap(err, "submit beacon committee selections")
	}

	var resp submitBeaconCommitteeSelectionsJSON
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, errors.Wrap(err, "failed to parse beacon committee selections response")
	}

	return resp.Data, nil
}

// AggregateSyncCommitteeSelections implements eth2exp.SyncCommitteeSelectionAggregator.
func (h *httpAdapter) AggregateSyncCommitteeSelections(ctx context.Context, selections []*eth2exp.SyncCommitteeSelection) ([]*eth2exp.SyncCommitteeSelection, error) {
	reqBody, err := json.Marshal(selections)
	if err != nil {
		return nil, errors.Wrap(err, "marshal sync committee selections")
	}

	respBody, err := httpPost(ctx, h.address, "/eth/v1/validator/sync_committee_selections", bytes.NewReader(reqBody), h.timeout)
	if err != nil {
		return nil, errors.Wrap(err, "submit sync committee selections")
	}

	var resp submitSyncCommitteeSelectionsJSON
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, errors.Wrap(err, "failed to parse sync committee selections response")
	}

	return resp.Data, nil
}

// BlockAttestations returns the attestations included in the requested block.
// See https://ethereum.github.io/beacon-APIs/#/Beacon/getBlockAttestations.
func (h *httpAdapter) BlockAttestations(ctx context.Context, stateID string) ([]*eth2p0.Attestation, error) {
	path := fmt.Sprintf("/eth/v1/beacon/blocks/%s/attestations", stateID)
	respBody, err := httpGet(ctx, h.address, path, h.timeout)
	if err != nil {
		return nil, errors.Wrap(err, "request block attestations")
	}

	var resp attestationsJSON
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, errors.Wrap(err, "failed to parse block attestations response")
	}

	return resp.Data, nil
}

func httpPost(ctx context.Context, base string, endpoint string, body io.Reader, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addr, err := url.JoinPath(base, endpoint)
	if err != nil {
		return nil, errors.Wrap(err, "invalid address")
	}

	url, err := url.Parse(addr)
	if err != nil {
		return nil, errors.Wrap(err, "invalid endpoint")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url.String(), body)
	if err != nil {
		return nil, errors.Wrap(err, "new POST request with ctx")
	}

	res, err := new(http.Client).Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call POST endpoint")
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read POST response")
	}

	if res.StatusCode/100 != 2 {
		return nil, errors.New("post failed", z.Int("status", res.StatusCode), z.Str("body", string(data)))
	}

	return data, nil
}

func httpGet(ctx context.Context, base string, endpoint string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addr, err := url.JoinPath(base, endpoint)
	if err != nil {
		return nil, errors.Wrap(err, "invalid address")
	}

	url, err := url.Parse(addr)
	if err != nil {
		return nil, errors.Wrap(err, "invalid endpoint")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "new GET request with ctx")
	}

	res, err := new(http.Client).Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call GET endpoint")
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read GET response")
	}

	if res.StatusCode/100 != 2 {
		return nil, errors.New("get failed", z.Int("status", res.StatusCode), z.Str("body", string(data)))
	}

	return data, nil
}
