# Copyright 2020 Axis Communications AB.
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
"""ETOS API routers."""
import json
import logging
import sys
from unittest import TestCase
from unittest.mock import patch

from etos_lib.lib.config import Config
from etos_lib.lib.debug import Debug
from fastapi.testclient import TestClient

from etos_api.main import APP
from etos_api.routers.v1alpha.router import convert_to_rfc1123
from tests.fake_database import FakeDatabase

logging.basicConfig(level=logging.DEBUG, stream=sys.stdout)
IUT_PROVIDER = {
    "iut": {
        "id": "default",
        "list": {
            "possible": {
                "$expand": {
                    "value": {
                        "type": "$identity.type",
                        "namespace": "$identity.namespace",
                        "name": "$identity.name",
                        "version": "$identity.version",
                        "qualifiers": "$identity.qualifiers",
                        "subpath": "$identity.subpath",
                    },
                    "to": "$amount",
                }
            },
            "available": "$this.possible",
        },
    }
}

EXECUTION_SPACE_PROVIDER = {
    "execution_space": {
        "id": "default",
        "list": {
            "possible": {
                "$expand": {
                    "value": {"instructions": "$execution_space_instructions"},
                    "to": "$amount",
                }
            },
            "available": "$this.possible",
        },
    }
}


LOG_AREA_PROVIDER = {
    "log": {
        "id": "default",
        "list": {
            "possible": {
                "$expand": {
                    "value": {"upload": {"url": "$dataset.host", "method": "GET"}},
                    "to": "$amount",
                }
            },
            "available": "$this.possible",
        },
    }
}


class TestRouters(TestCase):
    """Test the routers in etos-api."""

    logger = logging.getLogger(__name__)
    client = TestClient(APP)

    def tearDown(self):
        """Cleanup events from ETOS library debug."""
        Config().reset()
        Debug().events_received.clear()
        Debug().events_published.clear()

    @patch("etos_api.library.validator.Docker.digest")
    @patch("etos_api.routers.v0.utilities.download_suite")
    @patch("etos_api.library.graphql.GraphqlQueryHandler.execute")
    def test_start_etos(self, graphql_execute_mock, download_suite_mock, digest_mock):
        """Test that POST requests to /etos attempts to start ETOS tests.

        Approval criteria:
            - POST requests to ETOS shall return 200.
            - POST requests to ETOS shall attempt to send TERCC.
            - POST requests to ETOS shall configure environment provider.

        Test steps::
            1. Send a POST request to etos.
            2. Verify that the status code is 200.
            3. Verify that a TERCC was sent.
            4. Verify that the environment provider was configured.
        """
        digest_mock.return_value = (
            "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
        )
        fake_database = FakeDatabase()
        Config().set("database", fake_database)
        fake_database.put("/environment/provider/log-area/default", json.dumps(LOG_AREA_PROVIDER))
        fake_database.put("/environment/provider/iut/default", json.dumps(IUT_PROVIDER))
        fake_database.put(
            "/environment/provider/execution-space/default", json.dumps(EXECUTION_SPACE_PROVIDER)
        )

        graphql_execute_mock.return_value = {
            "artifactCreated": {
                "edges": [
                    {
                        "node": {
                            "meta": {"id": "cda58701-5614-49bf-9101-11b71a5721fb"},
                            "data": {"identity": "pkg:eiffel-community/etos-api"},
                        }
                    }
                ]
            }
        }
        download_suite_mock.return_value = [
            {
                "name": "TestRouters",
                "priority": 1,
                "recipes": [
                    {
                        "constraints": [
                            {"key": "ENVIRONMENT", "value": {}},
                            {"key": "PARAMETERS", "value": {}},
                            {"key": "COMMAND", "value": "exit 0"},
                            {"key": "TEST_RUNNER", "value": "TestRunner"},
                            {"key": "EXECUTE", "value": []},
                            {"key": "CHECKOUT", "value": ["echo 'checkout'"]},
                        ],
                        "id": "132a7499-7ad4-4c4a-8a66-4e9ac95c7885",
                        "testCase": {
                            "id": "test_start_etos",
                            "tracker": "Github",
                            "url": "https://github.com/eiffel-community/etos-api",
                        },
                    }
                ],
            }
        ]
        self.logger.info("STEP: Send a POST request to etos.")
        response = self.client.post(
            "/api/etos",
            json={
                "artifact_identity": "pkg:testing/etos",
                "test_suite_url": "http://localhost/my_test.json",
            },
        )
        self.logger.info("STEP: Verify that the status code is 200.")
        assert response.status_code == 200
        self.logger.info("STEP: Verify that a TERCC was sent.")
        debug = Debug()
        tercc = None
        for event in debug.events_published:
            if event.meta.type == "EiffelTestExecutionRecipeCollectionCreatedEvent":
                tercc = event
                break
        assert tercc is not None
        assert response.json().get("tercc") == tercc.meta.event_id
        self.logger.info("STEP: Verify that the environment provider was configured.")

        log_area = json.loads(
            fake_database.get(f"/testrun/{tercc.meta.event_id}/provider/log-area")[0]
        )
        iut = json.loads(fake_database.get(f"/testrun/{tercc.meta.event_id}/provider/iut")[0])
        execution_space = json.loads(
            fake_database.get(f"/testrun/{tercc.meta.event_id}/provider/execution-space")[0]
        )
        self.assertDictEqual(log_area, LOG_AREA_PROVIDER)
        self.assertDictEqual(iut, IUT_PROVIDER)
        self.assertDictEqual(execution_space, EXECUTION_SPACE_PROVIDER)

    @patch("etos_api.library.validator.Docker.digest")
    @patch("etos_api.routers.v0.utilities.download_suite")
    def test_start_etos_empty_suite(self, download_suite_mock, digest_mock):
        """Test that POST requests to /etos with an empty suite definition list fails validation
        and does not start ETOS tests.

        Approval criteria:
            - POST requests to ETOS with an empty suite definition list shall return 400.
            - POST requests to ETOS with an empty suite definition list shall not trigger
              sending a TERCC.

        Test steps::
            1. Send a POST request to etos with an empty suite definition list.
            2. Verify that the status code is 400.
            3. Verify that a TERCC was not sent.
        """
        digest_mock.return_value = (
            "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
        )

        download_suite_mock.return_value = []
        self.logger.info("STEP: Send a POST request to etos.")
        response = self.client.post(
            "/api/etos",
            json={
                "artifact_identity": "pkg:testing/etos",
                "test_suite_url": "http://localhost/my_test.json",
            },
        )
        self.logger.info("STEP: Verify that the status code is 400.")
        assert response.status_code == 400

        self.logger.info("STEP: Verify that a TERCC was not sent.")
        debug = Debug()
        tercc = None
        for event in debug.events_published:
            if event.meta.type == "EiffelTestExecutionRecipeCollectionCreatedEvent":
                tercc = event
                break
        assert tercc is None

    def test_start_etos_missing_artifact_identity_and_id(self):
        """Test that POST requests to /etos with missing artifact identity and ID fail validation.

        Approval criteria:
            - POST requests to ETOS with missing artifact_identity and artifact_id shall return 422.
            - The error message shall indicate that at least one is required.

        Test steps::
            1. Send a POST request to etos without artifact_identity or artifact_id.
            2. Verify that the status code is 422.
            3. Verify that the error message indicates missing required field.
        """
        self.logger.info("STEP: Send a POST request to etos without artifact_identity or artifact_id.")
        response = self.client.post(
            "/api/etos",
            json={
                "test_suite_url": "http://localhost/my_test.json",
                "artifact_id": None,  # Explicitly set to None to trigger validation
            },
        )
        self.logger.info("STEP: Verify that the status code is 422.")
        assert response.status_code == 422

        self.logger.info("STEP: Verify that the error message indicates missing required field.")
        error_detail = response.json()
        assert "detail" in error_detail
        error_messages = [error["msg"] for error in error_detail["detail"]]
        expected_message = "At least one of 'artifact_identity' or 'artifact_id' is required."
        assert any(expected_message in msg for msg in error_messages)

    def test_start_etos_empty_artifact_identity_and_none_artifact_id(self):
        """Test that POST requests to /etos with empty artifact_identity fail during processing.

        Approval criteria:
            - POST requests to ETOS with empty artifact_identity shall return 400.
            - The error should occur during suite validation or artifact processing.

        Test steps::
            1. Send a POST request to etos with empty artifact_identity and None artifact_id.
            2. Verify that the status code is 400.
            3. Verify that the request fails (empty identity is treated as provided but invalid).
        """
        self.logger.info("STEP: Send a POST request to etos with empty artifact_identity and None artifact_id.")
        response = self.client.post(
            "/api/etos",
            json={
                "artifact_identity": "",
                "artifact_id": None,
                "test_suite_url": "http://localhost/my_test.json",
            },
        )
        self.logger.info("STEP: Verify that the status code is 400.")
        assert response.status_code == 400

        self.logger.info("STEP: Verify that the request fails during processing.")
        error_detail = response.json()
        assert "detail" in error_detail
        # Empty string is considered "provided" by validation, so it fails later in processing

    def test_start_etos_both_artifact_identity_and_id_provided(self):
        """Test that POST requests to /etos with both artifact_identity and artifact_id fail validation.

        Approval criteria:
            - POST requests to ETOS with both artifact_identity and artifact_id shall return 422.
            - The error message shall indicate that only one is required.

        Test steps::
            1. Send a POST request to etos with both artifact_identity and artifact_id.
            2. Verify that the status code is 422.
            3. Verify that the error message indicates only one is required.
        """
        self.logger.info("STEP: Send a POST request to etos with both artifact_identity and artifact_id.")
        response = self.client.post(
            "/api/etos",
            json={
                "artifact_identity": "pkg:testing/etos",
                "artifact_id": "123e4567-e89b-12d3-a456-426614174000",
                "test_suite_url": "http://localhost/my_test.json",
            },
        )
        self.logger.info("STEP: Verify that the status code is 422.")
        assert response.status_code == 422

        self.logger.info("STEP: Verify that the error message indicates only one is required.")
        error_detail = response.json()
        assert "detail" in error_detail
        error_messages = [error["msg"] for error in error_detail["detail"]]
        expected_message = "Only one of 'artifact_identity' or 'artifact_id' is required."
        assert any(expected_message in msg for msg in error_messages)

    def test_selftest_get_ping(self):
        """Test that selftest ping with HTTP GET pings the system.

        Approval criteria:
            - GET requests to selftest ping shall return status code 204.

        Test steps::
            1. Send a GET request to selftest ping.
            2. Verify that status code is 204.
        """
        self.logger.info("STEP: Send a GET request to selftest ping.")
        response = self.client.get("/api/selftest/ping")
        self.logger.info("STEP: Verify that the status code is 204.")
        assert response.status_code == 204

    def test_convert_to_rfc1123(self):
        """Test that the testrun router can convert a string to an RFC-1123 accepted string.

        Approval criteria:
            - A string passed to the convert_to_rfc1123 method shall be returned as valid rfc-1123.

        Test steps::
            1. For a set of invalid strings.
                1.1. Pass the string to the conversion method.
                1.2. Verify that the string returned is valid rfc-1123.
        """
        # invalid strings contains an invalid string coupled with what it is
        # expected to convert to.
        invalid_strings = (
            ("Hello World!", "hello-world"),
            ("123_ABC", "123-abc"),
            ("No-Change", "no-change"),
            ("Special@#%&*()Characters", "special-characters"),
            ("Multiple     Spaces", "multiple-spaces"),
            ("EndWithSpecialCharacter!", "endwithspecialcharacter"),
            ("@StartWithSpecialCharacter", "startwithspecialcharacter"),
            ("Mixed$#Case123", "mixed-case123"),
            ("singleword", "singleword"),
            ("1234567890", "1234567890"),
        )
        self.logger.info("STEP: For a set of invalid strings.")
        for invalid, expected in invalid_strings:
            self.logger.info("STEP: Pass the string to the conversion method.")
            value = convert_to_rfc1123(invalid)
            self.logger.info("STEP: Verify that th string returned is valid rfc-1123.")
            assert value == expected
