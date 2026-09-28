# Copyright Axis Communications AB.
#
# For a full list of individual contributors, please see the commit history.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""ETOS API v1beta1 router tests."""

import logging
import sys
from unittest import TestCase
from unittest.mock import AsyncMock, patch

from fastapi.testclient import TestClient

from etos_api.main import APP
from etos_api.routers.v1beta1.testrun import Artifact

logging.basicConfig(level=logging.DEBUG, stream=sys.stdout)


def _test_suite(datasets: list) -> dict:
    """Create a v1beta1 test suite with one suite per dataset (None meaning no dataset key)."""
    suites = []
    for index, dataset in enumerate(datasets):
        suite = {
            "priority": 1,
            "testExecutions": [
                {
                    "id": f"00000000-0000-0000-0000-00000000000{index}",
                    "testCase": {"id": f"test_{index}", "version": "main"},
                    "execution": {
                        "checkout": ["git clone https://example.com/tests.git"],
                        "command": "pytest",
                    },
                    "environment": {
                        "environmentVariables": {},
                        "testRunner": "example.com/test-runner:latest",
                    },
                }
            ],
        }
        if dataset is not None:
            suite["dataset"] = dataset
        suites.append(suite)
    return {"name": "TestSuite", "schemaVersion": "v1beta1", "suites": suites}


class TestV1Beta1Router(TestCase):
    """Test the v1beta1 router in etos-api."""

    client = TestClient(APP)

    def _start(self, test_suite: dict, dataset) -> list:
        """Start a testrun and return the suites passed to TestRun.create."""
        artifact = Artifact(artifact_id="11111111-1111-1111-1111-111111111111", identity="pkg:x/y")
        with (
            patch(
                "etos_api.routers.v1beta1.testrun.TestRun.download_suite",
                AsyncMock(return_value=test_suite),
            ),
            patch("etos_api.routers.v1beta1.testrun.TestRun.validate_test_runners", AsyncMock()),
            patch(
                "etos_api.routers.v1beta1.testrun.TestRun.wait_for_artifact",
                AsyncMock(return_value=artifact),
            ),
            patch("etos_api.routers.v1beta1.testrun.TestRun.create", AsyncMock()) as create_mock,
        ):
            response = self.client.post(
                "/api/v1beta1/testrun",
                json={
                    "artifact_identity": "pkg:x/y",
                    "test_suite_url": "http://localhost/my_test.json",
                    "dataset": dataset,
                },
            )
        self.assertEqual(response.status_code, 200, response.text)
        create_mock.assert_awaited_once()
        return create_mock.await_args.args[-1].suites

    def test_start_testrun_suite_without_dataset(self):
        """Test that a request dataset is applied to a suite that has no dataset.

        Approval criteria:
            - POST requests to v1beta1 testrun shall return 200.
            - The request dataset shall be set on the suite.

        Test steps::
            1. Send a start request for a test suite without a dataset.
            2. Verify that the request dataset is set on the suite.
        """
        suites = self._start(_test_suite([None]), {"key": "value"})
        self.assertDictEqual(suites[0].dataset, {"key": "value"})

    def test_start_testrun_suite_with_dataset(self):
        """Test that a request dataset is merged into a suite dataset.

        Approval criteria:
            - POST requests to v1beta1 testrun shall return 200.
            - The request dataset shall be merged into the suite dataset.

        Test steps::
            1. Send a start request for a test suite with a dataset.
            2. Verify that the request dataset is merged into the suite dataset.
        """
        suites = self._start(_test_suite([{"a": 1, "key": "old"}]), {"key": "value"})
        self.assertDictEqual(suites[0].dataset, {"a": 1, "key": "value"})

    def test_start_testrun_suites_without_dataset_do_not_share(self):
        """Test that suites without a dataset do not share the same dataset object.

        Approval criteria:
            - POST requests to v1beta1 testrun shall return 200.
            - Each suite shall get its own copy of the request dataset.

        Test steps::
            1. Send a start request for a test suite with two suites without a dataset.
            2. Verify that each suite has the dataset and that they are separate objects.
        """
        suites = self._start(_test_suite([None, None]), {"key": "value"})
        self.assertDictEqual(suites[0].dataset, {"key": "value"})
        self.assertDictEqual(suites[1].dataset, {"key": "value"})
        self.assertIsNot(suites[0].dataset, suites[1].dataset)
