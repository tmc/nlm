// Handwritten protobuf types for GetNotes API response parsing.
// These types match the actual structure returned by the NotebookLM API
// rather than the standard Source structure.

package notebooklmv1alpha1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
)

// NoteEntry represents a single note entry in the GetNotes response.
// API format: [noteID, [noteID, content, metadata, null, title]]
// Position mapping:
//
//	Position 0 -> field 1: source_id (string)
//	Position 1 -> field 2: details (NoteDetails)
type NoteEntry struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	SourceId string       `protobuf:"bytes,1,opt,name=source_id,json=sourceId,proto3" json:"source_id,omitempty"`
	Details  *NoteDetails `protobuf:"bytes,2,opt,name=details,proto3" json:"details,omitempty"`
}

func (x *NoteEntry) Reset() {
	*x = NoteEntry{}
}

func (x *NoteEntry) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*NoteEntry) ProtoMessage() {}

func (x *NoteEntry) ProtoReflect() protoreflect.Message {
	return nil // Simplified - not needed for beprotojson
}

func (*NoteEntry) Descriptor() ([]byte, []int) {
	return nil, nil
}

func (x *NoteEntry) GetSourceId() string {
	if x != nil {
		return x.SourceId
	}
	return ""
}

func (x *NoteEntry) GetDetails() *NoteDetails {
	if x != nil {
		return x.Details
	}
	return nil
}

// NoteDetails represents the nested details array within a note entry.
// API format: [noteID, content, metadata, null, title]
// Position mapping:
//
//	Position 0 -> field 1: id (string, duplicate of source_id)
//	Position 1 -> field 2: content (string, the note body)
//	Position 2 -> field 3: metadata (NoteTimestampMetadata)
//	Position 3 -> field 4: reserved (null, skipped)
//	Position 4 -> field 5: title (string, the note title)
type NoteDetails struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id       string                 `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Content  string                 `protobuf:"bytes,2,opt,name=content,proto3" json:"content,omitempty"`
	Metadata *NoteTimestampMetadata `protobuf:"bytes,3,opt,name=metadata,proto3" json:"metadata,omitempty"`
	Reserved string                 `protobuf:"bytes,4,opt,name=reserved,proto3" json:"reserved,omitempty"`
	Title    string                 `protobuf:"bytes,5,opt,name=title,proto3" json:"title,omitempty"`
}

func (x *NoteDetails) Reset() {
	*x = NoteDetails{}
}

func (x *NoteDetails) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*NoteDetails) ProtoMessage() {}

func (x *NoteDetails) ProtoReflect() protoreflect.Message {
	return nil
}

func (*NoteDetails) Descriptor() ([]byte, []int) {
	return nil, nil
}

func (x *NoteDetails) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *NoteDetails) GetContent() string {
	if x != nil {
		return x.Content
	}
	return ""
}

func (x *NoteDetails) GetMetadata() *NoteTimestampMetadata {
	if x != nil {
		return x.Metadata
	}
	return nil
}

func (x *NoteDetails) GetTitle() string {
	if x != nil {
		return x.Title
	}
	return ""
}

// NoteTimestampMetadata represents the metadata array within note details.
// API format: [type, id, [seconds, nanos]]
// Position mapping:
//
//	Position 0 -> field 1: type (int32)
//	Position 1 -> field 2: id (string)
//	Position 2 -> field 3: timestamp (TimestampPair)
type NoteTimestampMetadata struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Type      int32          `protobuf:"varint,1,opt,name=type,proto3" json:"type,omitempty"`
	Id        string         `protobuf:"bytes,2,opt,name=id,proto3" json:"id,omitempty"`
	Timestamp *TimestampPair `protobuf:"bytes,3,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
}

func (x *NoteTimestampMetadata) Reset() {
	*x = NoteTimestampMetadata{}
}

func (x *NoteTimestampMetadata) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*NoteTimestampMetadata) ProtoMessage() {}

func (x *NoteTimestampMetadata) ProtoReflect() protoreflect.Message {
	return nil
}

func (*NoteTimestampMetadata) Descriptor() ([]byte, []int) {
	return nil, nil
}

func (x *NoteTimestampMetadata) GetType() int32 {
	if x != nil {
		return x.Type
	}
	return 0
}

func (x *NoteTimestampMetadata) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *NoteTimestampMetadata) GetTimestamp() *TimestampPair {
	if x != nil {
		return x.Timestamp
	}
	return nil
}

// TimestampPair represents a timestamp as [seconds, nanos].
// API format: [seconds, nanos]
// Position mapping:
//
//	Position 0 -> field 1: seconds (int64)
//	Position 1 -> field 2: nanos (int32)
type TimestampPair struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Seconds int64 `protobuf:"varint,1,opt,name=seconds,proto3" json:"seconds,omitempty"`
	Nanos   int32 `protobuf:"varint,2,opt,name=nanos,proto3" json:"nanos,omitempty"`
}

func (x *TimestampPair) Reset() {
	*x = TimestampPair{}
}

func (x *TimestampPair) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TimestampPair) ProtoMessage() {}

func (x *TimestampPair) ProtoReflect() protoreflect.Message {
	return nil
}

func (*TimestampPair) Descriptor() ([]byte, []int) {
	return nil, nil
}

func (x *TimestampPair) GetSeconds() int64 {
	if x != nil {
		return x.Seconds
	}
	return 0
}

func (x *TimestampPair) GetNanos() int32 {
	if x != nil {
		return x.Nanos
	}
	return 0
}
