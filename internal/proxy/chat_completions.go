/*
Copyright 2025 IBM.

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

package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

var (
	ChatCompletionsPath = "/v1/chat/completions"
	CompletionsPath     = "/v1/completions"
)

func (s *Server) ChatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	prefillPodURL := r.Header.Get(RequestHeaderPrefillURL)

	if prefillPodURL == "" {
		s.logger.V(5).Info("skip disagreggated prefill")
		s.decoderProxy.ServeHTTP(w, r)
		return
	}

	// Read and parse request body
	defer r.Body.Close() //nolint:all
	original, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest) // TODO: check FastAPI error code when failing to read body
		w.Write([]byte(err.Error()))         //nolint:all
		return
	}

	var completionRequest map[string]any
	if err := json.Unmarshal(original, &completionRequest); err != nil {
		if err := errorJSONInvalid(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}

	// Create prefiller request

	ctx := r.Context()
	preq := r.Clone(ctx)

	completionRequest["max_tokens"] = 1
	completionRequest["max_completion_tokens"] = 1

	pbody, err := json.Marshal(completionRequest)
	if err != nil {
		if err := errorJSONInvalid(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}
	preq.Body = io.NopCloser(strings.NewReader(string(pbody)))
	preq.ContentLength = int64(len(pbody))

	// Forward request

	prefillHandler, err := s.prefillerProxyHandler(prefillPodURL)
	if err != nil {
		if err := errorBadGateway(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}

	pw := &statusResponseWriter{}
	prefillHandler.ServeHTTP(pw, preq)

	if pw.statusCode < 200 || pw.statusCode >= 300 {
		s.logger.Error(err, "request failed", "code", pw.statusCode)
		w.WriteHeader(pw.statusCode)
		return
	}

	// Forward original request to local decoder

	r.Body = io.NopCloser(strings.NewReader(string(original)))
	s.decoderProxy.ServeHTTP(w, r)
}
