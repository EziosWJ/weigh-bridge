#!/usr/bin/env python3
"""Verify real native/Docker APIs against a host socket using temporary data only.
Run `task build` first. Docker image golang:1.26.5-bookworm must be available.
All processes, containers, ports and temporary PostgreSQL databases are bounded and cleaned up.
"""
import argparse
import datetime
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

import importlib.util

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("host_check", Path(__file__).with_name("check-host-collector.py"))
host_check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(host_check)
ROOT = Path(__file__).resolve().parents[1]


def run(command, **kwargs):
    return subprocess.run(command, check=True, text=True, capture_output=True, timeout=30, **kwargs).stdout.strip()


def wait_until(check, seconds=15):
    deadline = time.monotonic() + seconds
    last_error = None
    while time.monotonic() < deadline:
        try:
            value = check()
            if value:
                return value
        except (OSError, ValueError, KeyError, TypeError, urllib.error.URLError) as error:
            last_error = error
        time.sleep(0.15)
    raise AssertionError(f"Expected monitoring state did not arrive: {last_error}")


def http(base, path, token=None, body=None):
    headers = {"Authorization": token} if token else {}
    if body is not None:
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(base + path, data=None if body is None else json.dumps(body).encode(), headers=headers)
    with urllib.request.urlopen(request, timeout=3) as response:
        return json.load(response)


def login(base):
    wait_until(lambda: http(base, "/health")["code"] == 200)
    wait_until(lambda: http(base, "/ready")["code"] == 200)
    return http(base, "/api/auth/login", body={"username": "admin", "password": "admin123"})["data"]["tokenValue"]


def overview(base, token):
    return http(base, "/api/v1/monitoring/overview", token)["data"]


def fresh_overview(base, token):
    return wait_until(lambda: (s if s["cpu"]["status"] == "ok" and s["cpu"]["data"]["usagePercent"] is not None else None) if (s := overview(base, token)) else None)


def history(base, token):
    return http(base, "/api/v1/monitoring/history?resource=cpu", token)["data"]["points"]


def free_port():
    with socket.socket() as connection:
        connection.bind(("127.0.0.1", 0))
        return connection.getsockname()[1]


def check_scope(sample):
    assert sample["host"]["data"]["hostname"] == socket.gethostname()
    assert {item["id"] for item in sample["network"]["data"]} == set(os.listdir("/sys/class/net"))
    mounts = {line.split()[4] for line in Path("/proc/self/mountinfo").read_text().splitlines()}
    assert sample["filesystems"]["data"]
    assert all(item["mountpoint"].replace(" ", "\\040") in mounts for item in sample["filesystems"]["data"])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", help="Write JSON acceptance evidence")
    args = parser.parse_args()
    binaries = {name: ROOT / "bin" / name for name in ("weighbridge-server", "weighbridge-migrate", "weighbridge-host-collector")}
    for path in binaries.values():
        if not path.is_file():
            raise RuntimeError(f"Missing {path}; run task build first")
    processes = []
    container = "spec64-monitor-" + str(os.getpid())
    database_container = container + "-postgres"
    network = container + "-network"
    with tempfile.TemporaryDirectory(prefix="spec64-api-") as directory:
        try:
            run(["docker", "network", "create", network])
            run(["docker", "run", "-d", "--rm", "--name", database_container, "--network", network,
                 "-e", "POSTGRES_DB=integration", "-e", "POSTGRES_USER=integration", "-e", "POSTGRES_PASSWORD=temporary-monitoring-password",
                 "-p", "127.0.0.1::5432", "postgres:17-alpine"])
            db_port = run(["docker", "port", database_container, "5432/tcp"]).rsplit(":", 1)[1]
            def database_ready():
                result = subprocess.run(["docker", "exec", "-e", "PGPASSWORD=temporary-monitoring-password", database_container,
                    "psql", "-h", "127.0.0.1", "-U", "integration", "-d", "integration", "-tAc", "SELECT 1"],
                    capture_output=True, timeout=5)
                return result.returncode == 0
            wait_until(database_ready, seconds=45)
            work = Path(directory)
            (work / "configs").mkdir()
            shutil.copyfile(ROOT / "server/configs/config.yaml", work / "configs/config.yaml")
            (work / "migrations").symlink_to(ROOT / "server/migrations", target_is_directory=True)
            env = {key: value for key, value in os.environ.items() if not key.startswith("APP_")}
            env["APP_MONITORING__ENABLED"] = "true"
            env.update({"APP_ENV": "test", "APP_DATABASE__DRIVER": "postgres", "APP_DATABASE__URL": f"postgres://127.0.0.1:{db_port}/integration?sslmode=disable", "APP_DATABASE__USERNAME": "integration", "APP_DATABASE__PASSWORD": "temporary-monitoring-password", "APP_JWT__SECRET": "spec64-temporary-acceptance-secret-32-bytes", "APP_FILE__STORAGE_ROOT": str(work / "uploads")})
            run([str(binaries["weighbridge-migrate"]), "up", "--kind", "all"], cwd=work, env=env)
            port = free_port()
            base = f"http://127.0.0.1:{port}"
            env["APP_HTTP__ADDRESS"] = f"127.0.0.1:{port}"
            native_log = open(work / "native.log", "w")
            native = subprocess.Popen([str(binaries["weighbridge-server"])], cwd=work, env=env, stdout=native_log, stderr=native_log)
            processes.append(native)
            token = login(base)
            native_sample = fresh_overview(base, token)
            check_scope(native_sample)
            with urllib.request.urlopen(base + "/monitor/server", timeout=3) as response:
                assert response.status == 200 and b"<html" in response.read()
            host_check.stop(native)
            native_log.close()
            socket_dir = work / "socket"
            socket_dir.mkdir(mode=0o750)
            socket_path = str(socket_dir / "collector.sock")
            collector_log = open(work / "collector.log", "w")
            collector = subprocess.Popen([str(binaries["weighbridge-host-collector"]), "-socket", socket_path], stdout=collector_log, stderr=collector_log)
            processes.append(collector)
            host_check.wait_sample(socket_path, lambda s: s["cpu"]["data"] is not None)
            port = free_port()
            base = f"http://127.0.0.1:{port}"
            docker_env = dict(env)
            docker_env.update({"APP_DATABASE__URL": f"postgres://{database_container}:5432/integration?sslmode=disable", "APP_FILE__STORAGE_ROOT": "/acceptance/uploads", "APP_HTTP__ADDRESS": ":8080", "APP_MONITORING__SOURCE": "unix", "APP_MONITORING__SOCKET_PATH": "/host-monitor/collector.sock"})
            command = ["docker", "run", "-d", "--name", container, "--network", network, "--user", f"{os.getuid()}:{os.getgid()}", "--hostname", "spec64-isolated-api", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "-p", f"127.0.0.1:{port}:8080", "-v", f"{work}:/acceptance", "-v", f"{socket_dir}:/host-monitor:ro", "-v", f"{binaries['weighbridge-server']}:/app/api:ro", "-w", "/acceptance"]
            for key, value in docker_env.items():
                if key.startswith("APP_"):
                    command += ["-e", f"{key}={value}"]
            run(command + ["golang:1.26.5-bookworm", "/app/api"])
            token = login(base)
            docker_sample = fresh_overview(base, token)
            check_scope(docker_sample)
            actual_hostname = run(["docker", "exec", container, "hostname"])
            assert actual_hostname == "spec64-isolated-api" and actual_hostname != docker_sample["host"]["data"]["hostname"]
            baseline_source = docker_sample["sourceId"]
            baseline_points = history(base, token)
            assert baseline_points
            # Remove group/owner access temporarily, preserving original mode.
            original_mode = os.stat(socket_path).st_mode & 0o777
            os.chmod(socket_path, 0)
            permission = wait_until(lambda: (s if s["cpu"]["status"] == "error" else None) if (s := overview(base, token)) else None)
            assert "permission denied" in permission["cpu"]["message"].lower()
            permission_time = permission["cpu"]["collectedAt"]
            assert permission_time is not None
            time.sleep(6)  # Allow another five-second API sample to fail.
            repeated_permission = overview(base, token)
            assert repeated_permission["cpu"]["status"] == "error"
            assert "permission denied" in repeated_permission["cpu"]["message"].lower()
            assert repeated_permission["cpu"]["collectedAt"] == permission_time
            os.chmod(socket_path, original_mode)
            fresh_overview(base, token)
            host_check.stop(collector)
            disconnected = wait_until(lambda: (s if s["cpu"]["status"] == "error" else None) if (s := overview(base, token)) else None)
            assert disconnected["sourceId"] == baseline_source
            stale = wait_until(lambda: (s if s["cpu"]["stale"] else None) if (s := overview(base, token)) else None, seconds=20)
            assert stale["cpu"]["status"] == "error"
            collector = subprocess.Popen([str(binaries["weighbridge-host-collector"]), "-socket", socket_path], stdout=collector_log, stderr=collector_log)
            processes.append(collector)
            restarted = host_check.wait_sample(socket_path, lambda s: s["cpu"]["data"] is not None)
            assert restarted["sourceId"] != baseline_source and restarted["cpu"]["data"]["usagePercent"] is None
            restored = fresh_overview(base, token)
            assert restored["sourceId"] != baseline_source
            points = history(base, token)
            assert any(point["collectedAt"] == baseline_points[0]["collectedAt"] for point in points)
            assert any(point["values"]["usagePercent"] is None for point in points)
            evidence = {"checkedAt": datetime.datetime.now(datetime.timezone.utc).isoformat(), "nativeAPI": True, "embeddedSPA": True, "dockerAPI": True, "hostHostname": restored["host"]["data"]["hostname"], "containerHostname": actual_hostname, "hostInterfaces": sorted(item["id"] for item in restored["network"]["data"]), "filesystemCount": len(restored["filesystems"]["data"]), "socketPermissionDenied": True, "disconnectAndStale": True, "restartResetsBaseline": True, "apiHistoryPreserved": True, "historyGap": True, "gpu": restored["gpu"]}
            print(json.dumps(evidence, ensure_ascii=False, indent=2))
            if args.output:
                Path(args.output).write_text(json.dumps(evidence, ensure_ascii=False, indent=2) + "\n")
            collector_log.close()
        finally:
            try:
                for process in processes:
                    if process.poll() is None:
                        host_check.stop(process)
            finally:
                subprocess.run(["docker", "rm", "-f", container, database_container], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10)
                subprocess.run(["docker", "network", "rm", network], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10)


if __name__ == "__main__":
    main()
