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

	"github.com/google/uuid"
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

	s.runConnectorProtocol(w, r, prefillPodURL)
}

func (s *Server) runLMCacheProtocol(w http.ResponseWriter, r *http.Request, prefillPodURL string) {
	s.logger.Info("running LMCache protocol")

	// Read and parse request body
	defer r.Body.Close() //nolint:all
	original, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest) // TODO: check FastAPI error code when failing to read body
		w.Write([]byte(err.Error()))         //nolint:all
		return
	}

	// Parse completion request
	var completionRequest map[string]any
	if err := json.Unmarshal(original, &completionRequest); err != nil {
		if err := errorJSONInvalid(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}

	// Create prefiller request. Set max_tokens to 1.

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

	// Forward request to prefiller

	prefillHandler, err := s.prefillerProxyHandler(prefillPodURL)
	if err != nil {
		if err := errorBadGateway(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}

	pw := &bufferedResponseWriter{}
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

func (s *Server) runNIXLProtocolV2(w http.ResponseWriter, r *http.Request, prefillPodURL string) {
	s.logger.Info("running NIXL protocol V2")

	// Read request body
	defer r.Body.Close() //nolint:all
	original, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest) // TODO: check FastAPI error code when failing to read body
		w.Write([]byte(err.Error()))         //nolint:all
		return
	}

	// Parse completion request
	var completionRequest map[string]any
	if err := json.Unmarshal(original, &completionRequest); err != nil {
		if err := errorJSONInvalid(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}

	// Generate unique request UUID
	uuid, err := uuid.NewUUID()
	if err != nil {
		if err := errorBadGateway(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}
	uuidStr := uuid.String()

	// Send request to prefill pod

	// 1. Prepare request
	ctx := r.Context()
	preq := r.Clone(ctx)

	preq.Header.Add(RequestHeaderRequestID, uuidStr)

	streamValue, streamOk := completionRequest[RequestFieldStream]
	streamOptionsValue, streamOptionsOk := completionRequest[RequestFieldStreamOptions]
	maxTokensValue, maxTokensOk := completionRequest[RequestFieldMaxTokens]

	completionRequest[RequestFieldKVTransferParams] = map[string]any{
		RequestFieldDoRemoteDecode:  true,
		RequestFieldDoRemotePrefill: false,
		RequestFieldRemoteEngineID:  nil,
		RequestFieldRemoteBlockIDs:  nil,
		RequestFieldRemoteHost:      nil,
		RequestFieldRemotePort:      nil,
	}

	completionRequest[RequestFieldStream] = false
	delete(completionRequest, RequestFieldStreamOptions)
	completionRequest[RequestFieldMaxTokens] = 1

	pbody, err := json.Marshal(completionRequest)
	if err != nil {
		if err := errorJSONInvalid(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}
	preq.Body = io.NopCloser(strings.NewReader(string(pbody)))
	preq.ContentLength = int64(len(pbody))

	prefillHandler, err := s.prefillerProxyHandler(prefillPodURL)
	if err != nil {
		if err := errorBadGateway(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}

	// 2. Forward request to prefiller
	s.logger.Info("sending request to prefiller", "url", prefillPodURL, "body", string(pbody))
	pw := &bufferedResponseWriter{}
	prefillHandler.ServeHTTP(pw, preq)

	if pw.statusCode < 200 || pw.statusCode >= 300 {
		s.logger.Error(err, "request failed", "code", pw.statusCode)
		w.WriteHeader(pw.statusCode)
		return
	}

	// Process response - extract p/d fields
	var prefillerResponse map[string]any
	if err := json.Unmarshal([]byte(pw.buffer.String()), &prefillerResponse); err != nil {
		if err := errorJSONInvalid(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}

	// 1. Verify fields exists

	pKVTransferParams, ok := prefillerResponse[RequestFieldKVTransferParams]
	if !ok {
		s.logger.Info("warning: missing 'kv_transfer_params' field in prefiller response")
	}

	s.logger.Info("received prefiller response", RequestFieldKVTransferParams, pKVTransferParams)

	// 1. Prepare decode request
	dreq := r.Clone(ctx)

	dreq.Header.Add(RequestHeaderRequestID, uuidStr)

	delete(completionRequest, RequestFieldStream)
	if streamOk {
		completionRequest[RequestFieldStream] = streamValue
	}
	if streamOptionsOk {
		completionRequest[RequestFieldStreamOptions] = streamOptionsValue
	}
	delete(completionRequest, RequestFieldMaxTokens)
	if maxTokensOk {
		completionRequest[RequestFieldMaxTokens] = maxTokensValue
	}
	completionRequest[RequestFieldKVTransferParams] = pKVTransferParams

	dbody, err := json.Marshal(completionRequest)
	if err != nil {
		if err := errorJSONInvalid(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}
	dreq.Body = io.NopCloser(strings.NewReader(string(dbody)))
	dreq.ContentLength = int64(len(dbody))

	// 3. Forward to local decoder.
	s.logger.Info("sending request to decoder", "body", string(dbody))
	s.decoderProxy.ServeHTTP(w, dreq)
}

func (s *Server) runNIXLProtocolV1(w http.ResponseWriter, r *http.Request, prefillPodURL string) {
	s.logger.Info("running NIXL protocol V1")

	// Read request body
	defer r.Body.Close() //nolint:all
	original, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest) // TODO: check FastAPI error code when failing to read body
		w.Write([]byte(err.Error()))         //nolint:all
		return
	}

	// Parse completion request
	var completionRequest map[string]any
	if err := json.Unmarshal(original, &completionRequest); err != nil {
		if err := errorJSONInvalid(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}

	// Generate unique request UUID
	uuid, err := uuid.NewUUID()
	if err != nil {
		if err := errorBadGateway(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}
	uuidStr := uuid.String()

	// Send request to prefill pod

	// 1. Prepare request
	ctx := r.Context()
	preq := r.Clone(ctx)

	preq.Header.Add(RequestHeaderRequestID, uuidStr)

	streamValue, streamOk := completionRequest[RequestFieldStream]
	streamOptionsValue, streamOptionsOk := completionRequest[RequestFieldStreamOptions]

	completionRequest[RequestFieldDoRemoteDecode] = true
	completionRequest[RequestFieldStream] = false
	delete(completionRequest, RequestFieldStreamOptions)

	pbody, err := json.Marshal(completionRequest)
	if err != nil {
		if err := errorJSONInvalid(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}
	preq.Body = io.NopCloser(strings.NewReader(string(pbody)))
	preq.ContentLength = int64(len(pbody))

	prefillHandler, err := s.prefillerProxyHandler(prefillPodURL)
	if err != nil {
		if err := errorBadGateway(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}

	// 2. Forward request to prefiller
	s.logger.Info("sending request to prefiller", "url", prefillPodURL, "body", string(pbody))
	pw := &bufferedResponseWriter{}
	prefillHandler.ServeHTTP(pw, preq)

	if pw.statusCode < 200 || pw.statusCode >= 300 {
		s.logger.Error(err, "request failed", "code", pw.statusCode)
		w.WriteHeader(pw.statusCode)
		return
	}

	// Process response - extract p/d fields
	var prefillerResponse map[string]any
	if err := json.Unmarshal([]byte(pw.buffer.String()), &prefillerResponse); err != nil {
		if err := errorJSONInvalid(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}

	// 1. Verify fields exists

	blockIDs, ok := prefillerResponse[RequestFieldRemoteBlockIDs]
	if !ok {
		// TODO: error or ignore?
		s.logger.Info("warning: missing 'remote_block_ids' field in prefiller response")
	}

	engineID, ok := prefillerResponse[RequestFieldRemoteEngineID]
	if !ok {
		// TODO: error or ignore?
		s.logger.Info("warning: missing 'remote_engine_id' field in prefiller response")
	}

	remoteHost, ok := prefillerResponse[RequestFieldRemoteHost]
	if !ok {
		// TODO: error or ignore?
		s.logger.Info("warning: missing 'remote_host' field in prefiller response")
	}

	remotePort, ok := prefillerResponse[RequestFieldRemotePort]
	if !ok {
		// TODO: error or ignore?
		s.logger.Info("warning: missing 'remote_port' field in prefiller response")
	}

	s.logger.Info("received prefiller response",
		RequestFieldRemoteBlockIDs, blockIDs,
		RequestFieldRemoteEngineID, engineID,
		RequestFieldRemoteHost, remoteHost,
		RequestFieldRemotePort, remotePort,
	)

	// 2. Prepare decode request
	dreq := r.Clone(ctx)

	dreq.Header.Add(RequestHeaderRequestID, uuidStr)

	delete(completionRequest, RequestFieldDoRemoteDecode)
	delete(completionRequest, RequestFieldStream)
	if streamOk {
		completionRequest[RequestFieldStream] = streamValue
	}
	if streamOptionsOk {
		completionRequest[RequestFieldStreamOptions] = streamOptionsValue
	}

	completionRequest[RequestFieldDoRemotePrefill] = true
	completionRequest[RequestFieldRemoteBlockIDs] = blockIDs
	completionRequest[RequestFieldRemoteEngineID] = engineID
	completionRequest[RequestFieldRemoteHost] = remoteHost
	completionRequest[RequestFieldRemotePort] = remotePort

	dbody, err := json.Marshal(completionRequest)
	if err != nil {
		if err := errorJSONInvalid(err, w); err != nil {
			s.logger.Error(err, "failed to send error response to client")
		}
		return
	}
	dreq.Body = io.NopCloser(strings.NewReader(string(dbody)))
	dreq.ContentLength = int64(len(dbody))

	// 3. Forward to local decoder.
	s.logger.Info("sending request to decoder", "body", string(dbody))
	s.decoderProxy.ServeHTTP(w, dreq)
}
