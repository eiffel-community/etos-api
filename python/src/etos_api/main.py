# Copyright 2020-2021 Axis Communications AB.
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
"""ETOS API."""
import logging
from fastapi import FastAPI
from starlette.responses import RedirectResponse
from etos_api.routers.v0 import ETOSv0
from etos_api.routers.v1alpha import ETOSv1Alpha

LOGGER = logging.getLogger(__name__)
DEFAULT_VERSION = ETOSv0

APP = FastAPI()
APP.mount("/api/v1alpha", ETOSv1Alpha, "ETOS V1 Alpha")
APP.mount("/api/v0", ETOSv0, "ETOS V0")
APP.mount("/api", DEFAULT_VERSION, "ETOS V0")


APP.get("/api/selftest/ping")
async def ping():
    """Ping the ETOS service in order to check if it is up and running.

    This is deprecated in favor of `/api/etos/ping`.

    :return: HTTP 204 response.
    :rtype: :obj:`starlette.responses.Response`
    """
    LOGGER.warning("DEPRECATED request to selftest/ping received!")
    return RedirectResponse("/api/etos/ping")
