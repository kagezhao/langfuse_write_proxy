# Langfuse Write Proxy

[中文](README.md)

**What This Project Is**

Langfuse Write Proxy is a lightweight write-only proxy for Langfuse APIs. It allows clients to send observability data to Langfuse while only holding the Langfuse `project public key`, without holding the `project secret key`.

**What Problem It Solves**

If the Langfuse project secret key is exposed to a client, the client can do more than send observability data. It can also call Langfuse APIs with that key and browse existing data in the same project.

This project acts like a firewall: clients only send data to the proxy, without touching the real Langfuse API or the real project secret key.

**How It Works**

When the proxy receives a client request with a public key and secret key, it uses the public key to find the configured project, ignores the client-provided secret key, and replaces it with the real Langfuse secret key before forwarding.

The design principle is that clients should never contain the real Langfuse secret key. To prevent accidental leakage, for example when an operator forgets to change client configuration and deploys the real secret key to a machine, the proxy rejects requests where the client-provided secret key equals the real Langfuse secret key.

Only ingestion endpoints are forwarded. Read, query, update, and delete API access is blocked.

The proxy is stateless and can be deployed with horizontal scaling.

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
  listen_addr: ":12000"
  max_body_bytes: 20971520
  read_header_timeout: 30s
  idle_timeout: 90s

upstream:
  max_idle_conns: 200
  max_idle_conns_per_host: 50
  idle_conn_timeout: 90s

log:
  dir: "logs"
  max_days: 7

projects:
  - name: project-a
    langfuse_base_url: "https://langfuse-a.example.com"
    langfuse_public_key: "pk-lf-..."
    langfuse_secret_key: "sk-lf-..."

  - name: project-b
    langfuse_base_url: "https://langfuse-b.example.com"
    langfuse_public_key: "pk-lf-..."
    langfuse_secret_key: "sk-lf-..."
```

`langfuse_public_key` values must be unique.

| YAML field | Required | Default | Description |
| --- | --- | --- | --- |
| `server.listen_addr` | No | `:12000` | HTTP listen address |
| `server.max_body_bytes` | No | `20971520` | Maximum request body size |
| `server.read_header_timeout` | No | `30s` | HTTP server read header timeout |
| `server.idle_timeout` | No | `90s` | HTTP keep-alive idle timeout for client connections |
| `upstream.max_idle_conns` | No | `200` | Maximum idle connections kept for upstream Langfuse requests |
| `upstream.max_idle_conns_per_host` | No | `50` | Maximum idle connections kept per upstream host |
| `upstream.idle_conn_timeout` | No | `90s` | Keep-alive idle timeout for upstream Langfuse connections |
| `log.dir` | No | `logs` | Directory for daily log files |
| `log.max_days` | No | `7` | Maximum number of daily log files to keep |
| `projects[].name` | No | `project-N` | Human-readable project name used in logs and forwarded headers |
| `projects[].langfuse_base_url` | Yes | | Upstream Langfuse URL, for example `https://langfuse.example.com` |
| `projects[].langfuse_public_key` | Yes | | Langfuse project public key used to select this project |
| `projects[].langfuse_secret_key` | Yes | | Langfuse project secret key held by the proxy |

## Run Locally

```bash
cp config.example.yaml config.yaml
go run . -config config.yaml
```

You can also build and run the binary directly:

```bash
go build -o langfuse-write-proxy .
./langfuse-write-proxy -config config.yaml
```

Build for Linux x64 on Linux/macOS:

```bash
GOOS=linux GOARCH=amd64 go build .
```

Build for Linux x64 on Windows cmd:

```cmd
set GOOS=linux&& set GOARCH=amd64&& go build .
```

For standard Langfuse SDKs that send Basic Auth, configure the SDK with:

```text
LANGFUSE_HOST=http://your-proxy:12000
LANGFUSE_PUBLIC_KEY=<langfuse_public_key>
LANGFUSE_SECRET_KEY=
```

`LANGFUSE_PUBLIC_KEY` must be the real Langfuse public key. `LANGFUSE_SECRET_KEY` should be left empty.

After starting the proxy, send one trace through it:

```bash
python test/python/send_langfuse_trace.py --public-key pk-lf-...
```

## Developer Automated Test

Create a real `config.yaml`, then run:

```bash
python test/python/test_langfuse_proxy_e2e.py --config config.yaml
```

The test starts the Go proxy locally, sends a trace through the Python Langfuse SDK, verifies that read APIs are blocked through the proxy, and queries the configured Langfuse backend to confirm the trace was written.

If you already built the proxy binary and want the e2e runner to use it:

```bash
python test/python/test_langfuse_proxy_e2e.py --config config.yaml --proxy-command "./langfuse-write-proxy"
```

## Docker

```bash
docker build -t langfuse-write-proxy .

docker run --rm -p 12000:12000 -v "$PWD/config.yaml:/etc/langfuse-write-proxy/config.yaml:ro" langfuse-write-proxy -config /etc/langfuse-write-proxy/config.yaml
```

## Health Checks

```text
GET /healthz
GET /readyz
```

Both return `{"status":"ok"}`.
