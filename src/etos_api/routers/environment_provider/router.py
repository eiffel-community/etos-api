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
"""Environment provider proxy API."""
import asyncio
import os
import time
import aiohttp
from fastapi import APIRouter, HTTPException
from etos_lib import ETOS

from .schemas import ConfigureEnvironmentProviderRequest

ROUTER = APIRouter()


@ROUTER.post(
    "/environment_provider/configure", tags=["environment_provider"], status_code=204
)
async def configure_environment_provider(
    environment: ConfigureEnvironmentProviderRequest,
):
    """Configure environment provider request.

    :param environment: Environment to configure.
    :type environment: :obj:`etos_api.routers.etos.schemas.ConfigureEnvironmentProviderRequest`
    """
    etos_library = ETOS("ETOS API", os.getenv("HOSTNAME"), "ETOS API")

    end_time = time.time() + etos_library.debug.default_http_timeout
    async with aiohttp.ClientSession() as session:
        while time.time() < end_time:
            try:
                async with session.post(
                    f"{etos_library.debug.environment_provider}/configure",
                    json=environment.dict(),
                    headers={
                        "Content-Type": "application/json",
                        "Accept": "application/json",
                    },
                ) as response:
                    assert 200 <= response.status < 400
                break
            except AssertionError:
                await asyncio.sleep(2)
        else:
            raise HTTPException(
                status_code=400,
                detail=f"Unable to configure environment provider with '{environment.json()}'",
            )
