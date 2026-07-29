package betool

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/rpcinfo"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

type shapeKind uint8

const (
	shapeScalar shapeKind = iota + 1
	shapeMessage
	shapeRepeated
	shapeConflict
)

// wireShape is the smallest schema description that preserves the positional
// batchexecute structure. It deliberately does not try to recover proto
// distinctions erased by JSON (enum vs int, bytes vs string, and bool vs int).
type wireShape struct {
	kind   shapeKind
	scalar descriptorpb.FieldDescriptorProto_Type
	fields map[int32]*wireShape
	elem   *wireShape
}

func betoolInferProto(opts betoolOptions) error {
	samples, err := readInferSamples(opts)
	if err != nil {
		return err
	}
	merged := mergeShapeSamples(samples)
	if merged == nil || merged.kind != shapeMessage {
		return fmt.Errorf("infer-proto: payload root is not a positional message array")
	}

	var fd *descriptorpb.FileDescriptorProto
	var set *descriptorpb.FileDescriptorSet
	rootName := ""
	if opts.descriptorFile != "" {
		var md protoreflect.MessageDescriptor
		set, fd, md, err = readInferDescriptor(opts.descriptorFile, opts.messageName)
		if err != nil {
			return err
		}
		rootName = string(md.FullName())
		root := findMessageProto(fd, rootName)
		if root == nil {
			return fmt.Errorf("infer-proto: message %s is not in %s", md.FullName(), fd.GetName())
		}
		mergeStaticMessage(root, md, merged, fd)
	} else {
		method, methodErr := resolveInferMethod(opts.rpcID)
		if methodErr == nil {
			fd = proto.Clone(protodesc.ToFileDescriptorProto(method.Response.Descriptor().ParentFile())).(*descriptorpb.FileDescriptorProto)
			rootName = string(method.Response.Descriptor().FullName())
			root := findMessageProto(fd, rootName)
			if root == nil {
				return fmt.Errorf("infer-proto: response descriptor %s is not in %s", method.Response.Descriptor().FullName(), fd.GetName())
			}
			mergeStaticMessage(root, method.Response.Descriptor(), merged, fd)
		} else {
			var unknown rpcinfo.ErrUnknownRPCID
			if !errors.As(methodErr, &unknown) {
				return methodErr
			}
			fd = inferredFile(opts.rpcID, merged)
			rootName = "betool.inferred.InferredMessage"
		}
	}

	if set == nil {
		set = &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{fd}}
	}
	if opts.outputFile != "" {
		data, err := proto.Marshal(set)
		if err != nil {
			return fmt.Errorf("infer-proto: encode descriptor set: %w", err)
		}
		if err := os.WriteFile(opts.outputFile, data, 0666); err != nil {
			return fmt.Errorf("infer-proto: write descriptor set: %w", err)
		}
	}
	if opts.asJSON {
		value := proto.Message(fd)
		if opts.descriptorFile != "" {
			value = set
		}
		b, err := (protojson.MarshalOptions{UseProtoNames: true, Multiline: true}).Marshal(value)
		if err != nil {
			return fmt.Errorf("infer-proto: marshal JSON: %w", err)
		}
		return writeText(string(b))
	}
	return writeText(renderFocusedProto(fd, rootName))
}

func readInferDescriptor(path, messageName string) (*descriptorpb.FileDescriptorSet, *descriptorpb.FileDescriptorProto, protoreflect.MessageDescriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("infer-proto: read descriptor: %w", err)
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(data, &set); err != nil {
		return nil, nil, nil, fmt.Errorf("infer-proto: decode descriptor set: %w", err)
	}
	files, err := protodesc.NewFiles(&set)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("infer-proto: load descriptor set: %w", err)
	}
	desc, err := files.FindDescriptorByName(protoreflect.FullName(strings.TrimPrefix(messageName, ".")))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("infer-proto: find message %s: %w", messageName, err)
	}
	md, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, nil, nil, fmt.Errorf("infer-proto: %s is not a message", messageName)
	}
	var source *descriptorpb.FileDescriptorProto
	for _, candidate := range set.GetFile() {
		if candidate.GetName() == md.ParentFile().Path() {
			source = candidate
			break
		}
	}
	if source == nil {
		return nil, nil, nil, fmt.Errorf("infer-proto: file %s is missing from descriptor set", md.ParentFile().Path())
	}
	return &set, source, md, nil
}

func resolveInferMethod(id string) (rpcinfo.Method, error) {
	method, err := rpcinfo.Lookup(id)
	if err == nil {
		return method, nil
	}
	var unknown rpcinfo.ErrUnknownRPCID
	if errors.As(err, &unknown) {
		return rpcinfo.Method{}, err
	}
	return rpcinfo.Method{}, err
}

func readInferSamples(opts betoolOptions) ([]any, error) {
	paths := append([]string(nil), opts.files...)
	if opts.samplesDir != "" {
		entries, err := os.ReadDir(opts.samplesDir)
		if err != nil {
			return nil, fmt.Errorf("infer-proto: read samples directory: %w", err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			paths = append(paths, filepath.Join(opts.samplesDir, entry.Name()))
		}
		sort.Strings(paths[len(opts.files):])
	}
	if len(paths) == 0 {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("infer-proto: read stdin: %w", err)
		}
		v, err := inferPayload(b, opts.rpcID)
		if err != nil {
			return nil, err
		}
		return []any{v}, nil
	}
	values := make([]any, 0, len(paths))
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("infer-proto: read %s: %w", path, err)
		}
		fileValues, err := inferFileSamples(b, opts.rpcID)
		if err != nil {
			return nil, fmt.Errorf("infer-proto: %s: %w", path, err)
		}
		values = append(values, fileValues...)
	}
	return values, nil
}

func inferFileSamples(data []byte, rpcID string) ([]any, error) {
	if value, err := inferPayload(data, rpcID); err == nil {
		return []any{value}, nil
	}
	if values, err := inferHAR(data, rpcID); err == nil && len(values) > 0 {
		return values, nil
	}
	if values, err := inferJSONL(data, rpcID); err == nil && len(values) > 0 {
		return values, nil
	}
	if bodies, err := inferHTTPRR(data); err == nil && len(bodies) > 0 {
		values := make([]any, 0, len(bodies))
		for _, body := range bodies {
			value, err := inferPayload(body, rpcID)
			if err != nil {
				continue
			}
			values = append(values, value)
		}
		if len(values) > 0 {
			return values, nil
		}
	}
	return nil, fmt.Errorf("unsupported payload, HAR, or httprr input")
}

type trafficEntry struct {
	Request struct {
		URL string `json:"url"`
	} `json:"request"`
	Response struct {
		Content struct {
			Text     string `json:"text"`
			Encoding string `json:"encoding"`
		} `json:"content"`
	} `json:"response"`
}

func trafficBody(entry trafficEntry) []byte {
	body := []byte(entry.Response.Content.Text)
	if strings.EqualFold(entry.Response.Content.Encoding, "base64") {
		decoded, err := base64.StdEncoding.DecodeString(entry.Response.Content.Text)
		if err != nil {
			return nil
		}
		return decoded
	}
	return body
}

func inferHAR(data []byte, rpcID string) ([]any, error) {
	var har struct {
		Log struct {
			Entries []trafficEntry `json:"entries"`
		} `json:"log"`
	}
	if err := json.Unmarshal(data, &har); err != nil {
		return nil, err
	}
	values := make([]any, 0, len(har.Log.Entries))
	for _, entry := range har.Log.Entries {
		if !trafficMatchesRPC(entry, rpcID) {
			continue
		}
		body := trafficBody(entry)
		if body == nil {
			continue
		}
		value, err := inferPayload(body, rpcID)
		if err != nil {
			continue
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("HAR contains no matching response payloads")
	}
	return values, nil
}

func inferJSONL(data []byte, rpcID string) ([]any, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	values := make([]any, 0)
	for scanner.Scan() {
		var entry trafficEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if !trafficMatchesRPC(entry, rpcID) {
			continue
		}
		body := trafficBody(entry)
		if body == nil {
			continue
		}
		value, err := inferPayload(body, rpcID)
		if err != nil {
			continue
		}
		values = append(values, value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("JSONL contains no matching response payloads")
	}
	return values, nil
}

func trafficMatchesRPC(entry trafficEntry, rpcID string) bool {
	ids := corpusRPCIDs(entry.Request.URL)
	if len(ids) == 0 {
		return true
	}
	for _, id := range ids {
		if id == rpcID {
			return true
		}
	}
	return false
}

func inferHTTPRR(data []byte) ([][]byte, error) {
	line, rest, ok := strings.Cut(string(data), "\n")
	if !ok || strings.TrimSpace(strings.TrimSuffix(line, "\r")) != "httprr trace v1" {
		return nil, fmt.Errorf("not an httprr recording")
	}
	bodies := make([][]byte, 0)
	for rest != "" {
		sizeLine, content, ok := strings.Cut(rest, "\n")
		if !ok {
			return nil, fmt.Errorf("httprr size line missing")
		}
		var requestSize, responseSize int
		if _, err := fmt.Sscanf(sizeLine, "%d %d", &requestSize, &responseSize); err != nil {
			return nil, fmt.Errorf("parse httprr sizes: %w", err)
		}
		if requestSize < 0 || responseSize < 0 || requestSize+responseSize > len(content) {
			return nil, fmt.Errorf("httprr sizes exceed recording")
		}
		responseData := []byte(content[requestSize : requestSize+responseSize])
		response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(responseData)), nil)
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr == nil {
				bodies = append(bodies, body)
			}
		}
		rest = content[requestSize+responseSize:]
	}
	return bodies, nil
}

func inferPayload(data []byte, rpcID string) (any, error) {
	if resp, err := batchexecute.DecodeResponse(string(data)); err == nil && len(resp.Responses) > 0 {
		for _, item := range resp.Responses {
			if item.ID == rpcID || len(resp.Responses) == 1 {
				var v any
				if err := json.Unmarshal(item.Data, &v); err != nil {
					return nil, fmt.Errorf("parse response data: %w", err)
				}
				return unwrapTop(v), nil
			}
		}
		return nil, fmt.Errorf("rpc_id %q not present in response", rpcID)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parse payload JSON: %w", err)
	}
	return unwrapTop(v), nil
}

func mergeShapeSamples(samples []any) *wireShape {
	var merged *wireShape
	for _, sample := range samples {
		shape := observeMessage(sample)
		merged = mergeShapes(merged, shape)
	}
	return merged
}

func observeMessage(v any) *wireShape {
	arr, ok := v.([]any)
	if !ok {
		return observeValue(v)
	}
	s := &wireShape{kind: shapeMessage, fields: make(map[int32]*wireShape)}
	for i, value := range arr {
		if value == nil {
			continue
		}
		s.fields[int32(i+1)] = mergeShapes(s.fields[int32(i+1)], observeValue(value))
	}
	return s
}

func observeValue(v any) *wireShape {
	switch value := v.(type) {
	case nil:
		return nil
	case string:
		return &wireShape{kind: shapeScalar, scalar: descriptorpb.FieldDescriptorProto_TYPE_STRING}
	case bool:
		return &wireShape{kind: shapeScalar, scalar: descriptorpb.FieldDescriptorProto_TYPE_BOOL}
	case float64:
		if math.Trunc(value) == value {
			return &wireShape{kind: shapeScalar, scalar: descriptorpb.FieldDescriptorProto_TYPE_INT64}
		}
		return &wireShape{kind: shapeScalar, scalar: descriptorpb.FieldDescriptorProto_TYPE_DOUBLE}
	case []any:
		if isMessageArray(value) {
			return observeMessage(value)
		}
		var elem *wireShape
		for _, item := range value {
			elem = mergeShapes(elem, observeValue(item))
		}
		return &wireShape{kind: shapeRepeated, elem: elem}
	default:
		return &wireShape{kind: shapeScalar, scalar: descriptorpb.FieldDescriptorProto_TYPE_STRING}
	}
}

func isMessageArray(arr []any) bool {
	for _, item := range arr {
		if item == nil {
			return true
		}
	}
	if len(arr) <= 1 {
		return true
	}
	var first string
	for i, item := range arr {
		sig := shapeSignature(observeValue(item))
		if i == 0 {
			first = sig
		} else if sig != first {
			return true
		}
	}
	return false
}

func mergeShapes(a, b *wireShape) *wireShape {
	if a == nil {
		return cloneShape(b)
	}
	if b == nil {
		return a
	}
	if a.kind == shapeMessage && b.kind == shapeRepeated {
		if field := a.fields[1]; field != nil {
			return &wireShape{kind: shapeRepeated, elem: mergeShapes(field, b.elem)}
		}
		return a
	}
	if a.kind == shapeRepeated && b.kind == shapeMessage {
		if field := b.fields[1]; field != nil {
			return &wireShape{kind: shapeRepeated, elem: mergeShapes(a.elem, field)}
		}
		return a
	}
	if a.kind != b.kind {
		return &wireShape{kind: shapeConflict}
	}
	switch a.kind {
	case shapeScalar:
		if a.scalar == b.scalar {
			return a
		}
		return &wireShape{kind: shapeConflict}
	case shapeMessage:
		for number, field := range b.fields {
			a.fields[number] = mergeShapes(a.fields[number], field)
		}
	case shapeRepeated:
		a.elem = mergeShapes(a.elem, b.elem)
	case shapeConflict:
		return a
	}
	return a
}

func cloneShape(s *wireShape) *wireShape {
	if s == nil {
		return nil
	}
	c := &wireShape{kind: s.kind, scalar: s.scalar, elem: cloneShape(s.elem)}
	if s.fields != nil {
		c.fields = make(map[int32]*wireShape, len(s.fields))
		for number, field := range s.fields {
			c.fields[number] = cloneShape(field)
		}
	}
	return c
}

func shapeSignature(s *wireShape) string {
	if s == nil {
		return "null"
	}
	switch s.kind {
	case shapeScalar:
		return "scalar:" + s.scalar.String()
	case shapeRepeated:
		return "repeated:" + shapeSignature(s.elem)
	case shapeMessage:
		var b strings.Builder
		b.WriteString("message:")
		for _, number := range sortedShapeFields(s) {
			fmt.Fprintf(&b, "%d=%s;", number, shapeSignature(s.fields[number]))
		}
		return b.String()
	case shapeConflict:
		return "conflict"
	default:
		return "unknown"
	}
}

func sortedShapeFields(s *wireShape) []int32 {
	numbers := make([]int32, 0, len(s.fields))
	for number := range s.fields {
		numbers = append(numbers, number)
	}
	sort.Slice(numbers, func(i, j int) bool { return numbers[i] < numbers[j] })
	return numbers
}

func mergeStaticMessage(dst *descriptorpb.DescriptorProto, md protoreflect.MessageDescriptor, shape *wireShape, fd *descriptorpb.FileDescriptorProto) {
	if shape == nil || shape.kind != shapeMessage {
		return
	}
	for _, number := range sortedShapeFields(shape) {
		fieldShape := shape.fields[number]
		field := md.Fields().ByNumber(protoreflect.FieldNumber(number))
		if field != nil {
			if field.Message() != nil && !isFlattenedWellKnown(field.Message()) {
				child := fieldShape
				if child.kind == shapeRepeated {
					child = child.elem
				}
				if child != nil && child.kind == shapeMessage {
					if childProto := findMessageProto(fd, string(field.Message().FullName())); childProto != nil {
						mergeStaticMessage(childProto, field.Message(), child, fd)
					}
				}
			}
			continue
		}
		appendSyntheticField(dst, number, fieldShape, fullMessageName(fd, md), fd)
	}
}

func appendSyntheticField(dst *descriptorpb.DescriptorProto, number int32, shape *wireShape, parent string, fd *descriptorpb.FileDescriptorProto) {
	if shape == nil || shape.kind == shapeConflict {
		return
	}
	field := &descriptorpb.FieldDescriptorProto{
		Name:   proto.String(fmt.Sprintf("unknown_%d", number)),
		Number: proto.Int32(number),
		Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
	}
	valueShape := shape
	if shape.kind == shapeRepeated {
		field.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
		valueShape = shape.elem
	}
	if valueShape == nil {
		return
	}
	if valueShape.kind == shapeConflict {
		return
	}
	if valueShape.kind == shapeMessage {
		name := uniqueNestedName(dst, fmt.Sprintf("Unknown%d", number))
		dst.NestedType = append(dst.NestedType, &descriptorpb.DescriptorProto{Name: proto.String(name)})
		field.Type = descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum()
		field.TypeName = proto.String(parent + "." + name)
		child := dst.NestedType[len(dst.NestedType)-1]
		appendShapeFields(child, valueShape, parent+"."+name, fd)
	} else {
		field.Type = valueShape.scalar.Enum()
	}
	dst.Field = append(dst.Field, field)
}

func appendShapeFields(dst *descriptorpb.DescriptorProto, shape *wireShape, parent string, fd *descriptorpb.FileDescriptorProto) {
	if shape == nil || shape.kind != shapeMessage {
		return
	}
	for _, number := range sortedShapeFields(shape) {
		appendSyntheticField(dst, number, shape.fields[number], parent, fd)
	}
}

func inferredFile(rpcID string, shape *wireShape) *descriptorpb.FileDescriptorProto {
	name := "betool_inferred.proto"
	if rpcID != "" {
		name = "betool_" + sanitizeName(rpcID) + ".proto"
	}
	const pkg = "betool.inferred"
	const rootName = "InferredMessage"
	root := &descriptorpb.DescriptorProto{Name: proto.String(rootName)}
	appendShapeFields(root, shape, "."+pkg+"."+rootName, nil)
	return &descriptorpb.FileDescriptorProto{
		Name:        proto.String(name),
		Package:     proto.String(pkg),
		Syntax:      proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{root},
	}
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "rpc"
	}
	return b.String()
}

func uniqueNestedName(dst *descriptorpb.DescriptorProto, base string) string {
	name := base
	for i := 2; ; i++ {
		found := false
		for _, nested := range dst.NestedType {
			if nested.GetName() == name {
				found = true
				break
			}
		}
		if !found {
			return name
		}
		name = fmt.Sprintf("%s_%d", base, i)
	}
}

func findMessageProto(fd *descriptorpb.FileDescriptorProto, fullName string) *descriptorpb.DescriptorProto {
	prefix := fd.GetPackage()
	for _, message := range fd.GetMessageType() {
		if found := findMessageProtoNested(message, prefix, fullName); found != nil {
			return found
		}
	}
	return nil
}

func findMessageProtoNested(message *descriptorpb.DescriptorProto, prefix, target string) *descriptorpb.DescriptorProto {
	full := message.GetName()
	if prefix != "" {
		full = prefix + "." + full
	}
	if full == target {
		return message
	}
	for _, nested := range message.GetNestedType() {
		if found := findMessageProtoNested(nested, full, target); found != nil {
			return found
		}
	}
	return nil
}

func fullMessageName(fd *descriptorpb.FileDescriptorProto, md protoreflect.MessageDescriptor) string {
	return "." + string(md.FullName())
}

// renderFocusedProto prints the selected message and declarations reachable
// from its fields. FileDescriptorProto is intentionally not used for the
// default text form: it describes the whole generated file, while an operator
// inspecting one response generally wants the message carried by that RPC.
func renderFocusedProto(fd *descriptorpb.FileDescriptorProto, rootName string) string {
	messages := make(map[string]*descriptorpb.DescriptorProto)
	enums := make(map[string]*descriptorpb.EnumDescriptorProto)
	for _, m := range fd.GetMessageType() {
		collectMessageProtos(m, fd.GetPackage(), messages, enums)
	}
	var b strings.Builder
	b.WriteString("syntax = \"proto3\";\n")
	if fd.GetPackage() != "" {
		fmt.Fprintf(&b, "package %s;\n", fd.GetPackage())
	}
	imports := make(map[string]bool)
	root := messages[rootName]
	if root != nil {
		collectImports(root, messages, enums, imports)
	}
	for _, dep := range fd.GetDependency() {
		if imports[dep] {
			fmt.Fprintf(&b, "import %q;\n", dep)
		}
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	if root == nil {
		return b.String()
	}
	writtenM, writtenE := map[string]bool{}, map[string]bool{}
	writeMessageProto(&b, root, rootName, messages, enums, writtenM, writtenE, 0)
	return b.String()
}

func collectMessageProtos(m *descriptorpb.DescriptorProto, prefix string, messages map[string]*descriptorpb.DescriptorProto, enums map[string]*descriptorpb.EnumDescriptorProto) {
	name := m.GetName()
	if prefix != "" {
		name = prefix + "." + name
	}
	messages[name] = m
	for _, e := range m.GetEnumType() {
		enums[name+"."+e.GetName()] = e
	}
	for _, n := range m.GetNestedType() {
		collectMessageProtos(n, name, messages, enums)
	}
}

func collectImports(m *descriptorpb.DescriptorProto, messages map[string]*descriptorpb.DescriptorProto, enums map[string]*descriptorpb.EnumDescriptorProto, imports map[string]bool) {
	for _, f := range m.GetField() {
		if f.GetTypeName() == ".google.protobuf.Timestamp" || f.GetTypeName() == ".google.protobuf.Duration" || strings.HasPrefix(f.GetTypeName(), ".google.protobuf.FieldMask") {
			imports["google/protobuf/"+strings.ToLower(strings.TrimPrefix(f.GetTypeName(), ".google.protobuf."))+".proto"] = true
		}
		if strings.HasPrefix(f.GetTypeName(), ".") {
			if n := messages[strings.TrimPrefix(f.GetTypeName(), ".")]; n != nil {
				collectImports(n, messages, enums, imports)
			}
		}
	}
}

func writeMessageProto(b *strings.Builder, m *descriptorpb.DescriptorProto, fullName string, messages map[string]*descriptorpb.DescriptorProto, enums map[string]*descriptorpb.EnumDescriptorProto, writtenM, writtenE map[string]bool, indent int) {
	if writtenM[fullName] {
		return
	}
	writtenM[fullName] = true
	pad := strings.Repeat("  ", indent)
	fmt.Fprintf(b, "%smessage %s {\n", pad, m.GetName())
	for _, e := range m.GetEnumType() {
		writeEnumProto(b, e, fullName+"."+e.GetName(), indent+1, writtenE)
	}
	for _, f := range m.GetField() {
		label := ""
		if f.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
			label = "repeated "
		}
		typ := protoFieldType(f, fullName, messages, enums)
		fmt.Fprintf(b, "%s%s%s %s = %d;\n", strings.Repeat("  ", indent+1), label, typ, f.GetName(), f.GetNumber())
	}
	for _, o := range m.GetOneofDecl() {
		fmt.Fprintf(b, "%soneof %s {\n", strings.Repeat("  ", indent+1), o.GetName())
		for _, f := range m.GetField() {
			if f.GetOneofIndex() >= 0 && int(f.GetOneofIndex()) < len(m.GetOneofDecl()) && m.GetOneofDecl()[f.GetOneofIndex()] == o {
				fmt.Fprintf(b, "%s%s %s = %d;\n", strings.Repeat("  ", indent+2), protoFieldType(f, fullName, messages, enums), f.GetName(), f.GetNumber())
			}
		}
		fmt.Fprintf(b, "%s}\n", strings.Repeat("  ", indent+1))
	}
	for _, n := range m.GetNestedType() {
		writeMessageProto(b, n, fullName+"."+n.GetName(), messages, enums, writtenM, writtenE, indent+1)
	}
	fmt.Fprintf(b, "%s}\n\n", pad)
}

func writeEnumProto(b *strings.Builder, e *descriptorpb.EnumDescriptorProto, fullName string, indent int, written map[string]bool) {
	if written[fullName] {
		return
	}
	written[fullName] = true
	pad := strings.Repeat("  ", indent)
	fmt.Fprintf(b, "%senum %s {\n", pad, e.GetName())
	for _, v := range e.GetValue() {
		fmt.Fprintf(b, "%s%s = %d;\n", strings.Repeat("  ", indent+1), v.GetName(), v.GetNumber())
	}
	fmt.Fprintf(b, "%s}\n", pad)
}

func protoFieldType(f *descriptorpb.FieldDescriptorProto, parent string, messages map[string]*descriptorpb.DescriptorProto, enums map[string]*descriptorpb.EnumDescriptorProto) string {
	if f.GetTypeName() != "" {
		name := strings.TrimPrefix(f.GetTypeName(), ".")
		prefix := parent
		for prefix != "" {
			if strings.HasPrefix(name, prefix+".") {
				return strings.TrimPrefix(name, prefix+".")
			}
			if i := strings.LastIndex(prefix, "."); i >= 0 {
				prefix = prefix[:i]
			} else {
				prefix = ""
			}
		}
		return name
	}
	switch f.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
		return "double"
	case descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		return "float"
	case descriptorpb.FieldDescriptorProto_TYPE_INT64:
		return "int64"
	case descriptorpb.FieldDescriptorProto_TYPE_UINT64:
		return "uint64"
	case descriptorpb.FieldDescriptorProto_TYPE_INT32:
		return "int32"
	case descriptorpb.FieldDescriptorProto_TYPE_FIXED64:
		return "fixed64"
	case descriptorpb.FieldDescriptorProto_TYPE_FIXED32:
		return "fixed32"
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return "bool"
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		return "string"
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return "bytes"
	case descriptorpb.FieldDescriptorProto_TYPE_UINT32:
		return "uint32"
	case descriptorpb.FieldDescriptorProto_TYPE_SFIXED32:
		return "sfixed32"
	case descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
		return "sfixed64"
	case descriptorpb.FieldDescriptorProto_TYPE_SINT32:
		return "sint32"
	case descriptorpb.FieldDescriptorProto_TYPE_SINT64:
		return "sint64"
	}
	return "string"
}
