package betool

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tmc/nlm/internal/batchexecute"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

type corpusBinding struct {
	method   string
	request  protoreflect.MessageDescriptor
	response protoreflect.MessageDescriptor
}

type corpusSample struct {
	rpcID string
	side  string
	value any
}

type augmentReport struct {
	Descriptor          string            `json:"descriptor"`
	Output              string            `json:"output"`
	RPCOption           string            `json:"rpc_option"`
	BooleanOption       string            `json:"boolean_option,omitempty"`
	MinimumObservations int               `json:"minimum_observations"`
	Files               int               `json:"files"`
	Records             int               `json:"records"`
	Payloads            int               `json:"payloads"`
	MatchedPayloads     int               `json:"matched_payloads"`
	UnknownRPCs         int               `json:"unknown_rpcs"`
	AmbiguousPayloads   int               `json:"ambiguous_payloads"`
	ParserFailures      int               `json:"parser_failures"`
	Added               []augmentFinding  `json:"added,omitempty"`
	Annotated           []augmentFinding  `json:"annotated,omitempty"`
	Presence            []augmentFinding  `json:"presence,omitempty"`
	Conflicts           []augmentFinding  `json:"conflicts,omitempty"`
	Insufficient        []augmentFinding  `json:"insufficient,omitempty"`
	Methods             map[string]string `json:"methods"`
}

type augmentFinding struct {
	Message      string `json:"message"`
	Field        int32  `json:"field"`
	Name         string `json:"name,omitempty"`
	Type         string `json:"type,omitempty"`
	Observations int    `json:"observations"`
	Reason       string `json:"reason,omitempty"`
}

func betoolAugmentCorpus(opts betoolOptions) error {
	set, files, err := readDescriptorSet(opts.descriptorFile)
	if err != nil {
		return err
	}
	bindings, err := descriptorRPCBindings(set, files, opts.rpcOption)
	if err != nil {
		return err
	}
	var booleanOption protoreflect.FieldNumber
	if opts.booleanOption != "" {
		booleanOption, err = descriptorBooleanOption(files, opts.booleanOption)
		if err != nil {
			return err
		}
	}
	samples, report, err := readAugmentCorpus(opts.files)
	if err != nil {
		return err
	}
	report.Descriptor = opts.descriptorFile
	report.Output = opts.outputFile
	report.RPCOption = opts.rpcOption
	report.BooleanOption = opts.booleanOption
	report.MinimumObservations = opts.minObservations
	report.Files = len(opts.files)
	report.Methods = make(map[string]string)

	observed := make(map[protoreflect.FullName]*wireShape)
	for _, sample := range samples {
		report.Payloads++
		candidates := bindings[sample.rpcID]
		if len(candidates) == 0 {
			report.UnknownRPCs++
			continue
		}
		md, method, ok := unambiguousBinding(candidates, sample.side)
		if !ok {
			report.AmbiguousPayloads++
			continue
		}
		report.MatchedPayloads++
		report.Methods[sample.rpcID+" "+sample.side] = method
		shape := observeMessage(unwrapTop(sample.value))
		collectMessageShapes(md, shape, observed)
	}

	locations := descriptorMessageLocations(set)
	names := make([]string, 0, len(observed))
	for name := range observed {
		names = append(names, string(name))
	}
	sort.Strings(names)
	for _, name := range names {
		md, err := files.FindDescriptorByName(protoreflect.FullName(name))
		if err != nil {
			continue
		}
		message, ok := md.(protoreflect.MessageDescriptor)
		if !ok {
			continue
		}
		location := locations[message.FullName()]
		if location.message == nil {
			continue
		}
		mergeObservedFields(&report, location, message, observed[message.FullName()], booleanOption, opts.minObservations)
	}

	data, err := proto.Marshal(set)
	if err != nil {
		return fmt.Errorf("augment-corpus: encode descriptor set: %w", err)
	}
	if err := os.WriteFile(opts.outputFile, data, 0666); err != nil {
		return fmt.Errorf("augment-corpus: write descriptor set: %w", err)
	}
	sort.Slice(report.Added, func(i, j int) bool {
		if report.Added[i].Message != report.Added[j].Message {
			return report.Added[i].Message < report.Added[j].Message
		}
		return report.Added[i].Field < report.Added[j].Field
	})
	sort.Slice(report.Conflicts, func(i, j int) bool {
		if report.Conflicts[i].Message != report.Conflicts[j].Message {
			return report.Conflicts[i].Message < report.Conflicts[j].Message
		}
		return report.Conflicts[i].Field < report.Conflicts[j].Field
	})
	sort.Slice(report.Annotated, func(i, j int) bool {
		if report.Annotated[i].Message != report.Annotated[j].Message {
			return report.Annotated[i].Message < report.Annotated[j].Message
		}
		return report.Annotated[i].Field < report.Annotated[j].Field
	})
	sort.Slice(report.Presence, func(i, j int) bool {
		if report.Presence[i].Message != report.Presence[j].Message {
			return report.Presence[i].Message < report.Presence[j].Message
		}
		return report.Presence[i].Field < report.Presence[j].Field
	})
	sort.Slice(report.Insufficient, func(i, j int) bool {
		if report.Insufficient[i].Message != report.Insufficient[j].Message {
			return report.Insufficient[i].Message < report.Insufficient[j].Message
		}
		return report.Insufficient[i].Field < report.Insufficient[j].Field
	})
	if opts.asJSON {
		return writeJSON(report)
	}
	fmt.Fprintf(os.Stdout, "records=%d payloads=%d matched=%d added=%d annotated=%d presence=%d conflicts=%d insufficient=%d unknown=%d ambiguous=%d parser_failures=%d\n",
		report.Records, report.Payloads, report.MatchedPayloads, len(report.Added), len(report.Annotated), len(report.Presence), len(report.Conflicts), len(report.Insufficient),
		report.UnknownRPCs, report.AmbiguousPayloads, report.ParserFailures)
	return nil
}

func descriptorBooleanOption(files *protoregistry.Files, optionName string) (protoreflect.FieldNumber, error) {
	desc, err := files.FindDescriptorByName(protoreflect.FullName(strings.TrimPrefix(optionName, ".")))
	if err != nil {
		return 0, fmt.Errorf("augment-corpus: find boolean option %s: %w", optionName, err)
	}
	extension, ok := desc.(protoreflect.ExtensionDescriptor)
	if !ok || extension.Kind() != protoreflect.BoolKind || extension.ContainingMessage().FullName() != "google.protobuf.FieldOptions" {
		return 0, fmt.Errorf("augment-corpus: boolean option %s is not a bool extension of google.protobuf.FieldOptions", optionName)
	}
	return extension.Number(), nil
}

func readDescriptorSet(path string) (*descriptorpb.FileDescriptorSet, *protoregistry.Files, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("augment-corpus: read descriptor: %w", err)
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(data, &set); err != nil {
		return nil, nil, fmt.Errorf("augment-corpus: decode descriptor set: %w", err)
	}
	files, err := protodesc.NewFiles(&set)
	if err != nil {
		return nil, nil, fmt.Errorf("augment-corpus: load descriptor set: %w", err)
	}
	return &set, files, nil
}

func descriptorRPCBindings(set *descriptorpb.FileDescriptorSet, files *protoregistry.Files, optionName string) (map[string][]corpusBinding, error) {
	desc, err := files.FindDescriptorByName(protoreflect.FullName(strings.TrimPrefix(optionName, ".")))
	if err != nil {
		return nil, fmt.Errorf("augment-corpus: find RPC option %s: %w", optionName, err)
	}
	extension, ok := desc.(protoreflect.ExtensionDescriptor)
	if !ok || extension.Kind() != protoreflect.StringKind || extension.ContainingMessage().FullName() != "google.protobuf.MethodOptions" {
		return nil, fmt.Errorf("augment-corpus: RPC option %s is not a string extension of google.protobuf.MethodOptions", optionName)
	}
	out := make(map[string][]corpusBinding)
	for _, file := range set.GetFile() {
		for _, service := range file.GetService() {
			serviceName := service.GetName()
			if file.GetPackage() != "" {
				serviceName = file.GetPackage() + "." + serviceName
			}
			for _, method := range service.GetMethod() {
				id, err := methodOptionString(method.GetOptions(), extension.Number())
				if err != nil {
					return nil, fmt.Errorf("augment-corpus: %s.%s: %w", serviceName, method.GetName(), err)
				}
				if id == "" {
					continue
				}
				request, err := descriptorMessage(files, method.GetInputType())
				if err != nil {
					return nil, err
				}
				response, err := descriptorMessage(files, method.GetOutputType())
				if err != nil {
					return nil, err
				}
				out[id] = append(out[id], corpusBinding{
					method:   serviceName + "." + method.GetName(),
					request:  request,
					response: response,
				})
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("augment-corpus: no methods carry RPC option %s", optionName)
	}
	return out, nil
}

func methodOptionString(options *descriptorpb.MethodOptions, number protoreflect.FieldNumber) (string, error) {
	if options == nil {
		return "", nil
	}
	data, err := proto.Marshal(options)
	if err != nil {
		return "", err
	}
	for len(data) > 0 {
		fieldNumber, wireType, n := protowire.ConsumeTag(data)
		if n < 0 {
			return "", protowire.ParseError(n)
		}
		data = data[n:]
		if fieldNumber == protowire.Number(number) {
			if wireType != protowire.BytesType {
				return "", fmt.Errorf("RPC option field %d has wire type %d, want bytes", number, wireType)
			}
			value, n := protowire.ConsumeString(data)
			if n < 0 {
				return "", protowire.ParseError(n)
			}
			return value, nil
		}
		n = protowire.ConsumeFieldValue(fieldNumber, wireType, data)
		if n < 0 {
			return "", protowire.ParseError(n)
		}
		data = data[n:]
	}
	return "", nil
}

func descriptorMessage(files *protoregistry.Files, name string) (protoreflect.MessageDescriptor, error) {
	desc, err := files.FindDescriptorByName(protoreflect.FullName(strings.TrimPrefix(name, ".")))
	if err != nil {
		return nil, fmt.Errorf("augment-corpus: find message %s: %w", name, err)
	}
	message, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, fmt.Errorf("augment-corpus: %s is not a message", name)
	}
	return message, nil
}

func unambiguousBinding(bindings []corpusBinding, side string) (protoreflect.MessageDescriptor, string, bool) {
	var message protoreflect.MessageDescriptor
	var method string
	for _, binding := range bindings {
		candidate := binding.response
		if side == "request" {
			candidate = binding.request
		}
		if message == nil {
			message = candidate
			method = binding.method
			continue
		}
		if message.FullName() != candidate.FullName() {
			return nil, "", false
		}
	}
	return message, method, message != nil
}

func readAugmentCorpus(paths []string) ([]corpusSample, augmentReport, error) {
	var samples []corpusSample
	var report augmentReport
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, report, fmt.Errorf("augment-corpus: read %s: %w", path, err)
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
		for scanner.Scan() {
			report.Records++
			var entry corpusTrafficEntry
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
				report.ParserFailures++
				continue
			}
			entrySamples, err := augmentEntrySamples(entry)
			if err != nil {
				report.ParserFailures++
				continue
			}
			samples = append(samples, entrySamples...)
		}
		if err := scanner.Err(); err != nil {
			return nil, report, fmt.Errorf("augment-corpus: scan %s: %w", path, err)
		}
	}
	return samples, report, nil
}

func augmentEntrySamples(entry corpusTrafficEntry) ([]corpusSample, error) {
	if rpcID := corpusStreamRPCID(entry.Request.URL); rpcID != "" {
		return augmentStreamSamples(entry, rpcID)
	}
	if len(corpusRPCIDs(entry.Request.URL)) == 0 {
		return nil, nil
	}
	var samples []corpusSample
	requestBody, err := base64.StdEncoding.DecodeString(entry.Request.PostData.Text)
	if err != nil {
		requestBody = []byte(entry.Request.PostData.Text)
	}
	if request, err := batchexecute.DecodeRequest(string(requestBody)); err == nil {
		for _, call := range request.RPCs {
			var value any
			if err := json.Unmarshal(call.Args, &value); err == nil {
				samples = append(samples, corpusSample{rpcID: call.ID, side: "request", value: value})
			}
		}
	}

	responseBody := []byte(entry.Response.Content.Text)
	if strings.EqualFold(entry.Response.Content.Encoding, "base64") {
		decoded, err := base64.StdEncoding.DecodeString(entry.Response.Content.Text)
		if err != nil {
			return samples, err
		}
		responseBody = decoded
	}
	response, err := batchexecute.DecodeResponse(string(responseBody))
	if err != nil {
		if len(samples) > 0 {
			return samples, nil
		}
		return nil, err
	}
	expected := corpusRPCIDs(entry.Request.URL)
	for i, call := range response.Responses {
		if call.Status != 0 || call.Error != "" || len(call.Data) == 0 {
			continue
		}
		id := call.ID
		if id == "" && len(expected) == 1 {
			id = expected[0]
		} else if id == "" && len(expected) == len(response.Responses) {
			id = expected[i]
		}
		if id == "" {
			continue
		}
		var value any
		if err := json.Unmarshal(call.Data, &value); err == nil {
			samples = append(samples, corpusSample{rpcID: id, side: "response", value: value})
		}
	}
	return samples, nil
}

func augmentStreamSamples(entry corpusTrafficEntry, rpcID string) ([]corpusSample, error) {
	var samples []corpusSample
	requestBody, err := base64.StdEncoding.DecodeString(entry.Request.PostData.Text)
	if err != nil {
		requestBody = []byte(entry.Request.PostData.Text)
	}
	if wire, err := decodeWrbFRRequest(requestBody); err == nil {
		var value any
		if err := json.Unmarshal(wire, &value); err == nil {
			samples = append(samples, corpusSample{rpcID: rpcID, side: "request", value: value})
		}
	}

	responseBody := []byte(entry.Response.Content.Text)
	if strings.EqualFold(entry.Response.Content.Encoding, "base64") {
		decoded, err := base64.StdEncoding.DecodeString(entry.Response.Content.Text)
		if err != nil {
			return samples, err
		}
		responseBody = decoded
	}
	response, _, err := decodeWrbFRStream(responseBody, rpcID)
	if err != nil {
		if len(samples) > 0 {
			return samples, nil
		}
		return nil, err
	}
	if len(response.Responses) == 0 {
		return samples, nil
	}
	call := response.Responses[0]
	if call.Status != 0 || call.Error != "" || len(call.Data) == 0 {
		return samples, nil
	}
	var value any
	if err := json.Unmarshal(call.Data, &value); err == nil {
		samples = append(samples, corpusSample{rpcID: rpcID, side: "response", value: value})
	}
	return samples, nil
}

func collectMessageShapes(message protoreflect.MessageDescriptor, shape *wireShape, observed map[protoreflect.FullName]*wireShape) {
	if shape == nil || shape.kind != shapeMessage {
		return
	}
	observed[message.FullName()] = mergeShapes(observed[message.FullName()], shape)
	for number, fieldShape := range shape.fields {
		field := message.Fields().ByNumber(protoreflect.FieldNumber(number))
		if field == nil || field.Message() == nil || isFlattenedWellKnown(field.Message()) {
			continue
		}
		child := fieldShape
		if child.kind == shapeRepeated {
			child = child.elem
		}
		collectMessageShapes(field.Message(), child, observed)
	}
}

type messageLocation struct {
	file    *descriptorpb.FileDescriptorProto
	message *descriptorpb.DescriptorProto
}

func descriptorMessageLocations(set *descriptorpb.FileDescriptorSet) map[protoreflect.FullName]messageLocation {
	out := make(map[protoreflect.FullName]messageLocation)
	for _, file := range set.GetFile() {
		for _, message := range file.GetMessageType() {
			collectMessageLocations(out, file, protoreflect.FullName(file.GetPackage()), message)
		}
	}
	return out
}

func collectMessageLocations(out map[protoreflect.FullName]messageLocation, file *descriptorpb.FileDescriptorProto, prefix protoreflect.FullName, message *descriptorpb.DescriptorProto) {
	name := protoreflect.FullName(message.GetName())
	if prefix != "" {
		name = prefix.Append(protoreflect.Name(message.GetName()))
	}
	out[name] = messageLocation{file: file, message: message}
	for _, nested := range message.GetNestedType() {
		collectMessageLocations(out, file, name, nested)
	}
}

func mergeObservedFields(report *augmentReport, location messageLocation, message protoreflect.MessageDescriptor, shape *wireShape, booleanOption protoreflect.FieldNumber, minimum int) {
	if shape == nil || shape.kind != shapeMessage {
		return
	}
	for _, number := range sortedShapeFields(shape) {
		fieldShape := shape.fields[number]
		field := message.Fields().ByNumber(protoreflect.FieldNumber(number))
		if field != nil {
			protoField := protoFieldByNumber(location.message, number)
			if fieldShape.defaultCount > 0 && descriptorAcceptsShape(field, fieldShape) && protoField != nil {
				if fieldShape.defaultCount >= minimum {
					if setFieldPresence(location, field, protoField) {
						report.Presence = append(report.Presence, augmentFinding{
							Message: string(message.FullName()), Field: number, Name: protoField.GetName(),
							Type: descriptorFieldType(protoField), Observations: fieldShape.defaultCount,
						})
					}
				} else if !field.HasPresence() && field.Cardinality() != protoreflect.Repeated && field.Message() == nil {
					report.Insufficient = append(report.Insufficient, augmentFinding{
						Message: string(message.FullName()), Field: number, Name: protoField.GetName(),
						Type: descriptorFieldType(protoField), Observations: fieldShape.defaultCount, Reason: "presence below minimum observations",
					})
				}
			}
			if booleanOption != 0 && field.Kind() == protoreflect.BoolKind && fieldShape.kind == shapeScalar && fieldShape.scalar == descriptorpb.FieldDescriptorProto_TYPE_BOOL && protoField != nil {
				if fieldShape.count >= minimum {
					if setBooleanFieldOption(protoField, booleanOption) {
						report.Annotated = append(report.Annotated, augmentFinding{
							Message: string(message.FullName()), Field: number, Name: protoField.GetName(),
							Type: "BOOL", Observations: fieldShape.count,
						})
					}
				} else if !wireFieldPresent(marshalFieldOptions(protoField.GetOptions()), protowire.Number(booleanOption)) {
					report.Insufficient = append(report.Insufficient, augmentFinding{
						Message: string(message.FullName()), Field: number, Name: protoField.GetName(),
						Type: "BOOL", Observations: fieldShape.count, Reason: "boolean annotation below minimum observations",
					})
				}
			}
			if !descriptorAcceptsShape(field, fieldShape) {
				report.Conflicts = append(report.Conflicts, augmentFinding{
					Message: string(message.FullName()), Field: number, Name: string(field.Name()),
					Type: shapeSignature(fieldShape), Observations: fieldShape.count, Reason: "descriptor shape mismatch",
				})
			}
			continue
		}
		if fieldShape == nil || fieldShape.kind == shapeConflict || fieldShape.kind == shapeRepeated && (fieldShape.elem == nil || fieldShape.elem.kind == shapeConflict) {
			report.Conflicts = append(report.Conflicts, augmentFinding{
				Message: string(message.FullName()), Field: number,
				Type: shapeSignature(fieldShape), Observations: shapeCount(fieldShape), Reason: "conflicting observations",
			})
			continue
		}
		if fieldShape.count < minimum {
			report.Insufficient = append(report.Insufficient, augmentFinding{
				Message: string(message.FullName()), Field: number,
				Type: shapeSignature(fieldShape), Observations: fieldShape.count, Reason: "below minimum observations",
			})
			continue
		}
		appendSyntheticField(location.message, number, fieldShape, "."+string(message.FullName()), location.file)
		added := protoFieldByNumber(location.message, number)
		if added == nil {
			continue
		}
		report.Added = append(report.Added, augmentFinding{
			Message: string(message.FullName()), Field: number, Name: added.GetName(),
			Type: descriptorFieldType(added), Observations: fieldShape.count,
		})
		if fieldShape.defaultCount >= minimum && setSyntheticFieldPresence(location, added) {
			report.Presence = append(report.Presence, augmentFinding{
				Message: string(message.FullName()), Field: number, Name: added.GetName(),
				Type: descriptorFieldType(added), Observations: fieldShape.defaultCount,
			})
		}
		if booleanOption != 0 && added.GetType() == descriptorpb.FieldDescriptorProto_TYPE_BOOL && setBooleanFieldOption(added, booleanOption) {
			report.Annotated = append(report.Annotated, augmentFinding{
				Message: string(message.FullName()), Field: number, Name: added.GetName(),
				Type: "BOOL", Observations: fieldShape.count,
			})
		}
	}
}

func setFieldPresence(location messageLocation, field protoreflect.FieldDescriptor, protoField *descriptorpb.FieldDescriptorProto) bool {
	if location.file.GetSyntax() != "proto3" || field.HasPresence() || field.Cardinality() == protoreflect.Repeated || field.Message() != nil {
		return false
	}
	return setProto3Optional(location.message, protoField)
}

func setSyntheticFieldPresence(location messageLocation, field *descriptorpb.FieldDescriptorProto) bool {
	if location.file.GetSyntax() != "proto3" || field.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED || field.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE || field.GetType() == descriptorpb.FieldDescriptorProto_TYPE_GROUP {
		return false
	}
	return setProto3Optional(location.message, field)
}

func setProto3Optional(message *descriptorpb.DescriptorProto, field *descriptorpb.FieldDescriptorProto) bool {
	if field.GetProto3Optional() || field.OneofIndex != nil {
		return false
	}
	name := "_" + field.GetName()
	used := make(map[string]bool, len(message.GetOneofDecl()))
	for _, oneof := range message.GetOneofDecl() {
		used[oneof.GetName()] = true
	}
	if used[name] {
		for suffix := 2; ; suffix++ {
			candidate := fmt.Sprintf("_%s_%d", field.GetName(), suffix)
			if !used[candidate] {
				name = candidate
				break
			}
		}
	}
	index := int32(len(message.GetOneofDecl()))
	message.OneofDecl = append(message.OneofDecl, &descriptorpb.OneofDescriptorProto{Name: proto.String(name)})
	field.OneofIndex = proto.Int32(index)
	field.Proto3Optional = proto.Bool(true)
	return true
}

func setBooleanFieldOption(field *descriptorpb.FieldDescriptorProto, number protoreflect.FieldNumber) bool {
	if field.Options == nil {
		field.Options = new(descriptorpb.FieldOptions)
	}
	data, err := proto.Marshal(field.Options)
	if err == nil && wireFieldPresent(data, protowire.Number(number)) {
		return false
	}
	unknown := field.Options.ProtoReflect().GetUnknown()
	unknown = protowire.AppendTag(unknown, protowire.Number(number), protowire.VarintType)
	unknown = protowire.AppendVarint(unknown, 1)
	field.Options.ProtoReflect().SetUnknown(unknown)
	return true
}

func wireFieldPresent(data []byte, target protowire.Number) bool {
	for len(data) > 0 {
		number, wireType, n := protowire.ConsumeTag(data)
		if n < 0 {
			return false
		}
		data = data[n:]
		if number == target {
			return true
		}
		n = protowire.ConsumeFieldValue(number, wireType, data)
		if n < 0 {
			return false
		}
		data = data[n:]
	}
	return false
}

func marshalFieldOptions(options *descriptorpb.FieldOptions) []byte {
	if options == nil {
		return nil
	}
	data, _ := proto.Marshal(options)
	return data
}

func descriptorAcceptsShape(field protoreflect.FieldDescriptor, shape *wireShape) bool {
	if shape == nil || shape.kind == shapeConflict {
		return false
	}
	value := shape
	if field.Cardinality() == protoreflect.Repeated {
		if shape.kind != shapeRepeated {
			return false
		}
		value = shape.elem
	} else if shape.kind == shapeRepeated {
		return false
	}
	if value == nil || value.kind == shapeConflict {
		return false
	}
	if field.Message() != nil {
		return value.kind == shapeMessage
	}
	return value.kind == shapeScalar
}

func descriptorFieldType(field *descriptorpb.FieldDescriptorProto) string {
	label := ""
	if field.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		label = "repeated "
	}
	if field.GetTypeName() != "" {
		return label + strings.TrimPrefix(field.GetTypeName(), ".")
	}
	return label + strings.TrimPrefix(field.GetType().String(), "TYPE_")
}

func shapeCount(shape *wireShape) int {
	if shape == nil {
		return 0
	}
	return shape.count
}

func protoFieldByNumber(message *descriptorpb.DescriptorProto, number int32) *descriptorpb.FieldDescriptorProto {
	for _, field := range message.GetField() {
		if field.GetNumber() == number {
			return field
		}
	}
	return nil
}
