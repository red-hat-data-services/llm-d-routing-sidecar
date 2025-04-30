 curl  http://localhost:8000//v1/completions \
      -H "Content-Type: application/json" \
      -H "x-prefiller-url: http://10.129.5.160:8000" \
      -d '{
        "model": "facebook/opt-125m",
        "prompt": "San Francisco is a",
        "max_tokens": 50,
        "temperature": 0
      }'

#  -H "x-prefiller-url: http://10.128.5.12:8000" \


 curl  http://localhost:8000/fb/v1/chat/completions \
      -H "Content-Type: application/json" \
      -d '{
        "model": "facebook/opt-125m",
        "messages": [
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "Who won the world cup in 2020?"}
        ]
      }'
