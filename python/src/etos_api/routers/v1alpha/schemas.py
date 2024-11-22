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
"""Schemas for the testrun endpoint."""
import os
from typing import Optional, Union
from uuid import UUID

# Pylint refrains from linting C extensions due to arbitrary code execution.
from pydantic import BaseModel, Field, field_validator  # pylint:disable=no-name-in-module

# pylint: disable=too-few-public-methods
# pylint: disable=no-self-argument


class TestrunRequest(BaseModel):
    """Base class for testrun request models."""


class TestrunResponse(BaseModel):
    """Base class for testrun response models."""


class StartTestrunRequest(TestrunRequest):
    """Request model for the start endpoint of the ETOS testrun API."""

    artifact_identity: Optional[str]
    artifact_id: Optional[UUID] = Field(default=None, validate_default=True)
    test_suite_url: str
    dataset: Union[dict, list[dict]] = {}
    execution_space_provider: Optional[str] = os.getenv(
        "DEFAULT_EXECUTION_SPACE_PROVIDER", "default"
    )
    iut_provider: Optional[str] = os.getenv("DEFAULT_IUT_PROVIDER", "default")
    log_area_provider: Optional[str] = os.getenv("DEFAULT_LOG_AREA_PROVIDER", "default")

    @field_validator("artifact_id")
    def validate_id_or_identity(cls, artifact_id, info):
        """Validate that at least one and only one of id and identity are set.

        :param artifact_id: The value of 'artifact_id' to validate.
        :value artifact_id: str or None
        :param info: The information about the model.
        :type info: FieldValidationInfo
        :return: The value of artifact_id.
        :rtype: str or None
        """
        values = info.data
        if values.get("artifact_identity") is None and not artifact_id:
            raise ValueError("At least one of 'artifact_identity' or 'artifact_id' is required.")
        if values.get("artifact_identity") is not None and artifact_id:
            raise ValueError("Only one of 'artifact_identity' or 'artifact_id' is required.")
        return artifact_id


class StartTestrunResponse(TestrunResponse):
    """Response model for the start endpoint of the ETOS testrun API."""

    event_repository: str
    tercc: UUID
    artifact_id: UUID
    artifact_identity: str


class AbortTestrunResponse(TestrunResponse):
    """Response model for the abort endpoint of the ETOS testrun API."""

    message: str
