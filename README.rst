========
ETOS API
========

.. image:: https://img.shields.io/badge/Stage-Sandbox-yellow.svg
  :target: https://github.com/eiffel-community/community/blob/master/PROJECT_LIFECYCLE.md#stage-sandbox

ETOS (Eiffel Test Orchestration System) API


Description
===========

API for starting ETOS executions and for getting extra information about the system.


Installation
============

   pip install .


Running dockers in development mode
===================================

   make DEPLOY=etos-iut start
   make DEPLOY=etos-logarea start
   make DEPLOY=etos-sse start


Building the SSE image
=====================

``make sse-docker`` passes the current Git commit to the Docker build so that
the SSE server logs it even with Go VCS stamping disabled. Git metadata is
not copied into the SSE build container.

The ``version`` log field is the short commit SHA and identifies the exact
source.

For a direct Docker build, supply revision explicitly::

   docker build --build-arg BUILD_REVISION="$(git rev-parse --short HEAD)" \
     -f deploy/etos-sse/Dockerfile -t etos-sse:local .

The SSE image build fails if ``BUILD_REVISION``is missing or empty.


Contribute
==========

- Issue Tracker: https://github.com/eiffel-community/etos/issues
- Source Code: https://github.com/eiffel-community/etos-api


Support
=======

If you are having issues, please let us know.
There is a mailing list at: etos-maintainers@googlegroups.com or just write an Issue.
