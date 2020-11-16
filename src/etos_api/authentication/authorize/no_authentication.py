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
"""No authentication authentication backend."""
from etos_api.authentication.schemas import User
from etos_api.library.database import Database


class NoAuthentication:  # pylint:disable=too-few-public-methods
    """Backend for disabling authentication."""

    def __init__(self, config):
        """Initialize no login.

        :param config: Configuration for no login.
        :type config: :obj:`box.Box`
        """
        self.config = config

    async def get_token(self):
        """Return token for anonymous.

        :return: JWT access token for user.
        :rtype: str
        """
        user = User(username="Anonymous")
        token = await self.config.get_token(user)
        async with Database(self.config) as database:
            await database.set("Anonymous", user.dict())
        return token
