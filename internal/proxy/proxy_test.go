/*
Copyright 2025 The llm-d Authors.

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
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/llm-d/llm-d-routing-sidecar/test/mock"
	. "github.com/onsi/ginkgo/v2" // nolint:revive
	. "github.com/onsi/gomega"    // nolint:revive
	"k8s.io/klog/v2/ktesting"
)

var _ = Describe("Reverse Proxy", func() {
	When("x-prefiller-url is not present", func() {
		DescribeTable("should forward requests to decode server",

			func(path string, secureProxy bool) {
				_, ctx := ktesting.NewTestContext(GinkgoT())

				ackHandlerFn := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(200)
				})

				decodeBackend := httptest.NewServer(ackHandlerFn)
				defer decodeBackend.Close()

				targetURL, err := url.Parse(decodeBackend.URL)
				Expect(err).ToNot(HaveOccurred())

				cfg := Config{SecureProxy: secureProxy}
				proxy := NewProxy("0", targetURL, cfg) // port 0 to automatically choose one that's available.

				ctx, cancelFn := context.WithCancel(ctx)
				defer cancelFn()

				go func() {
					defer GinkgoRecover()

					err := proxy.Start(ctx)
					Expect(err).ToNot(HaveOccurred())
				}()

				time.Sleep(1 * time.Second)
				Expect(proxy.addr).ToNot(BeNil())

				tr := &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true, // Skip certificate verification
					},
				}
				client := &http.Client{
					Transport: tr,
					Timeout:   10 * time.Second,
				}

				proxyAddr := proxy.addr.String() + path
				if secureProxy {
					proxyAddr = "https://" + proxyAddr
				} else {
					proxyAddr = "http://" + proxyAddr
				}
				resp, err := client.Get(proxyAddr)
				Expect(err).ToNot(HaveOccurred())

				_, err = io.ReadAll(resp.Body)
				Expect(err).ToNot(HaveOccurred())
				err = resp.Body.Close()
				Expect(err).ToNot(HaveOccurred())

				Expect(resp.StatusCode).To(BeNumerically("==", 200))
			},

			Entry("when the path is /v1/chat/completions and secure proxy is false", "/v1/chat/completions", false),
			Entry("when the path is /v1/completions and secure proxy is false", "/v1/completions", false),
			Entry("when the path is /v1/embeddings and secure proxy is false", "/v1/embeddings", false),
			Entry("when the path is /score and secure proxy is false", "/score", false),
			Entry("when the path is /healthz and secure proxy is false", "/healthz", false),

			Entry("when the path is /v1/chat/completions and secure proxy is true", "/v1/chat/completions", true),
			Entry("when the path is /v1/completions and secure proxy is true", "/v1/completions", true),
			Entry("when the path is /v1/embeddings and secure proxy is true", "/v1/embeddings", true),
			Entry("when the path is /score and secure proxy is true", "/score", true),
			Entry("when the path is /healthz and secure proxy is true", "/healthz", true),
		)
	})

	When("x-prefiller-url is present", func() {
		var ctx context.Context
		var decodeBackend *httptest.Server
		var decodeHandler *mock.ChatCompletionHandler
		var prefillBackend *httptest.Server
		var prefillHandler *mock.ChatCompletionHandler
		var decodeURL *url.URL

		BeforeEach(func() {
			_, ctx = ktesting.NewTestContext(GinkgoT())

			// Decoder
			decodeHandler = &mock.ChatCompletionHandler{
				Role: mock.RoleDecode,
			}
			decodeBackend = httptest.NewServer(decodeHandler)
			DeferCleanup(decodeBackend.Close)

			// Prefiller
			prefillHandler = &mock.ChatCompletionHandler{
				Role: mock.RolePrefill,
			}
			prefillBackend = httptest.NewServer(prefillHandler)
			DeferCleanup(prefillBackend.Close)

			// Proxy
			url, err := url.Parse(decodeBackend.URL)
			Expect(err).ToNot(HaveOccurred())
			decodeURL = url
		})

		When("using NIXL connector V1", func() {
			var proxy *Server

			BeforeEach(func() {
				cfg := Config{Connector: ConnectorNIXLV1}
				proxy = NewProxy("0", decodeURL, cfg) // port 0 to automatically choose one that's available.

				decodeHandler.Connector = ConnectorNIXLV1
				prefillHandler.Connector = ConnectorNIXLV1
			})

			It("should successfully send request to 1. prefill 2. decode with the right fields (backward compatible behavior)", func() {
				By("starting the proxy")
				go func() {
					defer GinkgoRecover()

					err := proxy.Start(ctx)
					Expect(err).ToNot(HaveOccurred())
				}()

				time.Sleep(1 * time.Second)
				Expect(proxy.addr).ToNot(BeNil())
				proxyBaseAddr := "http://" + proxy.addr.String()

				By("sending a /v1/chat/completions request with prefill header")
				body := `{
        			"model": "Qwen/Qwen2-0.5B",
	        		"messages": [
    			      {"role": "user", "content": "Hello"}
        			],
        			"max_tokens": 50
				}`

				req, err := http.NewRequest(http.MethodPost, proxyBaseAddr+ChatCompletionsPath, strings.NewReader(body))
				Expect(err).ToNot(HaveOccurred())
				req.Header.Add(requestHeaderPrefillHostPort, prefillBackend.URL)

				_, err = http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())

				Expect(prefillHandler.RequestCount.Load()).To(BeNumerically("==", 1))

				Expect(prefillHandler.CompletionRequests).To(HaveLen(1))
				prq1 := prefillHandler.CompletionRequests[0]

				Expect(prq1).To(HaveKeyWithValue(requestFieldDoRemoteDecode, true))
				Expect(prq1).To(HaveKeyWithValue("stream", false))
				Expect(prq1).ToNot(HaveKey("stream_options"))

				Expect(prefillHandler.CompletionResponses).To(HaveLen(1))
				prp1 := prefillHandler.CompletionResponses[0]
				Expect(prp1).To(HaveKey(requestFieldRemoteBlockIDs))
				Expect(prp1).To(HaveKey(requestFieldRemoteEngineID))

				Expect(decodeHandler.RequestCount.Load()).To(BeNumerically("==", 1))
				Expect(decodeHandler.CompletionRequests).To(HaveLen(1))
				drq1 := decodeHandler.CompletionRequests[0]

				Expect(drq1).To(HaveKey(requestFieldRemoteBlockIDs))
				Expect(drq1).To(HaveKey(requestFieldRemoteEngineID))
			})

			It("should successfully send request to 1. prefill 2. decode with the right fields", func() {
				By("starting the proxy")
				go func() {
					defer GinkgoRecover()

					err := proxy.Start(ctx)
					Expect(err).ToNot(HaveOccurred())
				}()

				time.Sleep(1 * time.Second)
				Expect(proxy.addr).ToNot(BeNil())
				proxyBaseAddr := "http://" + proxy.addr.String()

				By("sending a /v1/chat/completions request with prefill header")
				body := `{
        			"model": "Qwen/Qwen2-0.5B",
	        		"messages": [
    			      {"role": "user", "content": "Hello"}
        			],
        			"max_tokens": 50
				}`

				req, err := http.NewRequest(http.MethodPost, proxyBaseAddr+ChatCompletionsPath, strings.NewReader(body))
				Expect(err).ToNot(HaveOccurred())
				req.Header.Add(requestHeaderPrefillHostPort, prefillBackend.URL[len("http://"):])

				_, err = http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())

				Expect(prefillHandler.RequestCount.Load()).To(BeNumerically("==", 1))

				Expect(prefillHandler.CompletionRequests).To(HaveLen(1))
				prq1 := prefillHandler.CompletionRequests[0]

				Expect(prq1).To(HaveKeyWithValue(requestFieldDoRemoteDecode, true))
				Expect(prq1).To(HaveKeyWithValue("stream", false))
				Expect(prq1).ToNot(HaveKey("stream_options"))

				Expect(prefillHandler.CompletionResponses).To(HaveLen(1))
				prp1 := prefillHandler.CompletionResponses[0]
				Expect(prp1).To(HaveKey(requestFieldRemoteBlockIDs))
				Expect(prp1).To(HaveKey(requestFieldRemoteEngineID))

				Expect(decodeHandler.RequestCount.Load()).To(BeNumerically("==", 1))
				Expect(decodeHandler.CompletionRequests).To(HaveLen(1))
				drq1 := decodeHandler.CompletionRequests[0]

				Expect(drq1).To(HaveKey(requestFieldRemoteBlockIDs))
				Expect(drq1).To(HaveKey(requestFieldRemoteEngineID))
			})
		})
	})
})
