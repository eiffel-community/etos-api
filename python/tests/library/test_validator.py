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
"""Tests for the validator library."""
import logging
import sys
from unittest.mock import patch

import pytest

from etos_api.library.validator import (
    ArtifactValidator,
    SuiteValidator,
    ValidationError,
)

logging.basicConfig(level=logging.DEBUG, stream=sys.stdout)


class TestValidator:
    """Test the validator library."""

    logger = logging.getLogger(__name__)
    # Mark all test methods as asyncio methods to tell pytest to 'await' them.
    pytestmark = pytest.mark.asyncio

    @patch("etos_api.library.validator.Docker.digest")
    async def test_validate_proper_suite(self, digest_mock):
        """Test that the validator validates a proper suite correctly.

        Approval criteria:
            - Suite validator shall approve a proper suite.

        Test steps::
            1. Validate a proper suite.
            2. Verify that no exceptions were raised.
        """
        digest_mock.return_value = (
            "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
        )
        test_suite = [
            {
                "name": "TestValidator",
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
                        "id": "131a7499-7ad4-4c4a-8a66-4e9ac95c7885",
                        "testCase": {
                            "id": "test_validate_proper_suite",
                            "tracker": "Github",
                            "url": "https://github.com/eiffel-community/etos-api",
                        },
                    }
                ],
            }
        ]
        self.logger.info("STEP: Validate a proper suite.")
        validator = SuiteValidator()
        try:
            await validator.validate(test_suite)
            exception = False
        except (AssertionError, ValidationError):
            exception = True
        self.logger.info("STEP: Verify that no exceptions were raised.")
        assert exception is False

    async def test_validate_missing_constraints(self):
        """Test that the validator fails when missing required constraints.

        Approval criteria:
            - Suite validator shall not approve a suite with missing constraints.

        Test steps::
            1. Validate a suite with a missing constraint.
            2. Verify that the validator raises ValidationError.
        """
        test_suite = [
            {
                "name": "TestValidator",
                "priority": 1,
                "recipes": [
                    {
                        "constraints": [
                            {"key": "ENVIRONMENT", "value": {}},
                            {"key": "PARAMETERS", "value": {}},
                            {"key": "COMMAND", "value": "exit 0"},
                            {"key": "EXECUTE", "value": []},
                            {"key": "CHECKOUT", "value": ["echo 'checkout'"]},
                        ],
                        "id": "131a7499-7ad4-4c4a-8a66-4e9ac95c7887",
                        "testCase": {
                            "id": "test_validate_missing_constraints",
                            "tracker": "Github",
                            "url": "https://github.com/eiffel-community/etos-api",
                        },
                    }
                ],
            }
        ]  # TEST_RUNNER is missing
        self.logger.info("STEP: Validate a suite with a missing constraint.")
        validator = SuiteValidator()
        try:
            await validator.validate(test_suite)
            exception = False
        except ValidationError:
            exception = True
        self.logger.info("STEP: Verify that the validator raises ValidationError.")
        assert exception is True

    async def test_validate_wrong_types(self):
        """Test that the validator fails when constraints have the wrong types.

        Approval criteria:
            - Suite validator shall not approve a suite wrong constraint types.

        Test steps::
            1. For each constraint.
                1. Validate constraint with wrong type.
                2. Verify that the validator raises ValidationError.
        """
        base_suite = {
            "name": "TestValidator",
            "priority": 1,
            "recipes": [
                {
                    "constraints": [],  # Filled in loop
                    "id": "131a7499-7ad4-4c4a-8a66-4e9ac95c7886",
                    "testCase": {
                        "id": "test_validate_wrong_types",
                        "tracker": "Github",
                        "url": "https://github.com/eiffel-community/etos-api",
                    },
                }
            ],
        }
        constraints = [
            [
                {"key": "ENVIRONMENT", "value": "Wrong"},  # Wrong
                {"key": "PARAMETERS", "value": {}},
                {"key": "COMMAND", "value": "exit 0"},
                {"key": "TEST_RUNNER", "value": "TestRunner"},
                {"key": "EXECUTE", "value": []},
                {"key": "CHECKOUT", "value": ["echo 'checkout'"]},
            ],
            [
                {"key": "ENVIRONMENT", "value": {}},
                {"key": "PARAMETERS", "value": "Wrong"},  # Wrong
                {"key": "COMMAND", "value": "exit 0"},
                {"key": "TEST_RUNNER", "value": "TestRunner"},
                {"key": "EXECUTE", "value": []},
                {"key": "CHECKOUT", "value": ["echo 'checkout'"]},
            ],
            [
                {"key": "ENVIRONMENT", "value": {}},
                {"key": "PARAMETERS", "value": {}},
                {"key": "COMMAND", "value": {"wrong": True}},  # Wrong
                {"key": "TEST_RUNNER", "value": "TestRunner"},
                {"key": "EXECUTE", "value": []},
                {"key": "CHECKOUT", "value": ["echo 'checkout'"]},
            ],
            [
                {"key": "ENVIRONMENT", "value": {}},
                {"key": "PARAMETERS", "value": {}},
                {"key": "COMMAND", "value": "exit 0"},
                {"key": "TEST_RUNNER", "value": {"wrong": True}},  # Wrong
                {"key": "EXECUTE", "value": []},
                {"key": "CHECKOUT", "value": ["echo 'checkout'"]},
            ],
            [
                {"key": "ENVIRONMENT", "value": {}},
                {"key": "PARAMETERS", "value": {}},
                {"key": "COMMAND", "value": "exit 0"},
                {"key": "TEST_RUNNER", "value": "TestRunner"},
                {"key": "EXECUTE", "value": "Wrong"},  # Wrong
                {"key": "CHECKOUT", "value": ["echo 'checkout'"]},
            ],
            [
                {"key": "ENVIRONMENT", "value": {}},
                {"key": "PARAMETERS", "value": {}},
                {"key": "COMMAND", "value": "exit 0"},
                {"key": "TEST_RUNNER", "value": "TestRunner"},
                {"key": "EXECUTE", "value": []},
                {"key": "CHECKOUT", "value": "Wrong"},  # Wrong
            ],
        ]
        self.logger.info("STEP: For each constraint.")
        validator = SuiteValidator()
        for constraint in constraints:
            self.logger.info("STEP: Validate constraint with wrong type.")
            base_suite["recipes"][0]["constraints"] = constraint
            self.logger.info("STEP: Verify that the validator raises ValidationError.")
            with pytest.raises(ValidationError):
                await validator.validate([base_suite])

    async def test_validate_too_many_constraints(self):
        """Test that the validator fails when a constraint is defined multiple times.

        Approval criteria:
            - Suite validator shall not approve a suite with too many constraints.

        Test steps::
            1. Validate a suite with a constraint defined multiple times.
            2. Verify that the validator raises ValidationError.
        """
        test_suite = [
            {
                "name": "TestValidator",
                "priority": 1,
                "recipes": [
                    {
                        "constraints": [
                            {"key": "ENVIRONMENT", "value": {}},
                            {"key": "PARAMETERS", "value": {}},
                            {"key": "TEST_RUNNER", "value": "TestRunner"},
                            {"key": "TEST_RUNNER", "value": "AnotherTestRunner"},
                            {"key": "COMMAND", "value": "exit 0"},
                            {"key": "EXECUTE", "value": []},
                            {"key": "CHECKOUT", "value": ["echo 'checkout'"]},
                        ],
                        "id": "131a7499-7ad4-4c4a-8a66-4e9ac95c7887",
                        "testCase": {
                            "id": "test_validate_too_many_constraints",
                            "tracker": "Github",
                            "url": "https://github.com/eiffel-community/etos-api",
                        },
                    }
                ],
            }
        ]
        self.logger.info("STEP: Validate a suite with a constraint defined multiple times.")
        validator = SuiteValidator()
        try:
            await validator.validate(test_suite)
            exception = False
        except ValidationError:
            exception = True
        self.logger.info("STEP: Verify that the validator raises ValidationError.")
        assert exception is True

    async def test_validate_unknown_constraint(self):
        """Test that the validator fails when an unknown constraint is defined.

        Approval criteria:
            - Suite validator shall not approve a suite with an unknown constraint.

        Test steps::
            1. Validate a suite with an unknown constraint.
            2. Verify that the validator raises ValidationError.
        """
        test_suite = [
            {
                "name": "TestValidator",
                "priority": 1,
                "recipes": [
                    {
                        "constraints": [
                            {"key": "ENVIRONMENT", "value": {}},
                            {"key": "PARAMETERS", "value": {}},
                            {"key": "TEST_RUNNER", "value": "TestRunner"},
                            {"key": "COMMAND", "value": "exit 0"},
                            {"key": "EXECUTE", "value": []},
                            {"key": "CHECKOUT", "value": ["echo 'checkout'"]},
                            {"key": "UNKNOWN", "value": "Hello"},
                        ],
                        "id": "131a7499-7ad4-4c4a-8a66-4e9ac95c7887",
                        "testCase": {
                            "id": "test_validate_unknown_constraint",
                            "tracker": "Github",
                            "url": "https://github.com/eiffel-community/etos-api",
                        },
                    }
                ],
            }
        ]
        self.logger.info("STEP: Validate a suite with an unknown constraint.")
        validator = SuiteValidator()
        try:
            await validator.validate(test_suite)
            exception = False
        except TypeError:
            exception = True
        self.logger.info("STEP: Verify that the validator raises ValidationError.")
        assert exception is True

    async def test_validate_empty_constraints(self):
        """Test that required constraints are not empty.

        Approval criteria:
            - Constraints 'TEST_RUNNER', 'CHECKOUT' & 'COMMAND' shall not be empty.

        Test steps::
            1. For each required key.
                1. Validate a suite without the required key.
        """
        base_suite = {
            "name": "TestValidator",
            "priority": 1,
            "recipes": [
                {
                    "constraints": [],  # Filled below
                    "id": "131a7499-7ad4-4c4a-8a66-4e9ac95c7892",
                    "testCase": {
                        "id": "test_validate_empty_constraints",
                        "tracker": "Github",
                        "url": "https://github.com/eiffel-community/etos-api",
                    },
                }
            ],
        }
        constraints = [
            [
                {"key": "ENVIRONMENT", "value": {}},
                {"key": "PARAMETERS", "value": {}},
                {"key": "COMMAND", "value": ""},  # Empty
                {"key": "TEST_RUNNER", "value": "TestRunner"},
                {"key": "EXECUTE", "value": []},
                {"key": "CHECKOUT", "value": ["echo 'checkout'"]},
            ],
            [
                {"key": "ENVIRONMENT", "value": {}},
                {"key": "PARAMETERS", "value": {}},
                {"key": "COMMAND", "value": "exit 0"},
                {"key": "TEST_RUNNER", "value": ""},  # Empty
                {"key": "EXECUTE", "value": []},
                {"key": "CHECKOUT", "value": ["echo 'checkout'"]},
            ],
            [
                {"key": "ENVIRONMENT", "value": {}},
                {"key": "PARAMETERS", "value": {}},
                {"key": "COMMAND", "value": "exit 0"},
                {"key": "TEST_RUNNER", "value": "TestRunner"},
                {"key": "EXECUTE", "value": []},
                {"key": "CHECKOUT", "value": []},  # Empty
            ],
        ]
        self.logger.info("STEP: For each required key.")
        validator = SuiteValidator()
        for constraint in constraints:
            base_suite["recipes"][0]["constraints"] = constraint
            self.logger.info("STEP: Validate a suite without the required key.")
            with pytest.raises(ValidationError):
                await validator.validate([base_suite])


class TestArtifactValidator:
    """Test the artifact validation functions."""

    def setup_method(self):
        """Set up test fixtures."""
        self.validator = ArtifactValidator()

    def test_validate_purl_valid(self):
        """Test that valid PURL strings are accepted."""
        valid_purls = [
            "pkg:npm/lodash@4.17.21",
            "pkg:pypi/requests@2.25.1",
            "pkg:maven/org.apache.commons/commons-lang3@3.12.0",
            "pkg:golang/github.com/gorilla/mux@v1.8.0",
            "pkg:docker/nginx@latest",
            "pkg:generic/openssl@1.1.1k",
        ]

        for purl in valid_purls:
            assert self.validator.validate_purl(purl) is True

    def test_validate_purl_invalid(self):
        """Test that invalid PURL strings are rejected."""
        invalid_purls = [
            "",
            None,
            "not-a-purl",
            "http://example.com",
            "pkg:",  # Missing parts
            "pkg:npm/",  # Incomplete
            "npm/lodash@4.17.21",  # Missing pkg: prefix
            "PKG:npm/lodash@4.17.21",  # Wrong case
        ]

        for purl in invalid_purls:
            assert self.validator.validate_purl(purl) is False

    def test_validate_uuid_valid(self):
        """Test that valid UUID strings are accepted."""
        valid_uuids = [
            "550e8400-e29b-41d4-a716-446655440000",
            "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
            "6ba7b811-9dad-11d1-80b4-00c04fd430c8",
            "6ba7b812-9dad-11d1-80b4-00c04fd430c8",
            "6ba7b814-9dad-11d1-80b4-00c04fd430c8",
            "f47ac10b-58cc-4372-a567-0e02b2c3d479",
        ]

        for uuid_str in valid_uuids:
            assert self.validator.validate_uuid(uuid_str) is True

    def test_validate_uuid_invalid(self):
        """Test that invalid UUID strings are rejected."""
        invalid_uuids = [
            "",
            None,
            "not-a-uuid",
            "550e8400-e29b-41d4-a716",  # Too short
            "550e8400-e29b-41d4-a716-446655440000-extra",  # Too long
            "550e8400-e29b-41d4-a716-44665544000g",  # Invalid character (g)
        ]

        for uuid_str in invalid_uuids:
            assert self.validator.validate_uuid(uuid_str) is False

    def test_validate_artifact_identity_or_id_valid_purl(self):
        """Test validation with valid PURL artifact_identity."""
        # Should not raise any exception
        self.validator.validate_artifact_identity_or_id(
            artifact_identity="pkg:npm/lodash@4.17.21", artifact_id=None
        )

    def test_validate_artifact_identity_or_id_valid_uuid(self):
        """Test validation with valid UUID artifact_id."""
        # Should not raise any exception
        self.validator.validate_artifact_identity_or_id(
            artifact_identity=None, artifact_id="550e8400-e29b-41d4-a716-446655440000"
        )

    def test_validate_artifact_identity_or_id_invalid_purl(self):
        """Test validation with invalid PURL artifact_identity raises ValueError."""
        with pytest.raises(ValueError) as exc_info:
            self.validator.validate_artifact_identity_or_id(
                artifact_identity="not-a-purl", artifact_id=None
            )
        assert "Invalid artifact_identity" in str(exc_info.value)
        assert "is not a valid PURL" in str(exc_info.value)

    def test_validate_artifact_identity_or_id_invalid_uuid(self):
        """Test validation with invalid UUID artifact_id raises ValueError."""
        with pytest.raises(ValueError) as exc_info:
            self.validator.validate_artifact_identity_or_id(
                artifact_identity=None, artifact_id="not-a-uuid"
            )
        assert "Invalid artifact_id" in str(exc_info.value)
        assert "is not a valid UUID" in str(exc_info.value)

    def test_validate_artifact_identity_or_id_both_provided(self):
        """Test validation with both valid values provided."""
        # Should not raise any exception when both are valid
        self.validator.validate_artifact_identity_or_id(
            artifact_identity="pkg:npm/lodash@4.17.21",
            artifact_id="550e8400-e29b-41d4-a716-446655440000",
        )

    def test_validate_artifact_identity_or_id_neither_provided(self):
        """Test validation with neither value provided."""
        # Should not raise any exception - no validation is performed when both are None
        self.validator.validate_artifact_identity_or_id(artifact_identity=None, artifact_id=None)

    # Tests for ArtifactValidator class methods (should raise ValueError)
    def test_artifact_validator_validate_purl_valid(self):
        """Test ArtifactValidator.validate_purl with valid PURL strings."""
        valid_purls = [
            "pkg:npm/lodash@4.17.21",
            "pkg:pypi/requests@2.25.1",
            "pkg:maven/org.apache.commons/commons-lang3@3.12.0",
        ]

        for purl in valid_purls:
            assert self.validator.validate_purl(purl) is True

    def test_artifact_validator_validate_uuid_valid(self):
        """Test ArtifactValidator.validate_uuid with valid UUID strings."""
        valid_uuids = [
            "550e8400-e29b-41d4-a716-446655440000",
            "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
        ]

        for uuid_str in valid_uuids:
            assert self.validator.validate_uuid(uuid_str) is True

    def test_artifact_validator_validate_artifact_identity_or_id_valid(self):
        """Test ArtifactValidator.validate_artifact_identity_or_id with valid inputs."""
        # Should not raise any exception
        self.validator.validate_artifact_identity_or_id(
            artifact_identity="pkg:npm/lodash@4.17.21", artifact_id=None
        )

        self.validator.validate_artifact_identity_or_id(
            artifact_identity=None, artifact_id="550e8400-e29b-41d4-a716-446655440000"
        )

    def test_artifact_validator_validate_identity_or_id_invalid_purl(self):
        """Test ArtifactValidator.validate_artifact_identity_or_id with invalid PURL.

        Should raise ValueError.
        """
        with pytest.raises(ValueError) as exc_info:
            self.validator.validate_artifact_identity_or_id(
                artifact_identity="not-a-purl", artifact_id=None
            )
        assert "Invalid artifact_identity" in str(exc_info.value)
        assert "is not a valid PURL" in str(exc_info.value)

    def test_artifact_validator_validate_identity_or_id_invalid_uuid(self):
        """Test ArtifactValidator.validate_artifact_identity_or_id with invalid UUID.

        Should raise ValueError.
        """
        with pytest.raises(ValueError) as exc_info:
            self.validator.validate_artifact_identity_or_id(
                artifact_identity=None, artifact_id="not-a-uuid"
            )
        assert "Invalid artifact_id" in str(exc_info.value)
        assert "is not a valid UUID" in str(exc_info.value)
