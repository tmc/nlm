package api

import (
	"encoding/json"
	"fmt"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
)

// AccountStatus is the public projection of the ZwVcOc account/status response.
// The generated Account message owns the wire shape; this type preserves the
// compact quota API used by existing callers.
type AccountStatus struct {
	NotebookLimit int `json:"notebook_limit,omitempty"`
	SourceLimit   int `json:"source_limit,omitempty"`
	UploadLimit   int `json:"upload_limit,omitempty"`
	Tier          int `json:"tier,omitempty"`
}

// GetAccountStatus dispatches ZwVcOc and projects the generated account shape.
func (c *Client) GetAccountStatus() (*AccountStatus, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCGetOrCreateAccount,
		Args: []interface{}{},
	})
	if err != nil {
		return nil, fmt.Errorf("get account status: %w", err)
	}
	status, err := parseAccountStatusProto(resp)
	if err != nil {
		return nil, fmt.Errorf("get account status: decode response: %w", err)
	}
	return status, nil
}

func parseAccountStatusProto(raw []byte) (*AccountStatus, error) {
	account := new(pb.Account)
	if err := beprotojson.Unmarshal(raw, account); err != nil {
		return nil, fmt.Errorf("account proto decode: %w", err)
	}
	limits := account.GetLimits()
	if limits == nil {
		return nil, fmt.Errorf("missing account limits")
	}
	return &AccountStatus{
		NotebookLimit: int(limits.GetNotebookLimit()),
		SourceLimit:   int(limits.GetSourceLimit()),
		UploadLimit:   int(limits.GetUploadLimit()),
		Tier:          int(limits.GetTier_2()),
	}, nil
}

func parseAccountStatus(b []byte) (*AccountStatus, error) {
	var raw any
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil, fmt.Errorf("missing account status")
	}
	if len(items) == 1 {
		if nested, ok := items[0].([]any); ok {
			items = nested
		}
	}
	if len(items) < 2 {
		return nil, fmt.Errorf("missing account limits")
	}
	limits, ok := items[1].([]any)
	if !ok || len(limits) < 5 {
		return nil, fmt.Errorf("missing account limits")
	}
	values := make([]int, 5)
	for i := range values {
		n, ok := accountNumber(limits[i])
		if !ok {
			return nil, fmt.Errorf("bad account limit %d", i)
		}
		values[i] = int(n)
	}
	status := &AccountStatus{
		NotebookLimit: values[1],
		SourceLimit:   values[2],
		UploadLimit:   values[3],
		Tier:          values[4],
	}
	return status, nil
}

func accountNumber(v any) (float64, bool) {
	n, ok := v.(float64)
	return n, ok
}
