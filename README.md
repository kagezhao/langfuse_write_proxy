# Langfuse Write Proxy

Langfuse Write Proxy is a lightweight write-only proxy for Langfuse APIs.

It allows clients to send diagnostic and observability data to Langfuse without holding a Langfuse Secret Key directly.

The proxy uses the public key to select the project, ignores the client-provided secret key, and replaces it with the real Langfuse secret key before forwarding. To prevent accidental secret leakage, the proxy rejects requests where the client-provided secret equals the real Langfuse secret key.

Only ingestion endpoints are forwarded. Read, query, update, and delete API access is blocked.

## Allowed Endpoints

The proxy only forwards:

```text
POST /api/public/otel/v1/traces
  Current Python SDK and current TS/JS tracing SDKs use this OTEL endpoint.

POST /api/public/ingestion
  Legacy/batch ingestion endpoint. Keep it only if you need old SDKs or custom ingestion clients.
```

Requests to other `/api/public/*` endpoints are rejected.

## Configuration

The proxy is configured with a YAML file. Each project maps one Langfuse public key to one Langfuse backend and one real Langfuse secret key.

```yaml
server:
  listen_addr: ":8080"
  max_body_bytes: 20971520
  read_header_timeout: 10s

projects:
  - name: customer-a
    langfuse_base_url: "https://langfuse-a.example.com"
    langfuse_public_key: "pk-lf-..."
    langfuse_secret_key: "sk-lf-..."

  - name: customer-b
    langfuse_base_url: "https://langfuse-b.example.com"
    langfuse_public_key: "pk-lf-..."
    langfuse_secret_key: "sk-lf-..."
```

`langfuse_public_key` values must be unique.

| YAML field | Required | Default | Description |
| --- | --- | --- | --- |
| `server.listen_addr` | No | `:8080` | HTTP listen address |
| `server.max_body_bytes` | No | `20971520` | Maximum request body size |
| `server.read_header_timeout` | No | `10s` | HTTP server read header timeout |
| `projects[].name` | No | `project-N` | Human-readable project name used in logs and forwarded headers |
| `projects[].langfuse_base_url` | Yes | | Upstream Langfuse URL, for example `https://langfuse.example.com` |
| `projects[].langfuse_public_key` | Yes | | Langfuse project public key used to select this project |
| `projects[].langfuse_secret_key` | Yes | | Langfuse project secret key held by the proxy |

## Run Locally

```bash
cp config.example.yaml config.yaml
go run . -config config.yaml
```

For standard Langfuse SDKs that send Basic Auth, configure the SDK with:

```text
LANGFUSE_HOST=http://your-proxy:8080
LANGFUSE_PUBLIC_KEY=<langfuse_public_key>
LANGFUSE_SECRET_KEY=
```

`LANGFUSE_PUBLIC_KEY` must be the real Langfuse public key. `LANGFUSE_SECRET_KEY` should be left empty.

## End-to-End Test

Create a real `config.yaml`, then run:

```bash
python test/python/test_langfuse_proxy_e2e.py --config config.yaml
```

The test starts the Go proxy locally, sends a trace through the Python Langfuse SDK, verifies that read APIs are blocked through the proxy, and queries the configured Langfuse backend to confirm the trace was written.

You can also test manually after starting the proxy yourself:

```bash
go build -o langfuse-write-proxy .
./langfuse-write-proxy -config config.yaml
```

Then send one trace through the proxy:

```bash
python test/python/send_langfuse_trace.py --public-key pk-lf-...
```

If you already built the proxy binary and want the e2e runner to use it:

```bash
python test/python/test_langfuse_proxy_e2e.py --config config.yaml --proxy-command "./langfuse-write-proxy"
```

## Docker

```bash
docker build -t langfuse-write-proxy .

docker run --rm -p 8080:8080 -v "$PWD/config.yaml:/etc/langfuse-write-proxy/config.yaml:ro" langfuse-write-proxy -config /etc/langfuse-write-proxy/config.yaml
```

## Health Checks

```text
GET /healthz
GET /readyz
```

Both return `{"status":"ok"}`.
