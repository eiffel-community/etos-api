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
"""Simple user/pass authentication backend."""
import bcrypt
from fastapi import status, HTTPException
from fastapi.security import HTTPBasicCredentials
from etos_api.library.database import Database
from etos_api.authentication.schemas import User


class Simple:
    """Authentication backend using simple user/pass model."""

    def __init__(self, config):
        """Initialize simple login.

        :param config: Configuration for simple login.
        :type config: :obj:`box.Box`
        """
        self.config = config

    async def get_token(self, credentials: HTTPBasicCredentials):
        """Login endpoint for simple authorization.

        :param credentials: Credentials for simple authorization.
        :type credentials: :obj:`fastapi.security.HTTPBasicCredentials`
        :return: JWT access token for user.
        :rtype: str
        """
        authorized, user = await self.authorize(credentials)
        if not authorized:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="Incorrect username or password",
                headers={"WWW-Authenticate": "Bearer"},
            )
        return await self.config.get_token(user)

    async def authorize(self, credentials):
        """Authorize user based with credentials.

        :param credentials: Credentials for simple authorization.
        :type credentials: :obj:`fastapi.security.HTTPBasicCredentials`
        :return: Whether or not user is authorized and a user object.
        :rtype: tuple
        """
        async with Database(self.config) as database:
            user = await database.get(credentials.username)
            if user and bcrypt.checkpw(
                credentials.password.encode("UTF-8"), user["hashed_password"]
            ):
                return True, User(**user)
        return False, None
