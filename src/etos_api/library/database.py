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
"""ETOS API database handler."""


class Database:
    """ETOS API database."""

    database = {}

    def __init__(self, config):
        """Initialize database config.

        :param config: Configuration parameters for the database connection.
        :type config: dict
        """
        self.connected = False
        self.config = config

    async def __aenter__(self):
        """Enter context. Connect to database."""
        if not self.connected:
            await self.connect()
        return self

    async def __aexit__(self, *args, **kwargs):
        """Exit context. Disconnect from database."""
        if self.connected:
            await self.disconnect()

    async def connect(self):
        """Connect to database."""

    async def disconnect(self):
        """Disconnect from database."""

    async def get(self, key):
        """Get value of key from database.

        :param key: Key to get value for.
        :type key: str
        """
        return self.database.get(key)

    async def set(self, key, value):
        """Set a value for key in database.

        :param key: Key to set value for.
        :type key: str
        :param value: Value to set for key.
        :type value: str
        """
        self.database[key] = value
