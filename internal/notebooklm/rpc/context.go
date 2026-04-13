package rpc

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// NotebookIDFromMessage extracts the notebook context used for batchexecute
// source-path routing from a protobuf request message.
func NotebookIDFromMessage(msg proto.Message) string {
	if msg == nil {
		return ""
	}
	return notebookIDFromReflect(msg.ProtoReflect())
}

func notebookIDFromReflect(msg protoreflect.Message) string {
	if !msg.IsValid() {
		return ""
	}

	fields := msg.Descriptor().Fields()
	for _, name := range []protoreflect.Name{"project_id", "notebook_id"} {
		field := fields.ByName(name)
		if field == nil || field.Kind() != protoreflect.StringKind || !msg.Has(field) {
			continue
		}
		if id := msg.Get(field).String(); id != "" {
			return id
		}
	}

	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if field.Kind() != protoreflect.MessageKind || !msg.Has(field) {
			continue
		}
		if id := notebookIDFromReflect(msg.Get(field).Message()); id != "" {
			return id
		}
	}

	return ""
}
