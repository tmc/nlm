// Package rpcinfo resolves a NotebookLM batchexecute rpc_id to the proto
// request and response message types bound to it.
//
// The binding is authoritative: it is read from the generated service
// descriptors, where each method carries an (rpc_id) extension
// (notebooklm.v1alpha1.rpc_id, field 51000) alongside its typed input and
// output messages. Callers can look up a method by rpc_id and construct a
// fresh, correctly-typed proto.Message to unmarshal a wire payload into.
package rpcinfo

import (
	"fmt"
	"sort"
	"sync"

	pbv1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// Method describes one rpc_id → service method binding.
type Method struct {
	// RPCID is the batchexecute rpc_id (e.g. "CCqFvf").
	RPCID string
	// Service is the unqualified service name (e.g. "LabsTailwindOrchestrationService").
	Service string
	// Name is the unqualified method name (e.g. "CreateProject").
	Name string
	// Request and Response are the method's input and output message types.
	Request  protoreflect.MessageType
	Response protoreflect.MessageType
}

// FullName returns "Service.Method".
func (m Method) FullName() string { return m.Service + "." + m.Name }

// NewRequest returns a fresh, zero-valued instance of the request type.
func (m Method) NewRequest() proto.Message { return m.Request.New().Interface() }

// NewResponse returns a fresh, zero-valued instance of the response type.
func (m Method) NewResponse() proto.Message { return m.Response.New().Interface() }

var (
	once     sync.Once
	byID     map[string][]Method
	buildErr error
)

// build walks the registered service descriptors once and records every
// method that carries an rpc_id extension.
func build() {
	byID = make(map[string][]Method)
	// Ensure the notebooklm protos are linked and registered.
	_ = pbv1.File_notebooklm_v1alpha1_rpc_extensions_proto

	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		services := fd.Services()
		for i := 0; i < services.Len(); i++ {
			svc := services.Get(i)
			methods := svc.Methods()
			for j := 0; j < methods.Len(); j++ {
				md := methods.Get(j)
				opts := md.Options()
				if opts == nil {
					continue
				}
				id, _ := proto.GetExtension(opts, pbv1.E_RpcId).(string)
				if id == "" {
					continue
				}
				reqType, err := protoregistry.GlobalTypes.FindMessageByName(md.Input().FullName())
				if err != nil {
					continue
				}
				respType, err := protoregistry.GlobalTypes.FindMessageByName(md.Output().FullName())
				if err != nil {
					continue
				}
				byID[id] = append(byID[id], Method{
					RPCID:    id,
					Service:  string(svc.Name()),
					Name:     string(md.Name()),
					Request:  reqType,
					Response: respType,
				})
			}
		}
		return true
	})
}

func load() error {
	once.Do(build)
	return buildErr
}

// ErrUnknownRPCID reports that no service method is bound to the given rpc_id.
type ErrUnknownRPCID struct{ RPCID string }

func (e ErrUnknownRPCID) Error() string {
	return fmt.Sprintf("rpcinfo: no proto method bound to rpc_id %q", e.RPCID)
}

// ErrAmbiguousRPCID reports that more than one service method is bound to the
// given rpc_id. The caller must pick one by full name.
type ErrAmbiguousRPCID struct {
	RPCID   string
	Methods []Method
}

func (e ErrAmbiguousRPCID) Error() string {
	names := make([]string, len(e.Methods))
	for i, m := range e.Methods {
		names[i] = m.FullName()
	}
	sort.Strings(names)
	return fmt.Sprintf("rpcinfo: rpc_id %q is bound to multiple methods: %v (disambiguate by method name)", e.RPCID, names)
}

// Lookup returns the single method bound to rpc_id. It returns ErrUnknownRPCID
// if none is bound and ErrAmbiguousRPCID if more than one is.
func Lookup(rpcID string) (Method, error) {
	if err := load(); err != nil {
		return Method{}, err
	}
	ms := byID[rpcID]
	switch len(ms) {
	case 0:
		return Method{}, ErrUnknownRPCID{RPCID: rpcID}
	case 1:
		return ms[0], nil
	default:
		return Method{}, ErrAmbiguousRPCID{RPCID: rpcID, Methods: append([]Method(nil), ms...)}
	}
}

// LookupAll returns every method bound to rpc_id, in the order discovered.
// It returns ErrUnknownRPCID if none is bound.
func LookupAll(rpcID string) ([]Method, error) {
	if err := load(); err != nil {
		return nil, err
	}
	ms := byID[rpcID]
	if len(ms) == 0 {
		return nil, ErrUnknownRPCID{RPCID: rpcID}
	}
	return append([]Method(nil), ms...), nil
}

// LookupByName returns the method whose unqualified name ("CreateAudioOverview")
// or qualified name ("Service.CreateAudioOverview") matches name. It returns
// ErrUnknownRPCID (carrying the name) if no method matches and
// ErrAmbiguousRPCID if more than one does.
func LookupByName(name string) (Method, error) {
	if err := load(); err != nil {
		return Method{}, err
	}
	var matches []Method
	for _, ms := range byID {
		for _, m := range ms {
			if name == m.Name || name == m.FullName() {
				matches = append(matches, m)
			}
		}
	}
	switch len(matches) {
	case 0:
		return Method{}, ErrUnknownRPCID{RPCID: name}
	case 1:
		return matches[0], nil
	default:
		return Method{}, ErrAmbiguousRPCID{RPCID: name, Methods: matches}
	}
}

// RPCIDs returns all bound rpc_ids in sorted order.
func RPCIDs() ([]string, error) {
	if err := load(); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}
