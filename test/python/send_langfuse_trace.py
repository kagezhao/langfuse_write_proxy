import argparse
import json
import os
import time
import uuid


def main() -> int:
    parser = argparse.ArgumentParser(description="Send one Langfuse trace through a Langfuse host or proxy.")
    parser.add_argument("--url", default="http://127.0.0.1:8080", help="Langfuse or proxy base URL.")
    parser.add_argument("--public-key", required=True, help="Langfuse public key.")
    parser.add_argument("--secret-key", default="", help="Langfuse secret key. Proxy allows any value except the real secret.")
    parser.add_argument("--name-prefix", default="write-proxy-manual", help="Trace span name prefix.")
    parser.add_argument("--input", default="manual input", help="Span input text.")
    parser.add_argument("--output", default="manual output", help="Span output text.")
    parser.add_argument("--json", action="store_true", help="Print machine-readable JSON.")
    args = parser.parse_args()

    trace_id = send_trace(
        url=args.url,
        public_key=args.public_key,
        secret_key=args.secret_key,
        name_prefix=args.name_prefix,
        input_text=args.input,
        output_text=args.output,
    )

    if args.json:
        print(json.dumps({"trace_id": trace_id}))
    else:
        print(f"trace_id={trace_id}")
    return 0


def send_trace(url: str, public_key: str, secret_key: str, name_prefix: str, input_text: str, output_text: str) -> str:
    os.environ["LANGFUSE_HOST"] = url.rstrip("/")
    os.environ["LANGFUSE_PUBLIC_KEY"] = public_key
    os.environ["LANGFUSE_SECRET_KEY"] = secret_key

    from langfuse import get_client

    langfuse = get_client()
    trace_id = uuid.uuid4().hex
    cid = uuid.uuid4().hex

    with langfuse.start_as_current_span(
        name=f"{name_prefix}-{cid}",
        trace_context={"trace_id": trace_id},
    ) as root_span:
        root_span.update(input={"cid": cid, "message": input_text})
        time.sleep(0.1)
        root_span.update(output={"status": "ok", "message": output_text})

    langfuse.flush()
    return trace_id


if __name__ == "__main__":
    raise SystemExit(main())
