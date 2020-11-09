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
"""Schemas for the ETOS endpoint."""
import os
from uuid import UUID
from typing import Optional
from pydantic import BaseModel

# There's a bug with pylint detecting subscription on Optional objects as problematic.
# https://github.com/PyCQA/pylint/issues/3882
# pylint: disable=unsubscriptable-object


class StartEtosRequest(BaseModel):
    """Request model for the ETOS start API."""

    artifact_identity: str
    test_suite_url: str
    dataset: Optional[dict] = {}
    execution_space_provider: Optional[str] = os.getenv(
        "DEFAULT_EXECUTION_SPACE_PROVIDER", "default"
    )
    iut_provider: Optional[str] = os.getenv("DEFAULT_IUT_PROVIDER", "default")
    log_area_provider: Optional[str] = os.getenv("DEFAULT_LOG_AREA_PROVIDER", "default")


class StartEtosResponse(BaseModel):
    """Response model for the ETOS start API."""

    event_repository: str
    tercc: UUID
