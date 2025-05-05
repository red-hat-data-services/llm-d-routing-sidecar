 curl  http://localhost:8000/v1/completions \
      -H "x-prefiller-url: http://localhost:8100"\
      -H "Content-Type: application/json" \
       -d '{
        "model": "Qwen/Qwen2-0.5B",
        "prompt": "San Francisco is a",
        "max_tokens": 50,
        "temperature": 0
      }'

#  -H "x-prefiller-url: http://10.128.5.12:8000" \


 curl  http://localhost:8000/v1/chat/completions \
      -H "Content-Type: application/json" \
      -d '{
        "model": "Qwen/Qwen2-0.5B",
        "messages": [
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "Who won the world cup in 2020?"}
        ]
      }'
