import argparse
import base64
import json
import os
import signal
import shlex
import subprocess
import sys
import time
from pathlib import Path

import requests
import yaml


ROOT = Path(__file__).resolve().parents[2]


def main() -> int:
    parser = argparse.ArgumentParser(description="Run an end-to-end Langfuse Write Proxy test.")
    parser.add_argument("--config", default=str(ROOT / "config.yaml"), help="Path to proxy YAML config.")
    parser.add_argument("--proxy-url", default="http://127.0.0.1:8080", help="Local proxy URL.")
    parser.add_argument("--project", default="", help="Project name to test. Defaults to the first project.")
    parser.add_argument("--timeout", type=int, default=60, help="Seconds to wait for trace visibility.")
    parser.add_argument(
        "--proxy-command",
        default="go run .",
        help="Command used to start the proxy. The e2e runner appends '-config <path>'.",
    )
    args = parser.parse_args()

    config_path = Path(args.config).resolve()
    cfg = load_config(config_path)
    project = select_project(cfg, args.project)
    proxy_url = args.proxy_url.rstrip("/")

    print(f"checking upstream credentials for project={project.get('name', '<unnamed>')}", flush=True)
    assert_upstream_credentials_are_valid(project)

    proc = start_proxy(args.proxy_command, config_path)
    try:
        print(f"waiting for proxy at {proxy_url}", flush=True)
        wait_for_health(proxy_url)
        print("proxy is healthy", flush=True)

        print("checking that read API is blocked by proxy", flush=True)
        assert_public_read_is_blocked(proxy_url, project["langfuse_public_key"])

        print("writing trace through standalone Langfuse sender", flush=True)
        trace_id = send_trace_with_worker(proxy_url, project, args.timeout)
        print(f"trace submitted through proxy: {trace_id}", flush=True)

        print("checking trace visibility on upstream Langfuse", flush=True)
        wait_for_trace(project, trace_id, args.timeout)
        print(f"e2e ok: trace_id={trace_id} project={project.get('name', '<unnamed>')}")
        return 0
    finally:
        stop_proxy(proc)


def start_proxy(proxy_command: str, config_path: Path) -> subprocess.Popen:
    command = [*shlex.split(proxy_command, posix=os.name != "nt"), "-config", str(config_path)]
    popen_kwargs = {}
    if os.name == "nt":
        popen_kwargs["creationflags"] = subprocess.CREATE_NEW_PROCESS_GROUP
    else:
        popen_kwargs["start_new_session"] = True

    return subprocess.Popen(
        command,
        cwd=ROOT,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        **popen_kwargs,
    )


def load_config(path: Path) -> dict:
    with path.open("r", encoding="utf-8") as f:
        cfg = yaml.safe_load(f)
    if not cfg or not get_projects(cfg):
        raise RuntimeError("config must contain at least one project")
    return cfg


def get_projects(cfg: dict) -> list[dict]:
    return cfg.get("projects") or []


def select_project(cfg: dict, name: str) -> dict:
    projects = get_projects(cfg)
    if not name:
        return projects[0]
    for project in projects:
        if project.get("name") == name:
            return project
    raise RuntimeError(f"project {name!r} not found")


def wait_for_health(proxy_url: str) -> None:
    deadline = time.time() + 30
    last_error = None
    while time.time() < deadline:
        try:
            res = requests.get(f"{proxy_url}/healthz", timeout=2)
            if res.status_code == 200:
                return
            last_error = RuntimeError(f"health status={res.status_code}")
        except Exception as exc:
            last_error = exc
        time.sleep(0.5)
    raise RuntimeError(f"proxy did not become healthy: {last_error}")


def assert_public_read_is_blocked(proxy_url: str, public_key: str) -> None:
    auth = base64.b64encode(f"{public_key}:sk-lf-none".encode("utf-8")).decode("ascii")
    res = requests.get(
        f"{proxy_url}/api/public/traces",
        headers={"Authorization": f"Basic {auth}"},
        timeout=10,
    )
    if res.status_code != 403:
        raise RuntimeError(f"expected read API to be blocked with 403, got {res.status_code}: {res.text[:200]}")


def assert_upstream_credentials_are_valid(project: dict) -> None:
    base_url = project["langfuse_base_url"].rstrip("/")
    auth = (project["langfuse_public_key"], project["langfuse_secret_key"])
    res = requests.get(f"{base_url}/api/public/traces?limit=1", auth=auth, timeout=15)
    if res.status_code == 200:
        return
    if res.status_code == 401:
        raise RuntimeError(
            "upstream Langfuse credentials are invalid for the selected project. "
            "Check langfuse_public_key and langfuse_secret_key in config.yaml."
        )
    raise RuntimeError(f"upstream credential check failed with status={res.status_code}: {res.text[:300]}")


def send_trace_with_worker(proxy_url: str, project: dict, timeout: int) -> str:
    res = subprocess.run(
        [
            sys.executable,
            str(ROOT / "test" / "python" / "send_langfuse_trace.py"),
            "--url",
            proxy_url,
            "--public-key",
            project["langfuse_public_key"],
            "--secret-key",
            "sk-lf-none",
            "--name-prefix",
            "write-proxy-e2e",
            "--input",
            "e2e input",
            "--output",
            "e2e output",
            "--json",
        ],
        cwd=ROOT,
        capture_output=True,
        text=True,
        timeout=timeout,
    )
    if res.returncode != 0:
        raise RuntimeError(
            "Langfuse sender failed\n"
            f"stdout:\n{res.stdout}\n"
            f"stderr:\n{res.stderr}"
        )

    lines = [line for line in res.stdout.splitlines() if line.strip()]
    if not lines:
        raise RuntimeError("Langfuse sender did not return a trace id")
    payload = json.loads(lines[-1])
    return payload["trace_id"]


def wait_for_trace(project: dict, trace_id: str, timeout: int) -> None:
    base_url = project["langfuse_base_url"].rstrip("/")
    auth = (project["langfuse_public_key"], project["langfuse_secret_key"])
    deadline = time.time() + timeout
    last_status = None
    last_body = ""

    while time.time() < deadline:
        res = requests.get(f"{base_url}/api/public/traces/{trace_id}", auth=auth, timeout=10)
        last_status = res.status_code
        last_body = res.text[:300]
        if res.status_code == 200:
            payload = res.json()
            if payload.get("id") == trace_id:
                return
        time.sleep(2)

    raise RuntimeError(f"trace was not visible after {timeout}s; last_status={last_status}; body={last_body}")


def stop_proxy(proc: subprocess.Popen) -> None:
    if proc.poll() is not None:
        dump_proxy_output(proc)
        return

    if os.name == "nt":
        subprocess.run(["taskkill", "/F", "/T", "/PID", str(proc.pid)], capture_output=True, text=True)
    else:
        os.killpg(proc.pid, signal.SIGTERM)
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            os.killpg(proc.pid, signal.SIGKILL)
            proc.wait(timeout=5)
    dump_proxy_output(proc)


def dump_proxy_output(proc: subprocess.Popen) -> None:
    if not proc.stdout:
        return
    output = proc.stdout.read()
    if output.strip():
        print("proxy output:")
        print(output)


if __name__ == "__main__":
    raise SystemExit(main())
