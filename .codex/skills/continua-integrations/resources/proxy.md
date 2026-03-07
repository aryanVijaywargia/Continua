# LLM Proxy

## Overview

The proxy intercepts LLM API calls, captures request/response, and forwards to the actual provider.

```
Client App
    ↓
Continua Proxy (localhost:8081)
    ↓ capture
    ↓
OpenAI/Anthropic API
    ↓
Response captured + forwarded
```

## Usage

```bash
# Instead of
export OPENAI_API_KEY=sk-xxx
export OPENAI_BASE_URL=https://api.openai.com/v1

# Use
export OPENAI_BASE_URL=http://localhost:8081/proxy/openai/v1
```

## Architecture

```
internal/proxy/
├── handler.go       # Main proxy handler
├── providers/
│   ├── openai.go    # OpenAI-specific logic
│   └── anthropic.go # Anthropic-specific logic
├── capture.go       # Request/response capture
└── streaming.go     # SSE streaming support
```

## Proxy Handler

```go
// internal/proxy/handler.go

func NewProxyHandler(queries *platform.Queries) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 1. Determine provider from path
        provider := extractProvider(r.URL.Path)  // "openai" or "anthropic"

        // 2. Create trace/span
        traceID := getOrCreateTrace(r)
        spanID := createSpan(traceID, "llm_call", "LLM")

        // 3. Capture request body
        reqBody, _ := io.ReadAll(r.Body)
        storePayload(spanID, "request", reqBody)
        r.Body = io.NopCloser(bytes.NewReader(reqBody))

        // 4. Forward to provider
        resp, err := forwardRequest(r, provider)
        if err != nil {
            handleError(w, spanID, err)
            return
        }

        // 5. Handle streaming vs non-streaming
        if isStreamingRequest(reqBody) {
            handleStreaming(w, resp, spanID)
        } else {
            handleNonStreaming(w, resp, spanID)
        }
    })
}
```

## Provider Configuration

```go
var providerURLs = map[string]string{
    "openai":    "https://api.openai.com",
    "anthropic": "https://api.anthropic.com",
}

func forwardRequest(r *http.Request, provider string) (*http.Response, error) {
    targetURL := providerURLs[provider]

    // Rewrite URL
    proxyURL, _ := url.Parse(targetURL)
    r.URL.Host = proxyURL.Host
    r.URL.Scheme = proxyURL.Scheme
    r.Host = proxyURL.Host

    // Forward
    client := &http.Client{Timeout: 5 * time.Minute}
    return client.Do(r)
}
```

## Streaming Support

OpenAI uses Server-Sent Events (SSE) for streaming:

```go
func handleStreaming(w http.ResponseWriter, resp *http.Response, spanID uuid.UUID) {
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming not supported", 500)
        return
    }

    // Copy headers
    for k, v := range resp.Header {
        w.Header()[k] = v
    }
    w.WriteHeader(resp.StatusCode)

    // Stream chunks
    scanner := bufio.NewScanner(resp.Body)
    var fullContent strings.Builder
    chunkIndex := 0

    for scanner.Scan() {
        line := scanner.Text()

        // Write to client
        fmt.Fprintln(w, line)
        flusher.Flush()

        // Capture content from data: lines
        if strings.HasPrefix(line, "data: ") {
            content := extractContent(line)
            fullContent.WriteString(content)

            // Broadcast chunk via WebSocket
            broadcastStreamChunk(spanID, chunkIndex, content)
            chunkIndex++
        }
    }

    // Store complete response
    storePayload(spanID, "response", fullContent.String())
    completeSpan(spanID)
}
```

## Token Counting

Extract token counts from provider responses:

```go
// OpenAI response structure
type OpenAIResponse struct {
    Usage struct {
        PromptTokens     int `json:"prompt_tokens"`
        CompletionTokens int `json:"completion_tokens"`
    } `json:"usage"`
}

func extractMetrics(body []byte, provider string) (tokensIn, tokensOut int) {
    switch provider {
    case "openai":
        var resp OpenAIResponse
        json.Unmarshal(body, &resp)
        return resp.Usage.PromptTokens, resp.Usage.CompletionTokens
    case "anthropic":
        // Anthropic format
    }
    return 0, 0
}
```

## Cost Calculation

```go
var costPerToken = map[string]struct{ input, output float64 }{
    "gpt-4-turbo":     {0.00001, 0.00003},
    "gpt-3.5-turbo":   {0.0000005, 0.0000015},
    "claude-3-opus":   {0.000015, 0.000075},
    "claude-3-sonnet": {0.000003, 0.000015},
}

func calculateCost(model string, tokensIn, tokensOut int) float64 {
    rates, ok := costPerToken[model]
    if !ok {
        return 0
    }
    return float64(tokensIn)*rates.input + float64(tokensOut)*rates.output
}
```

## Trace Correlation

Link proxy requests to existing traces via header:

```go
func getOrCreateTrace(r *http.Request) uuid.UUID {
    // Check for existing trace ID in header
    if traceID := r.Header.Get("X-Continua-Trace-ID"); traceID != "" {
        return uuid.MustParse(traceID)
    }

    // Create new trace
    trace, _ := queries.CreateTrace(ctx, platform.CreateTraceParams{
        Name:   extractModelName(r),
        Status: "RUNNING",
    })
    return trace.ID
}
```

## Routes

```go
// cmd/continua/main.go

r.Route("/proxy", func(r chi.Router) {
    r.HandleFunc("/openai/*", proxyHandler)
    r.HandleFunc("/anthropic/*", proxyHandler)
})
```
