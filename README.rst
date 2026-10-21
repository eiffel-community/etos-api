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


SSE tracing
===========

The ``/sse/v2alpha/events/:identifier`` endpoint emits one server span per
connection attempt when ``OTEL_EXPORTER_OTLP_ENDPOINT`` or
``OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`` is set. Export uses OTLP/gRPC with the
same resource conventions as SSE metrics (default service name ``etos-sse``).
``OTEL_TRACES_EXPORTER=none`` or ``OTEL_SDK_DISABLED=true`` disables SSE tracing
and TestRun lookups. Trace setup/export failures do not stop the SSE service.
Legacy SSE v1/v1alpha routes and self-test endpoints are not traced.

For v1beta1 TestRuns, the server queries its namespace using the label
``etos.eiffel-community.github.io/id=<identifier>`` and links to the
``etos.eiffel-community.github.io/traceparent`` annotation. Optional
``tracestate`` is preserved; baggage is not imported. The namespace comes from
the mounted service-account namespace file, falling back to ``ETOS_NAMESPACE``
outside a pod, or can be set with ``-etosnamespace``. The resolver uses
in-cluster credentials; without them spans remain unlinked.

The SSE service account needs namespace-scoped ``list`` permission on
``testruns`` in API group ``etos.eiffel-community.github.io``. The ETOS
controller reconciles this permission into the SSE Role.

Each connection makes at most one logical lookup, bounded to one second and
canceled with the request or server shutdown. Missing, expired, ambiguous or
invalid TestRun metadata and Kubernetes errors leave the span unlinked without
preventing event delivery. Lookup errors are logged separately, and
``etos.sse.trace_link.outcome`` records the outcome without marking a healthy
stream as failed. No polling or caching is used, so a reconnect can resolve
metadata that was not available for the original connection.

Incoming W3C HTTP trace context remains the parent; without it, SSE starts a
new trace. The TestRun context is a span link, never a parent. Spans include
``etos.id``, the route template, reconnect intent, HTTP status and close reason.
Backend/write failures have bounded ``error.type`` values; event payloads,
query strings, credentials and baggage are not exported as span attributes.
Normal disconnects and server shutdown are not marked as errors.

Connection spans are exported after the handler exits, not while a stream is
still open. Use the existing SSE metrics for live connections.


Contribute
==========

- Issue Tracker: https://github.com/eiffel-community/etos/issues
- Source Code: https://github.com/eiffel-community/etos-api


Support
=======

If you are having issues, please let us know.
There is a mailing list at: etos-maintainers@googlegroups.com or just write an Issue.
