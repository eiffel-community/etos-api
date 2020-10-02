FROM python:3.8.3-buster AS build

COPY . /src
WORKDIR /src
RUN python3 setup.py bdist_wheel

FROM python:3.8.3-slim-buster

COPY --from=build /src/dist/*.whl /tmp
# hadolint ignore=DL3013
RUN pip install --no-cache-dir /tmp/*.whl

RUN groupadd -r etos && useradd -r -s /bin/false -g etos etos
USER etos
EXPOSE 8080

LABEL org.opencontainers.image.source=https://github.com/eiffel-community/etos-api
LABEL org.opencontainers.image.authors=etos-maintainers@googlegroups.com
LABEL org.opencontainers.image.licenses=Apache-2.0

ENV GUNICORN_CMD_ARGS="--name etos_api --bind 0.0.0.0:8080 --worker-class gevent --worker-connections 1000 --workers 5"
ENTRYPOINT ["gunicorn", "etos_api.webserver:FALCON_APP"]
