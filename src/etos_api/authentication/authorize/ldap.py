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
"""Lightweight directory access protocol backend."""
from typing import Optional
from fastapi import status, HTTPException
from fastapi.security import HTTPBasicCredentials
from pydantic import BaseModel
from ldap3 import Connection, Server, ASYNC, ALL_ATTRIBUTES, SUBTREE, AUTO_BIND_NONE
from etos_api.authentication.schemas import User
from etos_api.library.utilities import sync_to_async
from etos_api.library.database import Database

# A bug in pylint https://github.com/PyCQA/pylint/issues/3882
# pylint:disable=unsubscriptable-object
class LdapServer(BaseModel):
    """Ldap server model."""

    host: str
    port: Optional[int]


class LdapConnection(BaseModel):
    """Ldap connection model."""

    user: str
    password: str
    read_only: bool = True
    client_strategy: str = ASYNC
    raise_exceptions: bool = False
    auto_bind: str = AUTO_BIND_NONE


class UserFields(BaseModel):
    """Model for userfields to get from ldap response."""

    username: str
    email: str
    full_name: str


class Search(BaseModel):
    """Search parameter models for ldap."""

    search_base: str
    extra_search_filters: dict
    object_class: str
    user_fields: UserFields
    active_directory_domain_host: Optional[str]
    active_directory_domain_name: Optional[str]


class Ldap:
    """Authentication backend for ldap."""

    def __init__(self, config):
        """Initialize ldap login.

        :param config: Configuration for ldap login.
        :type config: :obj:`box.Box`
        """
        self.config = config
        self.ldap_config = self.config.authorization_backend.config
        self.search = Search(**self.ldap_config.search)
        self.server_data = LdapServer(**self.ldap_config.server)

        self.server = Server(**self.server_data.dict())

    async def get_token(self, credentials: HTTPBasicCredentials):
        """Login endpoint for simple authorization.

        :param credentials: Credentials for ldap authorization.
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

    async def format_username(self, username):
        """Format username according to domain.

        :param username: Username to format.
        :type username: str
        :return: Formatted username.
        :rtype: str
        """
        if self.search.active_directory_domain_host:
            username = f"{username}@{self.search.active_directory_domain_host}"
        elif self.search.active_directory_domain_name:
            username = f"{self.search.active_directory_domain_name}\\{username}"
        return username

    async def authorize(self, credentials):  # pylint:disable=too-many-return-statements
        """Authorize user based on credentials.

        :param credentials: Credentials for ldap authorization.
        :type credentials: :obj:`fastapi.security.HTTPBasicCredentials`
        :return: Whether or not user is authorized and a user object.
        :rtype: tuple
        """
        user = await self.format_username(credentials.username)
        connection_data = LdapConnection(user=user, password=credentials.password)
        with Connection(self.server, **connection_data.dict()) as connection:
            connected = await sync_to_async(connection.bind)
            if not connected:
                return False, None

            search_filter = [
                f"({self.search.user_fields.username}={credentials.username})",
                f"(objectClass={self.search.object_class})",
            ]
            for key, value in self.search.extra_search_filters.items():
                search_filter.append(f"({key}={value})")
            search_filter = "(&{})".format("".join(search_filter))

            message_id = connection.search(
                search_base=self.search.search_base,
                search_filter=search_filter,
                search_scope=SUBTREE,
                attributes=ALL_ATTRIBUTES,
                get_operational_attributes=True,
                size_limit=1,
            )
            try:
                response, result = await sync_to_async(
                    connection.get_response, message_id
                )
                if result.get("result") != 0:
                    return False, None
                attributes = response[0].get("attributes")
                if not attributes:
                    return False, None
            except IndexError:
                return False, None
            try:
                user = User(
                    username=attributes[self.search.user_fields.username],
                    email=attributes.get(self.search.user_fields.email),
                    full_name=attributes.get(self.search.user_fields.full_name),
                )
            except KeyError:
                return False, None
            async with Database(self.config) as database:
                await database.set(user.username, user.dict())
            return True, user
        return False, None
