#!/usr/bin/env python3
"""Bounded native Linux/socket smoke check; leaves no collector or socket behind."""
import argparse
import http.client
import ipaddress
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time


class UnixHTTP(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__("localhost", timeout=2)
        self.path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.path)


def request(path):
    connection = UnixHTTP(path)
    try:
        connection.request("GET", "/snapshot")
        response = connection.getresponse()
        assert response.status == 200
        return json.loads(response.read())
    finally:
        connection.close()


def wait_sample(path, ready):
    deadline = time.monotonic() + 12
    while time.monotonic() < deadline:
        try:
            sample = request(path)
            if ready(sample):
                return sample
        except (OSError, http.client.HTTPException):
            pass
        time.sleep(0.05)
    raise RuntimeError("collector did not produce the expected sample within 12 seconds")


def stop(process):
    process.terminate()
    try:
        process.wait(timeout=3)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait(timeout=3)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", default=str(Path(__file__).resolve().parents[1] / "bin/weighbridge-host-collector"))
    parser.add_argument("--output", help="Optional JSON evidence output")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="host-monitor-check-") as directory:
        path = str(Path(directory) / "collector.sock")
        process = subprocess.Popen([args.binary, "-socket", path])
        try:
            first = wait_sample(path, lambda s: s["cpu"]["data"] is not None)
            assert first["host"]["data"]["hostname"] == socket.gethostname()
            assert first["cpu"]["data"]["usagePercent"] is None
            assert first["cpu"]["data"]["logicalCores"] > 0
            assert first["memory"]["data"]["totalBytes"] > 0
            assert os.stat(path).st_mode & 0o777 == 0o660
            second = wait_sample(path, lambda s: s["cpu"]["data"] and s["cpu"]["data"]["usagePercent"] is not None)
            repeated = request(path)
            assert repeated["cpu"]["collectedAt"] == second["cpu"]["collectedAt"]
            host_interfaces = set(os.listdir("/sys/class/net"))
            assert {n["id"] for n in second["network"]["data"]} == host_interfaces
            address_info = json.loads(subprocess.check_output(["ip", "-json", "address", "show"], text=True))
            expected_addresses = {item["ifname"]: {(address["family"], str(ipaddress.ip_address(address["local"])), address["prefixlen"]) for address in item["addr_info"] if address["family"] in ("inet", "inet6")} for item in address_info}
            for interface in second["network"]["data"]:
                assert not interface.get("addressError"), interface.get("addressError")
                actual = {("inet" if address["family"] == "IPv4" else "inet6", address["ip"], address["prefixLength"]) for address in interface["addresses"]}
                assert actual == expected_addresses.get(interface["id"], set()), f"IP mismatch for {interface['id']}"
            mountpoints = {line.split()[4] for line in Path("/proc/self/mountinfo").read_text().splitlines()}
            assert all(f["mountpoint"].replace(" ", "\\040") in mountpoints for f in second["filesystems"]["data"])
            stop(process)
            assert not Path(path).exists()
            try:
                request(path)
            except OSError:
                pass
            else:
                raise AssertionError("stopped collector socket unexpectedly served a sample")
            process = subprocess.Popen([args.binary, "-socket", path])
            restarted = wait_sample(path, lambda s: s["cpu"]["data"] is not None)
            assert restarted["sourceId"] != second["sourceId"]
            assert restarted["cpu"]["data"]["usagePercent"] is None
            assert all(n["receiveBytesPerSecond"] is None for n in restarted["network"]["data"])
            assert all(d["readBytesPerSecond"] is None for d in restarted["disks"]["data"])
            evidence = {"checkedAt": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "hostname": second["host"]["data"]["hostname"], "interfaces": sorted(host_interfaces), "filesystemCount": len(second["filesystems"]["data"]), "diskCount": len(second["disks"]["data"]), "gpu": second["gpu"], "cpu": second["cpu"], "memory": second["memory"], "socketPermissions": "0660", "repeatPreservesTime": True, "restartResetsBaselines": True, "nativeHostScope": True, "ipAddressesMatchHost": True}
            print(json.dumps(evidence, ensure_ascii=False, indent=2))
            if args.output:
                Path(args.output).write_text(json.dumps(evidence, ensure_ascii=False, indent=2) + "\n")
        finally:
            if process.poll() is None:
                stop(process)


if __name__ == "__main__":
    main()
