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
	"github.com/golang/gddo/httputil"
	"net/http"
)

// bestMarshaller inspects the provided map of Marshallers and the incoming Message's Accept header,
// and returns the marshaller (and mediaType) that best fits one of the accepted media type.
// If no match is found, (nil, "") is returned.
func bestMarshaller(accept []string, marshallers map[MediaType]Marshaller) (Marshaller, MediaType) {
	if accept == nil {
		accept = []string{"text/plain"}
	}
	fakeRequest := http.Request{Header: http.Header{"Accept": accept}}
	offers := make([]string, 0, len(marshallers))
	for o, _ := range marshallers {
		offers = append(offers, string(o))
	}
	chosenMediaType := MediaType(httputil.NegotiateContentType(&fakeRequest, offers, ""))
	return marshallers[chosenMediaType], chosenMediaType
}
