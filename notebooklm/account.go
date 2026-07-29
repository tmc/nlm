package notebooklm

import (
	"context"
	"fmt"

	genmethod "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
	"google.golang.org/protobuf/proto"
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

func accountRequest() *pb.GetOrCreateAccountRequest {
	return &pb.GetOrCreateAccountRequest{
		ContextVersion: proto.Int32(2),
		ContextSurface: &pb.RequestSurface{Value: proto.Int32(1)},
		ContextCaps:    &pb.RequestClientCaps{Version: proto.Int32(1), CapabilityCodes: []int32{1, 3}},
	}
}

// GetAccountStatus dispatches ZwVcOc and projects the generated account shape.
func (c *Client) GetAccountStatus(ctx context.Context) (*AccountStatus, error) {
	resp, err := c.rpc.Do(ctx, rpc.Call{
		ID:   rpc.RPCGetOrCreateAccount,
		Args: genmethod.EncodeGetOrCreateAccountArgs(accountRequest()),
	})
	if err != nil {
		return nil, fmt.Errorf("get account status: %w", err)
	}
	status, err := parseAccountStatusProtoWithOptions(resp, c.unmarshalOptions())
	if err != nil {
		return nil, fmt.Errorf("get account status: decode response: %w", err)
	}
	return status, nil
}

func parseAccountStatusProto(raw []byte) (*AccountStatus, error) {
	return parseAccountStatusProtoWithOptions(raw, beprotojson.UnmarshalOptions{DiscardUnknown: true})
}

func parseAccountStatusProtoWithOptions(raw []byte, options beprotojson.UnmarshalOptions) (*AccountStatus, error) {
	account := new(pb.Account)
	if err := options.Unmarshal(raw, account); err != nil {
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
