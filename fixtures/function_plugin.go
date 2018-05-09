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
	"errors"
	"fmt"
	"time"
	"strconv"
	"net/http"
	"strings"
)

func StringInStringOut(in string) (string, error) {
	if in == "Riff" {
		return "", errors.New("error condition")
	}
	return "Hello " + in, nil
}

type RLE struct {
	Word  string
	Count int
}

func RunLengthEncode(in <-chan string) (<-chan RLE, <-chan error) {

	out := make(chan RLE)
	errors := make(chan error)
	go func() {
		current := RLE{Count: -1}
		defer close(out)
		defer close(errors)
		for w := range in {

			if current.Word == w {
				current.Count++
				if current.Count == 3 {
					errors <- fmt.Errorf("Too many occurrences of %v", current.Word)
					return
				}
			} else {
				if current.Count != -1 {
					out <- current
				}
				current.Count = 1
				current.Word = w
			}
		}
		out <- current
	}()

	return out, errors
}

func SupplierFunc(in chan struct{}) (<-chan int) {
	out := make(chan int, 100)
	go func() {
		defer close(out)
		ticker := time.Tick(10 * time.Millisecond)
		i := 0
		for {
			select {
			case _, more := <-in: // Used to signal the function to stop
				if !more {
					return
				} else {
					panic("Not meant to be used this way")
				}
			case <-ticker:
				out <- i
				i++
			}
		}
	}()
	return out
}

func RunningAverage(in <-chan float32) <-chan float32 {
	out := make(chan float32)
	go func() {
		defer close(out)
		n := 0
		sum := float32(0)
		for f := range in {
			sum += f
			n++
			out <- sum/float32(n)
		}
	}()
	return out
}

// f(X) (Y, error)
// f(X) Y
// f(X)
// f(X) error
// f() (Y, error)
// f() Y
// f()
// f() error

func Direct1(s string) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return v * 2, nil
}

func Direct2(s string) int {
	v, _ := strconv.Atoi(s)
	return v * 2
}

func Direct3(s string) {
	http.Get(s)
}

func Direct4(s string) error {
	if strings.Contains(s, "Riff") {
		return fmt.Errorf("%v contained 'Riff'", s)
	}
	return nil
}

func Direct5() (int, error) {
	return 5, nil
}

func Direct5e() (int, error) {
	return 0, errors.New("Direct5e error")
}

func Direct6() int {
	return 42
}

func Direct7() {

}

func Direct8() error {
	return nil
}
func Direct8e() error {
	return errors.New("Direct8e error")
}
