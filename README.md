# zwiki

A personal knowledge base powered by LLM — capture, connect, and query your notes using natural language.

## What it does

- Store notes, documents, and ideas in a structured local knowledge base
- Use LLM to search, summarize, and answer questions from your own content
- Link concepts automatically and surface relevant context when you need it

## Why

Search engines find the web. zwiki finds *your* knowledge — the notes you wrote, the docs you saved, the ideas you connected. LLM makes it conversational instead of keyword-based.

## Stack

| Layer | Choice |
|-------|--------|
| Storage | Local files / SQLite |
| Embeddings | OpenAI / local model |
| LLM | Claude / GPT-4 |
| Interface | CLI / Web UI |

## Getting started

```bash
git clone https://github.com/yourname/zwiki
cd zwiki
cp .env.example .env   # add your API key
npm install            # or pip install -r requirements.txt
npm start
```

## Usage

```bash
# Add a note
zwiki add "React useEffect runs after every render by default"

# Ask a question
zwiki ask "When should I use useEffect cleanup?"

# Search
zwiki search "React hooks"
```

## Project status

Early stage — core ingestion and query loop in progress.

## License

MIT
