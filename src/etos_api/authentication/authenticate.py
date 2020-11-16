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
"""Authentication endpoints for ETOS API."""
import os
from pathlib import Path
from datetime import datetime, timedelta
from fastapi import status, HTTPException, Depends, Request, APIRouter
from fastapi.security import HTTPBearer
from jose import jwt, JWTError, ExpiredSignatureError
from box import Box

from etos_api.authentication import authorize
from etos_api.library.database import Database
from .schemas import Token, TokenConfig, User


ROUTER = APIRouter()

CONFIG = Box.from_json(filename=os.getenv("CONFIG", "config.json"))
SECRET = Path(CONFIG.secret)
assert SECRET.is_file(), f"{SECRET} is not a file!"
SECRET_KEY = SECRET.read_text().strip()
BACKEND = getattr(
    authorize, CONFIG.authorization_backend.name, authorize.NoAuthentication
)

ALGORITHM = "HS256"
ISSUER = CONFIG.issuer
AUDIENCE = [
    "etos-api/etos/",
    "etos-api/environment-provider/configure",
    "etos-api/selftest/ping",
]
AUTH_SCHEME = HTTPBearer()


async def get_audience(request: Request):
    """Get audience for the request.

    An audience is used to limit which endpoints a token can access.

    :param request: Request model for fastapi.
    :type request: :obj:`fastapi.Request`
    :return: The audience for the endpoint based on path.
    :rtype: str
    """
    return f"etos-api{request.scope.get('path')}"


async def generate_token(data: TokenConfig, expires_delta=None):
    """Generate a JWT access token based on data.

    :param data: Extra data to add to token.
    :type data: :obj:`TokenConfig`
    :param expires_delta: Timedelta for expiration of token
    :type expires_delta: :obj:`timedelta`
    :return: An encoded JWT token.
    :rtype: str
    """
    to_encode = data.dict().copy()
    if expires_delta:
        expire = datetime.utcnow() + expires_delta
    else:
        expire = datetime.utcnow() + timedelta(minutes=15)
    to_encode.update({"exp": expire})
    encoded_jwt = jwt.encode(to_encode, CONFIG.secret, algorithm=ALGORITHM)
    return encoded_jwt


async def validate(
    credentials: str = Depends(AUTH_SCHEME), audience: str = Depends(get_audience)
):
    """Validate token against stored tokens.

    :raises HTTPException: If token is expired or unknown errors.

    :param credentials: Credentials in the Authorization header.
    :type credentials: :obj:`fastapi.security.HTTPAuthorizationCredentials`
    :param audience: Which audience to validate the token against.
    :type audience: str
    :return: Decoded JWT token data.
    :rtype: dict
    """
    generated_token = credentials.credentials
    try:
        payload = jwt.decode(
            generated_token, CONFIG.secret, algorithms=[ALGORITHM], audience=audience
        )
        username: str = payload.get("sub")
        assert payload.get("iss") == ISSUER, "Wrong issuer in token."
        assert username is not None, "User does not exist."
    except ExpiredSignatureError as exception:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Signature expired",
            headers={"WWW-Authenticate": "Bearer"},
        ) from exception
    except (AssertionError, JWTError) as exception:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Could not validate credentials",
            headers={"WWW-Authenticate": "Bearer"},
        ) from exception
    async with Database(CONFIG) as database:
        database_user = await database.get(username)
        if not database_user:
            raise HTTPException(
                status_code=status.HTTP_401_UNAUTHORIZED,
                detail="No user connected to token.",
                headers={"WWW-Authenticate": "Bearer"},
            )
        return User(**database_user)


async def get_token(user):
    """Generate a token for logged in user.

    :param user: User to generate token for.
    :type user: :obj:`etos_api.authentication.schemas.User`
    :return: JWT access token for user.
    :rtype: str
    """
    config = TokenConfig(sub=user.username, aud=AUDIENCE, iss=ISSUER)
    generated_token = await generate_token(config, timedelta(minutes=1))
    return Token(access_token=generated_token, token_type="bearer")


CONFIG["get_token"] = get_token
CONFIG["validate"] = validate
ROUTER.add_api_route(
    "/token", BACKEND(CONFIG).get_token, response_model=Token, methods=["post"]
)
