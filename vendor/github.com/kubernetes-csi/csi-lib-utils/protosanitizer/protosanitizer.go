/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package protosanitizer supports logging of gRPC messages without
// accidentally revealing sensitive fields.
package protosanitizer

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/descriptor"
	"github.com/golang/protobuf/proto"
	protobuf "github.com/golang/protobuf/protoc-gen-go/descriptor"
)

// StripSecrets returns a wrapper around the original CSI gRPC message
// which has a Stringer implementation that serializes the message
// as one-line JSON, but without including secret information.
// Instead of the secret value(s), the string "***stripped***" is
// included in the result.
//
// StripSecrets itself is fast and therefore it is cheap to pass the
// result to logging functions which may or may not end up serializing
// the parameter depending on the current log level.
func StripSecrets(msg interface{}) fmt.Stringer {
	return &stripSecrets{msg}
}

type stripSecrets struct {
	msg interface{}
}

func (s *stripSecrets) String() string {
	// First convert to a generic representation. That's less efficient
	// than using reflect directly, but easier to work with.
	var parsed interface{}
	b, err := json.Marshal(s.msg)
	if err != nil {
		return fmt.Sprintf("<<json.Marshal %T: %s>>", s.msg, err)
	}
	if err := json.Unmarshal(b, &parsed); err != nil {
		return fmt.Sprintf("<<json.Unmarshal %T: %s>>", s.msg, err)
	}

	// Now remove secrets from the generic representation of the message.
	strip(parsed, s.msg)

	// Re-encoded the stripped representation and return that.
	b, err = json.Marshal(parsed)
	if err != nil {
		return fmt.Sprintf("<<json.Marshal %T: %s>>", s.msg, err)
	}
	return string(b)
}

func strip(parsed interface{}, msg interface{}) {
	protobufMsg, ok := msg.(descriptor.Message)
	if !ok {
		// Not a protobuf message, so we are done.
		return
	}

	// The corresponding map in the parsed JSON representation.
	parsedFields, ok := parsed.(map[string]interface{})
	if !ok {
		// Probably nil.
		return
	}

	// Walk through all fields and replace those with ***stripped*** that
	// are marked as secret. This relies on protobuf adding "json:" tags
	// on each field where the name matches the field name in the protobuf
	// spec (like volume_capabilities). The field.GetJsonName() method returns
	// a different name (volumeCapabilities) which we don't use.
	_, md := descriptor.ForMessage(protobufMsg)
	fields := md.GetField()
	if fields != nil {
		for _, field := range fields {
			ex, err := proto.GetExtension(field.Options, csi.E_CsiSecret)
			if err == nil && ex != nil && *ex.(*bool) {
				// Overwrite only if already set.
				if _, ok := parsedFields[field.GetName()]; ok {
					parsedFields[field.GetName()] = "***stripped***"
				}
			} else if field.GetType() == protobuf.FieldDescriptorProto_TYPE_MESSAGE {
				// When we get here,
				// the type name is something like ".csi.v1.CapacityRange" (leading dot!)
				// and looking up "csi.v1.CapacityRange"
				// returns the type of a pointer to a pointer
				// to CapacityRange. We need a pointer to such
				// a value for recursive stripping.
				typeName := field.GetTypeName()
				if strings.HasPrefix(typeName, ".") {
					typeName = typeName[1:]
				}
				t := proto.MessageType(typeName)
				if t == nil || t.Kind() != reflect.Ptr {
					// Shouldn't happen, but
					// better check anyway instead
					// of panicking.
					continue
				}
				v := reflect.New(t.Elem())

				// Recursively strip the message(s) that
				// the field contains.
				i := v.Interface()
				entry := parsedFields[field.GetName()]
				if slice, ok := entry.([]interface{}); ok {
					// Array of values, like VolumeCapabilities in CreateVolumeRequest.
					for _, entry := range slice {
						strip(entry, i)
					}
				} else {
					// Single value.
					strip(entry, i)
				}
			}
		}
	}
}
