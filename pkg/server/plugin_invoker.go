/*
 * Copyright 2018-Present the original author or authors.
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
	"github.com/projectriff/go-function-invoker/pkg/function"
	"reflect"
	"net/url"
	"plugin"
	"errors"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
)

const (
	// Headers

	ContentType   = "Content-Type"
	Accept        = "Accept"
	CorrelationId = "correlationId"
	Error         = "error"

	// Url query parameter that identifies the exported function to execute
	Handler = "handler"

	AssumedContentType = MediaType("text/plain")

	// Errors

	ContentTypeNotSupported = errorCode("error-client-content-type-unsupported")
	AcceptNotSupported      = errorCode("error-client-accept-type-unsupported")
	ErrorWhileUnmarshalling = errorCode("error-client-unmarshall")
	ErrorWhileMarshalling   = errorCode("error-client-marshall")
	InvocationError         = errorCode("error-server-function-invocation")
)

type pluginInvoker struct {
	// user function to invoke, in 'canonical' func (in <-chan X) (out <-chan Y [, errs <-chan error]) form.
	fn            reflect.Value
	inType        reflect.Type // The in channel elem type.
	marshallers   []Marshaller
	unmarshallers []Unmarshaller
}

type errorCode string

type invokerError struct {
	code    errorCode
	cause   error
	message string
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()
var nilError reflect.Value

var Trace *log.Logger // exported so the main package can redirect output

func init() {
	var e error
	nilError = reflect.ValueOf(&e).Elem()

	Trace = log.New(os.Stdout, "", log.LstdFlags)
}

// type shared captures all coordination state between the two goroutines and the Call()
// function, to make function signatures more digestible.
type shared struct {
	input  reflect.Value // reflects the 'input' channel to the user function
	output reflect.Value // reflects the 'output' channel of the user function
	fnErrs reflect.Value // reflects the 'errors' channel of the user function (optional)

	sidecar function.MessageFunction_CallServer

	errs chan error    // used to signal errors to the Call() function
	done chan struct{} // used to broadcast early cancellation to all parties, and opt out of an otherwise blocking channel operation

	// TODO: make Accept passing a responsibility of the sidecar
	// TODO: make correlationId propagation a responsibility of the sidecar
	acceptC chan []string
}

func (pi *pluginInvoker) Call(callServer function.MessageFunction_CallServer) error {
	
	input := makeChannel(pi.inType)
	channelValues := pi.fn.Call([]reflect.Value{input})

	ss := &shared{
		input:          input,
		output:         channelValues[0],
		sidecar:        callServer,
		errs:           make(chan error, 1),
		done:           make(chan struct{}),
		acceptC:        make(chan []string, 1),
	}

	if len(channelValues) == 2 {
		ss.fnErrs = channelValues[1]
	}

	// Sidecar => function input
	go pi.sidecar2Function()(ss)

	// Function output => sidecar
	go pi.function2Sidecar()(ss)

	var err error
	for i := 0; i < 2+(len(channelValues)-1); i++ { // Read errors from both goroutines + 1 optional from fn itself
		err, _ = <-ss.errs // Will read the zero value of error, which is nil, in case none was posted
		if err != nil {
			break
		}
	}

	Trace.Printf("Exiting Call(). error = %#v\n\n", err)
	return err

}

func (pi *pluginInvoker) sidecar2Function() func(*shared) {
	return func(s *shared) {
		for {

			in, err := s.sidecar.Recv()
			if err == io.EOF {
				s.input.Close()
				s.errs <- nil
				Trace.Printf("[Sidecar -> Function] Reached EOF\n")
				break
			}
			if err != nil {
				Trace.Printf("[Sidecar -> Function] Error returned from callServer.Recv: %#v\n", err)
				s.input.Close()
				s.errs <- err
				break
			}

			if in.Headers[Accept] != nil {
				select {
				case s.acceptC <- in.Headers[Accept].Values:
				default:
				}
			}
			unmarshalled, err := pi.messageToFunctionArgs(in)
			if err != nil {
				Trace.Printf("[Sidecar -> Function] Sending %v to errors\n", err)
				s.input.Close()
				s.errs <- err
				break
			}
			Trace.Printf("[Sidecar -> Function] About to send %v to function\n", unmarshalled)

			//select {
			//	case input <- unmarshalled:
			//	case <-done: // by virtue of being closed somewhere else
			//    break
			//}
			cases := []reflect.SelectCase{
				{Dir: reflect.SelectSend, Chan: s.input, Send: reflect.ValueOf(unmarshalled)},
				{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(s.done)},
			}
			chosen, _, recvOK := reflect.Select(cases)
			if chosen == 1 {
				if recvOK {
					panic("illegal state: should only fall in this case because done channel was closed")
				}
				s.input.Close()
				s.errs <- nil
				break
			}
		}
		Trace.Printf("[Sidecar -> Function] Returning from sidecar => function input goroutine\n")
	}
}

func (pi *pluginInvoker) function2Sidecar() func(*shared) {
	return func(s *shared) {
		var accept []string

		cases := []reflect.SelectCase{
			{Dir: reflect.SelectRecv, Chan: s.output},
		}
		open := 1
		if s.fnErrs.IsValid() { // user function has a (<-chan error) second result
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: s.fnErrs})
			open++
		}

		for {

			chosen, value, more := reflect.Select(cases)
			switch chosen {
			case 0: // output
				Trace.Printf("[Function -> Sidecar] Returning %v more=%v\n", value, more)

				if !more {
					s.errs <- nil
					cases[chosen].Chan = reflect.ValueOf(nil)
					open--
					break
				}

				if accept == nil {
					select {
					case v := <-s.acceptC:
						accept = v
					default:
						accept = []string{"text/plain"}
					}
				}

				marshalled, err := pi.functionResultToMessage(value.Interface(), accept)
				if err != nil {
					Trace.Printf("[Function -> Sidecar] Error returned from marshall: %#v\n", err)
					s.errs <- err
					cases[chosen].Chan = reflect.ValueOf(nil)
					open--
					close(s.done)
					break
				}

				err = s.sidecar.Send(marshalled)
				if err != nil {
					Trace.Printf("[Function -> Sidecar] Error returned from callServer.Send: %v\n", err)
					s.errs <- err
					cases[chosen].Chan = reflect.ValueOf(nil)
					open--
					close(s.done)
					break
				}
			case 1: // optional error
				cases[chosen].Chan = reflect.ValueOf(nil)
				open--
				if more && !value.IsNil() {
					s.errs <- value.Interface().(error)
					close(s.done)
				} else {
					s.errs <- nil
				}
			}

			if open == 0 {
				break
			}
		}
		Trace.Printf("[Function -> Sidecar] Returning from function output => sidecar goroutine\n")
	}
}


func (pi *pluginInvoker) messageToFunctionArgs(in *function.Message) (interface{}, error) {
	contentType := AssumedContentType
	if ct, ok := in.Headers[ContentType]; ok {
		contentType = MediaType(ct.Values[0])
	}
	for _, um := range pi.unmarshallers {
		if um.canUnmarshall(pi.inType, contentType) {
			result, err := um.unmarshall(bytes.NewReader(in.Payload), pi.inType, contentType)
			if err != nil {
				return nil, invokerError{code: ErrorWhileUnmarshalling, cause: err}
			} else {
				return result, nil
			}
		}
	}
	return nil, unsupportedContentType(contentType)
}

func (invoker *pluginInvoker) functionResultToMessage(value interface{}, accept []string) (*function.Message, error) {

	var payload []byte
	var contentType MediaType

	// successful invocation
	supportedMarshallers := make(map[MediaType]Marshaller)
	for _, m := range invoker.marshallers {
		t := reflect.TypeOf(value)
		offers := m.supportedMediaTypes(t)
		for _, o := range offers {
			if _, present := supportedMarshallers[o]; !present {
				supportedMarshallers[o] = m
			}
		}
	}
	chosen, contentType := bestMarshaller(accept, supportedMarshallers)
	if chosen != nil {
		var buffer bytes.Buffer
		err := chosen.marshall(value, &buffer, contentType)
		if err != nil {
			return nil, invokerError{code: ErrorWhileMarshalling, cause: err}
		}
		payload = buffer.Bytes()
	} else {
		return nil, invokerError{code: AcceptNotSupported, cause: fmt.Errorf("unsupported content types: %v", accept)}
	}

	return &function.Message{Payload: payload,
		Headers: map[string]*function.Message_HeaderValue{ContentType: &function.Message_HeaderValue{Values: []string{string(contentType)}}}}, nil

}

func NewInvoker(fnUri string) (*pluginInvoker, error) {
	result := pluginInvoker{}

	url, err := url.Parse(fnUri)
	if err != nil {
		return &result, err
	}
	if url.Scheme != "" && url.Scheme != "file" {
		return &result, errors.New("Unsupported scheme in function URI: " + fnUri)
	}
	lib, err := plugin.Open(url.Path)
	if err != nil {
		return &result, err
	}
	fnName := url.Query()[Handler][0]
	fnSymbol, err := lib.Lookup(fnName)
	if err != nil {
		return &result, err
	}
	result.fn = reflect.ValueOf(fnSymbol)
	err = result.canonicalize()

	Trace.Printf("FUNCTION %v = %#v\n", fnName, result.fn)

	result.marshallers = []Marshaller{&jsonMarshalling{}, &textMarshalling{}}
	result.unmarshallers = []Unmarshaller{&jsonMarshalling{}, &textMarshalling{}}
	return &result, err

}

// canonicalize turns a function value that may be non-streaming, non-error-returning into
// a value reflecting a "func (in <-chan X) (out <-chan Y, errors <-chan error)" form.
//
// If the provided function does not accept a channel as first parameter, it is assumed that it is a non-streaming
// function. In that case, its return type (if present and different from error) must not be a channel type either.
// Such a function will be wrapped into a function that accepts the desired channel(s) and invokes the provided function f.
func (invoker *pluginInvoker) canonicalize() error {

	var inputType0, outputType0, outputType1 reflect.Type = nil, nil, nil
	if invoker.fn.Type().NumIn() > 0 {
		inputType0 = invoker.fn.Type().In(0)
	}
	if invoker.fn.Type().NumOut() > 0 {
		outputType0 = invoker.fn.Type().Out(0)
	}
	if invoker.fn.Type().NumOut() > 1 {
		outputType1 = invoker.fn.Type().Out(1)
	}
	if invoker.fn.Type().NumOut() > 2 {
		return fmt.Errorf("too many return values in %#v", invoker.fn)
	}

	// Is the function working with channels?
	if inputType0 != nil && inputType0.Kind() == reflect.Chan &&
		outputType0 != nil && outputType0.Kind() == reflect.Chan {
		if !canReceive(inputType0) || !canReceive(outputType0) {
			return fmt.Errorf("wrong direction of channels in function %#v", invoker.fn)
		}

		if outputType1 == nil || outputType1.Kind() == reflect.Chan && outputType1.Elem() == errorType && canReceive(outputType1) {
			// Already exactly what we want
			invoker.inType = inputType0.Elem()
			return nil
		} else {
			return fmt.Errorf("second return type of function should be ([<-]chan error) in %#v", invoker.fn)
		}
	} else {
		// The original fn could have any of the following forms:
		// f(X) (Y, error)
		// f(X) Y
		// f(X)
		// f(X) error
		// f() (Y, error)
		// f() Y
		// f()
		// f() error

		oldFn := invoker.fn

		// TODO: check IN or OUT are not channel
		invoker.inType = reflect.TypeOf(struct{}{})
		if oldFn.Type().NumIn() > 1 {
			return fmt.Errorf("too many arguments to non streaming function: %#v", oldFn)
		} else if isAcceptingInput(oldFn) {
			invoker.inType = oldFn.Type().In(0)
		}

		outType := reflect.TypeOf(struct{}{})
		if oldFn.Type().NumOut() > 2 {
			return fmt.Errorf("too many return values for non streaming function: %#v", oldFn)
		} else if hasReturnValue(oldFn) {
			outType = oldFn.Type().Out(0)
		}

		wrapper := func(args []reflect.Value) []reflect.Value {
			in := args[0]
			out := makeChannel(outType)
			errs := makeChannel(errorType)

			go func() {
				defer out.Close()
				defer errs.Close()

				i, open := in.Recv()
				Trace.Printf("[-Function Wrapper->] In function, input = %#v, open=%v\n", i, open)
				var fnResult []reflect.Value
				if open {
					// original function receiving actual input
					fnResult = oldFn.Call([] reflect.Value{i})
				} else if !isAcceptingInput(oldFn) {
					// input channel closed immediately. Invoke original zero-arg fn
					fnResult = oldFn.Call([]reflect.Value{})
				} else {
					// input closed early, because of earlier (eg unmarshalling) error. Do nothing
					return
				}

				Trace.Printf("[-Function Wrapper->] In function, result = %#v\n", unwrap(fnResult))
				if isErroring(oldFn) && !fnResult[oldFn.Type().NumOut()-1].IsNil() {
					Trace.Printf("[-Function Wrapper->] Sending error %#v", fnResult[oldFn.Type().NumOut()-1])
					errs.Send(fnResult[oldFn.Type().NumOut()-1])
				} else if hasReturnValue(oldFn) {
					Trace.Printf("[-Function Wrapper->] Sending result %#v", fnResult[0])
					out.Send(fnResult[0])
				}
			}()
			return []reflect.Value{out, errs}
		}

		cInType := reflect.ChanOf(reflect.RecvDir, invoker.inType)
		cOutType := reflect.ChanOf(reflect.BothDir, outType)
		cErrorType := reflect.ChanOf(reflect.BothDir, errorType)
		t := reflect.FuncOf([]reflect.Type{cInType}, []reflect.Type{cOutType, cErrorType}, false)
		invoker.fn = reflect.MakeFunc(t, wrapper)

		return nil
	}
}

// isAcceptingInput returns true if the Value provided (representing a func value) accepts exactly one parameter
func isAcceptingInput(oldFn reflect.Value) bool {
	return oldFn.Type().NumIn() == 1
}

// isErroring returns true if the Value provided (representing a func value) has its last return value of type error
func isErroring(fnValue reflect.Value) bool {
	return fnValue.Type().NumOut() > 0 && fnValue.Type().Out(fnValue.Type().NumOut()-1) == errorType
}

// hasReturnValue returns true if the Value provided (representing a func value) has its first return value (if present)
// as a non-error type
func hasReturnValue(fnValue reflect.Value) bool {
	return fnValue.Type().NumOut() > 0 && fnValue.Type().Out(0) != errorType
}

// canReceive returns true if the given type (representing a channel type) can be used to receive
func canReceive(inputType0 reflect.Type) bool {
	return inputType0.ChanDir()&reflect.RecvDir == reflect.RecvDir
}

func unsupportedContentType(ct MediaType) invokerError {
	return invokerError{
		code:    ContentTypeNotSupported,
		message: "Unsupported Content-Type: " + string(ct),
	}
}

func (ie invokerError) Error() string {
	if ie.cause != nil {
		return ie.cause.Error()
	} else {
		return ie.message
	}
}

func makeChannel(t reflect.Type) reflect.Value {
	ctype := reflect.ChanOf(reflect.BothDir, t)
	return reflect.MakeChan(ctype, 0)
}

func unwrap(in []reflect.Value) []interface{} {
	result := make([]interface{}, len(in))
	for i, v := range in {
		result[i] = v.Interface()
	}
	return result
}
