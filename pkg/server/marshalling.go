/*
 * Copyright 2017 the original author or authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package server

import (
	"io"
	"reflect"
	"mime"
	"encoding/json"
	"fmt"
	"bytes"
	"strconv"
)

type MediaType string

// Marshaller is used to convert a runtime instance of some type t to bytes
type Marshaller interface {
	// supportedMediaTypes returns the list of types that this marshaller is able to use to marshall type y.
	// Should return nil/empty if t is not supported. Any returned media type should then be acceptable as
	// a call to marshall. Typically, the return mediaTypes will be matched against an 'Accept' header.
	supportedMediaTypes(t reflect.Type) []MediaType

	// marshall should write out the given value to w according to the given mediaType
	marshall(value interface{}, w io.Writer, mediaType MediaType) error
}

// Unmarshaller is used to read bytes to a runtime go object, according to a given mediaType.
type Unmarshaller interface {
	// canUnmarshall should return true if it supports the target receiving type t and the given mediaType (typically
	// the value of a 'Content-Type' header). A subsequent call to unmarshall with that mediaType should not fail
	// by lack of support of that (type, mediaType) combination.
	canUnmarshall(t reflect.Type, mediaType MediaType) bool

	// unmarshall should read bytes from r and turn them into an instance of t, according to the given mediaType.
	unmarshall(r io.Reader, t reflect.Type, mediaType MediaType) (interface{}, error)
}

// jsonMarshalling supports both marshalling and unmarshalling to/from json according to golang's json rules.
type jsonMarshalling struct {
}

func (*jsonMarshalling) supportedMediaTypes(t reflect.Type) []MediaType {
	// Technically, should check that t is marshallable to json
	return []MediaType{"application/json"}
}

func (*jsonMarshalling) marshall(value interface{}, w io.Writer, mediaType MediaType) error {
	err := json.NewEncoder(w).Encode(value)
	return err
}

func (*jsonMarshalling) canUnmarshall(t reflect.Type, mediaType MediaType) bool {
	contentType, _, err := mime.ParseMediaType(string(mediaType))
	if err != nil {
		return false
	}
	// TODO should also chack that t is one of the supported types
	return contentType == "application/json"
}

func (*jsonMarshalling) unmarshall(r io.Reader, t reflect.Type, mediaType MediaType) (interface{}, error) {
	ptrToData := reflect.New(t)
	err := json.NewDecoder(r).Decode(ptrToData.Interface())
	if err != nil {
		return nil, err
	}
	return reflect.Indirect(ptrToData).Interface(), nil
}

// textMarshalling supports marshalling from golang's fmt.Stringer type and unmarshalling to golang's string
type textMarshalling struct {
}

func (*textMarshalling) supportedMediaTypes(t reflect.Type) []MediaType {
	var pstringer *fmt.Stringer
	stringerType := reflect.TypeOf(pstringer).Elem()
	if t.AssignableTo(stringerType) || t.Kind() == reflect.String || t.Kind() == reflect.Int || t.Kind() == reflect.Float32 ||
		t.Kind() == reflect.Float64 {
		return []MediaType{"text/plain"}
	} else {
		return nil
	}
}

// X -> string
func (*textMarshalling) marshall(value interface{}, w io.Writer, mediaType MediaType) error {
	var s string
	switch v := value.(type) {
	case int:
		s = strconv.Itoa(v)
	case float32:
		s = strconv.FormatFloat(float64(v), 'f', -1, 32)
	case float64:
		s = strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		s = v
	case fmt.Stringer:
		s = v.String()
	}
	_, error := io.WriteString(w, s)
	return error
}

func (*textMarshalling) canUnmarshall(t reflect.Type, mediaType MediaType) bool {
	contentType, _, err := mime.ParseMediaType(string(mediaType))
	if err != nil {
		return false
	}
	return contentType == "text/plain" &&
		(t.Kind() == reflect.String || t.Kind() == reflect.Int || t.Kind() == reflect.Float32 || t.Kind() == reflect.Float64)
}

// string -> X
func (*textMarshalling) unmarshall(r io.Reader, t reflect.Type, mediaType MediaType) (interface{}, error) {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(r)
	if err != nil {
		return nil, err
	}
	switch t.Kind() {
	case reflect.String:
		return buf.String(), err
	case reflect.Int:
		return strconv.Atoi(buf.String())
	case reflect.Float32:
		f, err := strconv.ParseFloat(buf.String(), 32)
		return float32(f), err
	case reflect.Float64:
		return strconv.ParseFloat(buf.String(), 64)
	default:
		panic("Unreachable thanks to canUnmarshall()")

	}
}
