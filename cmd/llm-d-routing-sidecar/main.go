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
package main

import (
	"context"
	"flag"
	"net/url"

	"k8s.io/klog/v2"

	"github.com/llm-d/llm-d-routing-sidecar/internal/proxy"
	"github.com/llm-d/llm-d-routing-sidecar/internal/signals"
)

func main() {
	port := flag.String("port", "8000", "the port the sidecar is listening on")
	vLLMPort := flag.String("vllm-port", "8001", "the port vLLM is listening on")
	connector := flag.String("connector", "nixl", "the P/D connector being used. Either nixl, nixlv2 or lmcache")
	prefillerUseTLS := flag.Bool("prefiller-use-tls", false, "whether to use TLS when sending requests to prefillers")
	decoderUseTLS := flag.Bool("decoder-use-tls", false, "whether to use TLS when sending requests to the decoder")
	secureProxy := flag.Bool("secure-proxy", true, "Enables secure proxy. Defaults to true.")
	certPath := flag.String(
		"cert-path", "", "The path to the certificate for secure proxy. The certificate and private key files "+
			"are assumed to be named tls.crt and tls.key, respectively. If not set, and secureProxy is enabled, "+
			"then a self-signed certificate is used (for testing).")
	klog.InitFlags(nil)
	flag.Parse()

	// make sure to flush logs before exiting
	defer klog.Flush()

	ctx := signals.SetupSignalHandler(context.Background())
	logger := klog.FromContext(ctx)

	if *connector != proxy.ConnectorNIXLV1 && *connector != proxy.ConnectorNIXLV2 && *connector != proxy.ConnectorLMCache {
		logger.Info("Error: --connector must either be 'nixl', 'nixlv2' or 'lmcache'")
		return
	}
	logger.Info("p/d connector validated", "connector", connector)

	// start reverse proxy HTTP server
	scheme := "http"
	if *decoderUseTLS {
		scheme = "https"
	}
	targetURL, err := url.Parse(scheme + "://localhost:" + *vLLMPort)
	if err != nil {
		logger.Error(err, "failed to create targetURL")
		return
	}

	config := proxy.Config{
		Connector:       *connector,
		PrefillerUseTLS: *prefillerUseTLS,
		SecureProxy:     *secureProxy,
		CertPath:        *certPath,
	}

	proxy := proxy.NewProxy(*port, targetURL, config)
	if err := proxy.Start(ctx); err != nil {
		logger.Error(err, "failed to start proxy server")
	}
}
