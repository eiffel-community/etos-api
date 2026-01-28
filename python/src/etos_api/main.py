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

from fastapi import FastAPI

from etos_api.routers.v0 import ETOSV0
from etos_api.routers.v1alpha import ETOSV1ALPHA

DEFAULT_VERSION = ETOSV0

APP = FastAPI()
APP.mount("/api/v1alpha", ETOSV1ALPHA, "ETOS V1 Alpha")
APP.mount("/api/v0", ETOSV0, "ETOS V0")
APP.mount("/api", DEFAULT_VERSION, "ETOS V0")
