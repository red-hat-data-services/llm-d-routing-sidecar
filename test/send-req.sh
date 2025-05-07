 curl  http://localhost:8000/v1/completions \
      -H "Content-Type: application/json" \
       -d '{
        "model": "Qwen/Qwen2-0.5B",
        "prompt": "Author-contribution statements and acknowledgements in research papers should state clearly and specifically whether, and to what extent, the authors used AI technologies such as ChatGPT in the preparation of their manuscript and analysis. They should also indicate which LLMs were used. This will alert editors and reviewers to scrutinize manuscripts more carefully for potential biases, inaccuracies and improper source crediting. Likewise, scientific journals should be transparent about their use of LLMs, for example when selecting submitted manuscripts. Mention the large language model based product mentioned in the paragraph above:",
      }'

 curl  http://localhost:8000/v1/chat/completions \
      -H "Content-Type: application/json" \
      -d '{
        "model": "Qwen/Qwen2-0.5B",
        "messages": [
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "Who won the world cup in 2020?"}
        ]
      }'


lm_eval --model local-completions --tasks gsm8k --model_args model="Qwen/Qwen2-0.5B",base_url=http://127.0.0.1:8000/v1/completions,num_concurrent=5,max_retries=3,tokenized_requests=False
