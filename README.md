# P/D Routing Proxy

This project provides a reverse proxy redirecting incoming requests
to the prefill worker specified in the `x-prefiller-url` HTTP request header.

## Getting Started

### Requirements

- a container engine (docker, podman, etc...)
- a server with at least 2 GPUs

### Quick Start (From source)

1. Start two vLLM servers with P/D enabled via the NIXLConnector.

In one terminal, run this command to start the decoder:

```
$ podman run -p 8001:8000 --device nvidia.com/gpu=0 -v $HOME/models:/models \
    -e VLLM_NIXL_SIDE_CHANNEL_PORT=8001 \
    -e VLLM_NIXL_SIDE_CHANNEL_HOST=localhost \
    -e VLLM_LOGGING_LEVEL=DEBUG \
    -e HF_HOME=/models ghcr.io/llm-d/llm-d:0.0.7 --model facebook/opt-125m \
    --kv-transfer-config='{"kv_connector":"NixlConnector","kv_role":"kv_both"}'
```

In another terminal, run this command to start the prefiller:

```
$ podman run -p 8002:8000 --device nvidia.com/gpu=1 -v $HOME/models:/models \
    -e VLLM_NIXL_SIDE_CHANNEL_PORT=8002 \
    -e VLLM_NIXL_SIDE_CHANNEL_HOST=localhost \
    -e VLLM_LOGGING_LEVEL=DEBUG \
    -e HF_HOME=/models ghcr.io/llm-d/llm-d:0.0.7 --model facebook/opt-125m \
    --kv-transfer-config='{"kv_connector":"NixlConnector","kv_role":"kv_both"}'
```


2. Clone and start the routing proxy.

In another terminal, clone this repository and build the routing proxy:

```
$ git clone https://github.com/llm-d/llm-d-routing-sidecar.git && \
  cd llm-d-routing-sidecar && \
  make build
```

In the same terminal, start the routing proxy:

```
$ ./bin/llm-d-routing-sidecar -port=8000 -vllm-port=8001 -connector=nixlv2
```

3. Send a request.

Finally, in another terminal, send a chat completions request to the router proxy on port 8000 and tell it to use the prefiller on port 8002:

```
$ curl  http://localhost:8000/v1/completions \
      -H "Content-Type: application/json" \
      -H "x-prefiller-url: http://localhost:8002" \
       -d '{
        "model": "facebook/opt-125m",
        "prompt": "Author-contribution statements and acknowledgements in research papers should state clearly and specifically whether, and to what extent, the authors used AI technologies such as ChatGPT in the preparation of their manuscript and analysis. They should also indicate which LLMs were used. This will alert editors and reviewers to scrutinize manuscripts more carefully for potential biases, inaccuracies and improper source crediting. Likewise, scientific journals should be transparent about their use of LLMs, for example when selecting submitted manuscripts. Mention the large language model based product mentioned in the paragraph above:"
      }'
```

## Development

### Building the routing proxy

Build the routing proxy from source:

```sh
$ make build
```

Check the build was successful by running it locally:

```
$ ./bin/llm-d-routing-sidecar -help
Usage of ./bin/llm-d-routing-sidecar:
  -connector string
        the P/D connector being used. Either nixl, nixlv2 or lmcache (default "nixl")
  -port string
        the port the sidecar is listening on (default "8000")
  -vllm-port string
        the port vLLM is listening on (default "8001")
...
```

> **Note:** lmcache connector is deprecated. 


## License

This project is licensed under the Apache License 2.0. See the [LICENSE](./LICENSE) file for details.
