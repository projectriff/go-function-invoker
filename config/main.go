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

package main

import (
	"time"
	"google.golang.org/grpc"
	"fmt"
	"github.com/projectriff/go-function-invoker/pkg/function"
	"golang.org/x/net/context"
)

func main() {
	ctx, _ := context.WithTimeout(context.Background(), 60*time.Second)
	conn, err := grpc.DialContext(ctx, fmt.Sprintf("localhost:%v", 10382), grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		panic(err)
	}

	fnStream, err := function.NewMessageFunctionClient(conn).Call(context.Background())
	if err != nil {
		panic(err)
	}

	err = fnStream.Send(&function.Message{Payload: []byte("hello")})
	if err != nil {
		panic(err)
	}
	err = fnStream.Send(&function.Message{Payload: []byte("hello")})
	if err != nil {
		panic(err)
	}
	err = fnStream.Send(&function.Message{Payload: []byte("world")})
	if err != nil {
		panic(err)
	}
	err = fnStream.Send(&function.Message{Payload: []byte("world")})
	if err != nil {
		panic(err)
	}
	err = fnStream.Send(&function.Message{Payload: []byte("Riff")})
	if err != nil {
		panic(err)
	}

	err = fnStream.CloseSend()

	for {
		result, err := fnStream.Recv()
		fmt.Printf("Got %v (err = %v)\n", result, err)
		if err != nil {
			break
		}

	}

	fmt.Println("Waiting")
	time.Sleep(5 * time.Second)

	fmt.Println("Done")

}
