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
"""Authentication and user schemas."""
from typing import Optional
from pydantic import BaseModel

# A bug in pylint https://github.com/PyCQA/pylint/issues/3882
# pylint:disable=unsubscriptable-object


class Token(BaseModel):
    """Token model for defining token response."""

    access_token: str
    token_type: str


class TokenConfig(BaseModel):
    """Token config. Used to generate JWT."""

    iss: str
    aud: list[str]
    sub: str


class User(BaseModel):
    """User model for describing a user in the system."""

    username: str
    email: Optional[str] = None
    full_name: Optional[str] = None
    disabled: Optional[bool] = None


class DatabaseUser(User):
    """User model for describing a user in the database."""

    hashed_password: bytes
