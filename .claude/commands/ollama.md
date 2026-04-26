# Ollama Offload

Route a task to the local Ollama instance instead of burning Claude tokens. Use this for mechanical, repetitive, or data-heavy work where a smaller model is sufficient.

## When to use

- Parsing and structuring large HTML/text files into JSON
- Deduplicating or merging datasets
- Extracting structured data from messy sources
- Bulk formatting or transformation tasks
- Any task where you'd otherwise read large files into context just to reformat them

## How to invoke

The user or Claude can call this with a prompt describing the task and any input. Claude should:

1. Read any relevant files needed for the task
2. Construct a focused, self-contained prompt for Ollama
3. Call Ollama via the shell and capture the output
4. Use the output to continue the work — write files, summarise, etc.

## Ollama call pattern

```bash
curl -s http://localhost:11434/api/generate \
  -d '{
    "model": "qwen3.5:9b",
    "prompt": "<your prompt here>",
    "stream": false,
    "think": false,
    "options": {"num_ctx": 16000, "temperature": 0.1}
  }' | python3 -c "import sys,json; print(json.load(sys.stdin)['response'])"
```

For structured JSON output, append to the prompt:
> "Respond with only valid JSON, no commentary, no markdown fences."

## Model selection

- `qwen3.5:9b` — default, good reasoning + instruction following
- Use a smaller/faster model if the task is purely mechanical (regex-like extraction, dedup)

## $ARGUMENTS

The task description and any context the user provided: **$ARGUMENTS**

---

Construct the Ollama prompt, run it via Bash, and use the result to complete the task. Don't summarise what you're doing — just do it.
