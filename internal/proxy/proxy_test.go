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
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/neuralmagic/llm-d-routing-sidecar/test/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2/ktesting"
)

var _ = Describe("Reverse Proxy", func() {

	When("x-prefiller-url is not present", func() {

		DescribeTable("should forward requests to decode server",

			func(path string) {
				_, ctx := ktesting.NewTestContext(GinkgoT())

				ackHandlerFn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(200)
				})

				decodeBackend := httptest.NewServer(ackHandlerFn)
				defer decodeBackend.Close()

				targetURL, err := url.Parse(decodeBackend.URL)
				Expect(err).ToNot(HaveOccurred())

				proxy := NewProxy("0", targetURL, ProtocolLMCache) // port 0 to automatically choose one that's available.

				ctx, cancelFn := context.WithCancel(ctx)
				defer cancelFn()

				go func() {
					defer GinkgoRecover()

					err := proxy.Start(ctx)
					Expect(err).ToNot(HaveOccurred())
				}()

				time.Sleep(1 * time.Second)
				Expect(proxy.addr).ToNot(BeNil())

				proxyAddr := "http://" + proxy.addr.String() + path
				resp, err := http.Get(proxyAddr)
				Expect(err).ToNot(HaveOccurred())

				_, err = io.ReadAll(resp.Body)
				Expect(err).ToNot(HaveOccurred())
				err = resp.Body.Close()
				Expect(err).ToNot(HaveOccurred())

				Expect(resp.StatusCode).To(BeNumerically("==", 200))
			},

			Entry("when the path is /v1/chat/completions", "/v1/chat/completions"),
			Entry("when the path is /v1/completions", "/v1/completions"),
			Entry("when the path is /v1/embeddings", "/v1/embeddings"),
			Entry("when the path is /score", "/score"),
			Entry("when the path is /healthz", "/healthz"),
		)
	})

	When("x-prefiller-url is present", func() {
		var ctx context.Context
		var decodeBackend *httptest.Server
		var prefillBackend *httptest.Server
		var proxy *Server

		BeforeEach(func() {
			_, ctx = ktesting.NewTestContext(GinkgoT())

			// Decoder
			decodeBackend = httptest.NewServer(&mock.ChatCompletionHandler{})
			DeferCleanup(decodeBackend.Close)

			// Prefiller
			prefillBackend = httptest.NewServer(&mock.ChatCompletionHandler{})
			DeferCleanup(prefillBackend.Close)

			// Proxy
			decodeURL, err := url.Parse(decodeBackend.URL)
			Expect(err).ToNot(HaveOccurred())

			proxy = NewProxy("0", decodeURL, ProtocolLMCache) // port 0 to automatically choose one that's available.
		})

		It("should successfully send request to prefill worker and then to decoder worker", func() {
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
			req.Header.Add(RequestHeaderPrefillURL, prefillBackend.URL)

			_, err = http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(decodeBackend.Config.Handler.(*mock.ChatCompletionHandler).RequestCount.Load()).To(BeNumerically("==", 1))
			Expect(prefillBackend.Config.Handler.(*mock.ChatCompletionHandler).RequestCount.Load()).To(BeNumerically("==", 1))
		})

		It("should fail when the request is an invalid JSON", func() {
			By("starting the proxy")
			go func() {
				defer GinkgoRecover()

				err := proxy.Start(ctx)
				Expect(err).ToNot(HaveOccurred())
			}()

			time.Sleep(1 * time.Second)
			Expect(proxy.addr).ToNot(BeNil())
			proxyBaseAddr := "http://" + proxy.addr.String()

			By("sending an invalid /v1/chat/completions request with prefill header")
			body := `{
        		"model": "Qwen/Qwen2-0.5B",
	        	"messages": [
    			      {"role": "user", "content": "Hello"}
        		],
        		"max_tokens: 50
			}`
			req, err := http.NewRequest(http.MethodPost, proxyBaseAddr+ChatCompletionsPath, strings.NewReader(body))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Add(RequestHeaderPrefillURL, prefillBackend.URL)

			r, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(r.StatusCode).To(BeNumerically("==", 400))
		})

		DescribeTable("should not forward non-completion requests to prefill server",

			func(path string) {
				decodeBackend.Config.Handler = &mock.GenericHandler{}
				prefillBackend.Config.Handler = &mock.GenericHandler{}

				By("starting the proxy")
				go func() {
					defer GinkgoRecover()

					err := proxy.Start(ctx)
					Expect(err).ToNot(HaveOccurred())
				}()

				time.Sleep(1 * time.Second)
				Expect(proxy.addr).ToNot(BeNil())
				proxyBaseAddr := "http://" + proxy.addr.String()

				req, err := http.NewRequest(http.MethodPost, proxyBaseAddr+path, nil)
				Expect(err).ToNot(HaveOccurred())
				req.Header.Add(RequestHeaderPrefillURL, prefillBackend.URL)

				Expect(err).ToNot(HaveOccurred())
				resp, err := http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())

				Expect(resp.StatusCode).To(BeNumerically("==", 200))
				Expect(decodeBackend.Config.Handler.(*mock.GenericHandler).RequestCount.Load()).To(BeNumerically("==", 1))
				Expect(prefillBackend.Config.Handler.(*mock.GenericHandler).RequestCount.Load()).To(BeNumerically("==", 0))
			},

			Entry("when the path is /v1/embeddings", "/v1/embeddings"),
			Entry("when the path is /score", "/score"),
			Entry("when the path is /healthz", "/healthz"),
			Entry("when the path is /metrics", "/metrics"),
		)
	})
})
