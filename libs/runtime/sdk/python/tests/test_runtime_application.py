import subprocess
from typing import List
import os
import time

import pytest
import requests


def test_http_endpoint(runtime_server):
    response = requests.post("http://localhost:22346/orders/2393483", json={})
    assert response.status_code == 200
    assert response.json() == {"message": "Order received"}


@pytest.fixture(scope="session")
def runtime_server(command_args: List[str]):

    server_proc = subprocess.Popen(
        command_args,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )
    # Give the server time to start up.
    time.sleep(2)

    yield server_proc
    server_proc.terminate()


@pytest.fixture(name="command_args", scope="session")
def fixture_command_args() -> List[str]:
    if os.getenv("GITHUB_ACTIONS"):
        return [
            "python",
            "tests/server.py",
        ]
    return [
        "pipenv",
        "run",
        "python",
        "tests/server.py",
    ]
