# Testing

## Testing the NIXLConnector


The [nixl](config/overlays/fmass/nixl) directory contains the configuration files deploying
a simple 1P1D sample application using the NIXL connector.

To deploy this application in the `eval` cluster, run this command:

```
$ kustomize build test/config/overlays/fmass/nixl | oc apply -f -
```

Wait a bit (up to 10mn) for the pods to be running.

### Sending a request to the decode sidecar

Port-forward the decode sidecar port on your local machine:

```
$ oc port-forward pod/qwen-decoder 8000:8000
```

Send a request:

```
$ curl http://localhost:8000/v1/completions \
      -H "Content-Type: application/json" \
      -H "x-prefiller-url: http://qwen-prefiller:8000" \
      -d '{"model": "Qwen/Qwen2-0.5B", "prompt": "Question: Greta worked 40 hours and was paid $12 per hour. Her friend Lisa earned $15 per hour at her job. How many hours would Lisa have to work to equal Gretas earnings for 40 hours?", "max_tokens": 200 }'
```

Observe the decoder logs:

```
$ oc logs qwen-decoder
...
NFO 05-06 01:42:03 [logger.py:39] Received request cmpl-4fa3a7af-2a1b-11f0-854f-0a580a800726-0: prompt: 'Question: Greta worked 40 hours and was paid $12 per hour. Her friend Lisa earned $15 per hour at her job. How many hours would Lisa have to work to equal Gretas earnings for 40 hours?', params: SamplingParams(n=1, presence_penalty=0.0, frequency_penalty=0.0, repetition_penalty=1.0, temperature=1.0, top_p=1.0, top_k=-1, min_p=0.0, seed=None, stop=[], stop_token_ids=[], bad_words=[], include_stop_str_in_output=False, ignore_eos=False, max_tokens=200, min_tokens=0, logprobs=None, prompt_logprobs=None, skip_special_tokens=True, spaces_between_special_tokens=True, truncate_prompt_tokens=None, guided_decoding=None, extra_args=None), prompt_token_ids: [14582, 25, 479, 65698, 6439, 220, 19, 15, 4115, 323, 572, 7171, 400, 16, 17, 817, 6460, 13, 6252, 4238, 28556, 15303, 400, 16, 20, 817, 6460, 518, 1059, 2618, 13, 2585, 1657, 4115, 1035, 28556, 614, 311, 975, 311, 6144, 87552, 300, 23681, 369, 220, 19, 15, 4115, 30], lora_request: None, prompt_adapter_request: None.
INFO 05-06 01:42:03 [async_llm.py:255] Added request cmpl-4fa3a7af-2a1b-11f0-854f-0a580a800726-0.
DEBUG 05-06 01:42:03 [core.py:431] EngineCore loop active.
DEBUG 05-06 01:42:03 [nixl_connector.py:545] start_load_kv for request cmpl-4fa3a7af-2a1b-11f0-854f-0a580a800726-0 from remote engine 2faadfa5-23cf-4e58-81c3-4cfb89887471. Num local_block_ids: 3. Num remote_block_ids: 3.
DEBUG 05-06 01:42:03 [nixl_connector.py:449] Rank 0, get_finished: 0 requests done sending and 1 requests done recving
DEBUG 05-06 01:42:03 [scheduler.py:862] Finished recving KV transfer for request cmpl-4fa3a7af-2a1b-11f0-854f-0a580a800726-0
INFO 05-06 01:42:03 [loggers.py:116] Engine 000: Avg prompt throughput: 5.0 tokens/s, Avg generation throughput: 1.6 tokens/s, Running: 1 reqs, Waiting: 0 reqs, GPU KV cache usage: 0.0%, Prefix cache hit rate: 50.0%
DEBUG 05-06 01:42:05 [core.py:425] EngineCore waiting for work.
INFO:     ::1:0 - "POST /v1/completions HTTP/1.1" 200 OK
```

## LMCache KVConnector Testing

The [lmcache](config/nixl) directory contains the configuration files deploying a simple 1P/1D application
using NIXL with GPU-2-GPU transfer and sidecar.

To deploy this application in the `fmass-eval` cluster, run this command:

```
$ kustomize build test/config/overlays/fmass/nixl | oc apply -f -
```

Port-forward the sidecar port on your local machine:

```
$ oc port-forward svc/qwen-decoder 8000:8000
```

In another terminal, verify vLLM is up and running (it might take up to a minute):

```
$ curl -s -o /dev/null -w "%{http_code}" localhost:8000/health
200
```

And finally send a request:

```
$ curl  http://localhost:8000/v1/completions \
      -H "Content-Type: application/json" \
      -H "x-prefiller-url: http://qwen-prefiller:8000" \
      -d '{
        "model": "facebook/opt-125m",
        "prompt": "San Francisco is a",
        "max_tokens": 50,
        "temperature": 0
      }'
```

Observe the logs to check both the prefiller and decoder got the request:

```
$ oc logs qwen-prefiller
...
[2025-04-24 20:22:08,365] LMCache INFO: Loading LMCache config file /vllm-workspace/lmcache-prefiller-config.yaml (utils.py:32:lmcache.integration.vllm.utils)
[2025-04-24 20:22:08,368] LMCache INFO: Creating LMCacheEngine instance vllm-instance (cache_engine.py:435:lmcache.experimental.cache_engine)
[2025-04-24 20:22:08,368] LMCache INFO: Creating LMCacheEngine with config: LMCacheEngineConfig(chunk_size=256, local_cpu=False, max_local_cpu_size=0, local_disk=None, max_local_disk_size=0, remote_url=None, remote_serde=None, save_decode_cache=False, enable_blending=False, blend_recompute_ratio=0.15, blend_min_tokens=256, enable_p2p=False, lookup_url=None, distributed_url=None, error_handling=False, enable_controller=False, lmcache_instance_id='lmcache_default_instance', enable_nixl=True, nixl_role='sender', nixl_peer_host='qwen-decoder', nixl_peer_port=55555, nixl_buffer_size=524288, nixl_buffer_device='cuda', nixl_enable_gc=True) (cache_engine.py:73:lmcache.experimental.cache_engine)
Loaded plugin GDS
Loaded plugin UCX
Loaded plugin UCX_MO
[2025-04-24 20:22:08,568] LMCache INFO: Received remote transfer descriptors (nixl_connector_v2.py:212:lmcache.experimental.storage_backend.connector.nixl_connector_v2)
[2025-04-24 20:22:08,568] LMCache INFO: Initializing usage context. (usage_context.py:235:lmcache.usage_context)
Initialized NIXL agent: NixlRole.SENDER
...
WARNING 04-24 20:23:43 [protocol.py:71] The following fields were present in the request but ignored: {'max_completion_tokens'}
INFO 04-24 20:23:43 [logger.py:39] Received request cmpl-c88924956e7a42a0a2013afd08d2d484-0: prompt: 'San Francisco is a', params: SamplingParams(n=1, presence_penalty=0.0, frequency_penalty=0.0, repetition_penalty=1.0, temperature=0.0, top_p=1.0, top_k=-1, min_p=0.0, seed=None, stop=[], stop_token_ids=[], bad_words=[], include_stop_str_in_output=False, ignore_eos=False, max_tokens=1, min_tokens=0, logprobs=None, prompt_logprobs=None, skip_special_tokens=True, spaces_between_special_tokens=True, truncate_prompt_tokens=None, guided_decoding=None, extra_args=None), prompt_token_ids: [2, 16033, 2659, 16, 10], lora_request: None, prompt_adapter_request: None.
INFO 04-24 20:23:43 [async_llm.py:239] Added request cmpl-c88924956e7a42a0a2013afd08d2d484-0.
INFO:     10.128.6.200:56700 - "POST /v1/completions HTTP/1.1" 200 OK
```


```
$ oc logs qwen-decoder
...
2025-04-24 20:22:05,908] LMCache INFO: Loading LMCache config file /vllm-workspace/lmcache-decoder-config.yaml (utils.py:32:lmcache.integration.vllm.utils)
[2025-04-24 20:22:05,913] LMCache INFO: Creating LMCacheEngine instance vllm-instance (cache_engine.py:435:lmcache.experimental.cache_engine)
[2025-04-24 20:22:05,913] LMCache INFO: Creating LMCacheEngine with config: LMCacheEngineConfig(chunk_size=256, local_cpu=False, max_local_cpu_size=0, local_disk=None, max_local_disk_size=0, remote_url=None, remote_serde=None, save_decode_cache=False, enable_blending=False, blend_recompute_ratio=0.15, blend_min_tokens=256, enable_p2p=False, lookup_url=None, distributed_url=None, error_handling=False, enable_controller=False, lmcache_instance_id='lmcache_default_instance', enable_nixl=True, nixl_role='receiver', nixl_peer_host='0.0.0.0', nixl_peer_port=55555, nixl_buffer_size=524288, nixl_buffer_device='cuda', nixl_enable_gc=True) (cache_engine.py:73:lmcache.experimental.cache_engine)
Loaded plugin GDS
Loaded plugin UCX
Loaded plugin UCX_MO
[2025-04-24 20:22:08,560] LMCache INFO: Sent local transfer descriptors to sender (nixl_connector_v2.py:223:lmcache.experimental.storage_backend.connector.nixl_connector_v2)
[2025-04-24 20:22:08,562] LMCache INFO: Initializing usage context. (usage_context.py:235:lmcache.usage_context)
Initialized NIXL agent: NixlRole.RECEIVER
...
INFO 04-24 20:23:43 [logger.py:39] Received request cmpl-188d9f3e998e441387c45762aee9d315-0: prompt: 'San Francisco is a', params: SamplingParams(n=1, presence_penalty=0.0, frequency_penalty=0.0, repetition_penalty=1.0, temperature=0.0, top_p=1.0, top_k=-1, min_p=0.0, seed=None, stop=[], stop_token_ids=[], bad_words=[], include_stop_str_in_output=False, ignore_eos=False, max_tokens=50, min_tokens=0, logprobs=None, prompt_logprobs=None, skip_special_tokens=True, spaces_between_special_tokens=True, truncate_prompt_tokens=None, guided_decoding=None, extra_args=None), prompt_token_ids: [2, 16033, 2659, 16, 10], lora_request: None, prompt_adapter_request: None.
INFO 04-24 20:23:43 [async_llm.py:239] Added request cmpl-188d9f3e998e441387c45762aee9d315-0.
[2025-04-24 20:23:43,555] LMCache INFO: Reqid: cmpl-188d9f3e998e441387c45762aee9d315-0, Total tokens 5, LMCache hit tokens: 0, need to load: 0 (vllm_v1_adapter.py:557:lmcache.integration.vllm.vllm_v1_adapter)
INFO:     127.0.0.1:0 - "POST /v1/completions HTTP/1.1" 200 OK
```
