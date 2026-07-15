# Langfuse Write Proxy

[English](README.md)

**这个项目是什么**

Langfuse Write Proxy 是一个轻量级的 Langfuse API 只写代理。它允许客户端向 Langfuse 上报观测数据时，只需要持有 Langfuse `项目公钥`，不需要持有 `项目私钥`。

**这个项目解决什么问题**

如果把 Langfuse 项目私钥暴露给客户端，客户端不只是能上报观测数据，还可以用这组密钥调用 Langfuse API，浏览这个 project 中已经存在的数据。

这个项目相当于一层 firewall：客户端只需要把数据上报到代理，不需要接触真正的 Langfuse API，也不需要接触真实项目私钥。

**是如何实现的**

代理收到客户端请求带的 public key 和 secret key 时，会先通过 public key 定位到配置中的 project，忽略客户端传来的 secret key，并在转发前替换为真实的 Langfuse secret key。

这个项目的设计原则是：客户端侧不应该出现真实 Langfuse secret key。为避免误泄露，比如运维忘了改客户端配置、误把真实 secret key 下发到机器上，若客户端传来的 secret key 等于真实 Langfuse secret key，代理会拒绝请求。

代理只转发上报端点。读取、查询、更新、删除 API 都会被阻断。

代理本身是无状态的，支持水平扩展部署。

## 允许的端点

代理只转发：

```text
POST /api/public/otel/v1/traces
  当前 Python SDK 和当前 TS/JS tracing SDK 使用这个 OTEL 端点。

POST /api/public/ingestion
  Legacy/batch ingestion 端点。只有需要旧 SDK 或自定义 ingestion 客户端时才保留。
```

其他 `/api/public/*` 请求都会被拒绝。

## 配置

代理使用 YAML 配置文件。每个 project 对应一个 Langfuse public key、一个 Langfuse 后端和一个真实 Langfuse secret key。

```yaml
server:
  listen_addr: ":12000"
  max_body_bytes: 20971520
  read_header_timeout: 30s

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

`langfuse_public_key` 必须唯一。

| YAML 字段 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `server.listen_addr` | 否 | `:12000` | HTTP 监听地址 |
| `server.max_body_bytes` | 否 | `20971520` | 最大请求体大小 |
| `server.read_header_timeout` | 否 | `30s` | HTTP 请求头读取超时 |
| `projects[].name` | 否 | `project-N` | 用于日志和转发 header 的 project 名称 |
| `projects[].langfuse_base_url` | 是 | | 上游 Langfuse 地址，例如 `https://langfuse.example.com` |
| `projects[].langfuse_public_key` | 是 | | 用于选择 project 的 Langfuse public key |
| `projects[].langfuse_secret_key` | 是 | | 由代理持有的 Langfuse secret key |

## 本地运行

```bash
cp config.example.yaml config.yaml
go run . -config config.yaml
```

也可以直接构建并运行二进制：

```bash
go build -o langfuse-write-proxy .
./langfuse-write-proxy -config config.yaml
```

在 Linux/macOS 上编译 Linux x64 版本：

```bash
GOOS=linux GOARCH=amd64 go build .
```

在 Windows cmd 上编译 Linux x64 版本：

```cmd
set GOOS=linux&& set GOARCH=amd64&& go build .
```

标准 Langfuse SDK 使用 Basic Auth，可这样配置：

```text
LANGFUSE_HOST=http://your-proxy:12000
LANGFUSE_PUBLIC_KEY=<langfuse_public_key>
LANGFUSE_SECRET_KEY=
```

`LANGFUSE_PUBLIC_KEY` 必须是真实 Langfuse public key。`LANGFUSE_SECRET_KEY` 应该留空。

启动代理后，通过示例代码测试 trace 上报：

```bash
python test/python/send_langfuse_trace.py --public-key pk-lf-...
```

## 开发者自动化测试

创建真实 `config.yaml` 后运行：

```bash
python test/python/test_langfuse_proxy_e2e.py --config config.yaml
```

测试会启动本地 Go 代理，通过 Python Langfuse SDK 写入一条 trace，验证读 API 被代理阻断，并查询上游 Langfuse 确认 trace 已写入。

如果已经构建好二进制，可以让 e2e 使用它：

```bash
python test/python/test_langfuse_proxy_e2e.py --config config.yaml --proxy-command "./langfuse-write-proxy"
```

## Docker

```bash
docker build -t langfuse-write-proxy .

docker run --rm -p 12000:12000 -v "$PWD/config.yaml:/etc/langfuse-write-proxy/config.yaml:ro" langfuse-write-proxy -config /etc/langfuse-write-proxy/config.yaml
```

## 健康检查

```text
GET /healthz
GET /readyz
```

都会返回 `{"status":"ok"}`。
