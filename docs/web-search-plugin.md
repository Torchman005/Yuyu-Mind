# Web Search Plugin

Yuyu-Mind can use `web_search.search` as a context plugin when the user asks for current, online, or source-backed information.

```text
agent/plugins/web_search/
  plugin.json
  config.json
  main.py
```

## What It Does

- Searches the web for the user's message or explicit query.
- Returns a concise list of source-backed results with title, URL, and snippet.
- Lets the normal companion reply flow turn those results into a natural answer.

## Providers

Default provider:

```json
{
  "provider": "auto"
}
```

Supported values:

- `duckduckgo`: no API key, uses DuckDuckGo HTML results.
- `brave`: uses Brave Search API.
- `tavily`: uses Tavily Search API.
- `auto`: uses Brave if configured, then Tavily if configured, otherwise DuckDuckGo.

## Configure

Edit `agent/plugins/web_search/config.json`:

```json
{
  "provider": "auto",
  "maxResults": 5,
  "timeoutSeconds": 12,
  "proxy": "http://127.0.0.1:7897",
  "brave": {
    "apiKey": "your_brave_key"
  },
  "tavily": {
    "apiKey": "your_tavily_key"
  }
}
```

You can also use environment variables:

```env
YUYU_SEARCH_PROVIDER=auto
YUYU_SEARCH_PROXY=http://127.0.0.1:7897
YUYU_SEARCH_MAX_RESULTS=5
YUYU_SEARCH_TIMEOUT_SECONDS=12
YUYU_SEARCH_BRAVE_API_KEY=your_brave_key
YUYU_SEARCH_TAVILY_API_KEY=your_tavily_key
```

## Conversation Integration

The plugin declares chat triggers in `plugin.json`, such as:

- `搜索`
- `查一下`
- `联网`
- `最新`
- `今天`
- `news`
- `latest`

When a user message matches those triggers, the Go app calls the plugin and injects the returned search context into the normal Agent prompt.

## API

List plugins:

```http
GET http://127.0.0.1:8765/plugins
```

Search:

```http
POST http://127.0.0.1:8765/plugins/web_search/search
Content-Type: application/json

{"query":"Yuyu-Mind latest news","maxResults":5}
```

Response shape:

```json
{
  "ok": true,
  "plugin": "web_search",
  "action": "search",
  "summary": "Web search results...",
  "metadata": {
    "provider": "duckduckgo",
    "query": "Yuyu-Mind latest news",
    "results": [
      {
        "title": "Result title",
        "url": "https://example.com",
        "snippet": "Result snippet"
      }
    ]
  }
}
```
