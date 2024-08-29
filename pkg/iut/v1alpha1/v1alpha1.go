// Copyright Axis Communications AB.
//
// For a full list of individual contributors, please see the commit history.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package v1alpha1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	config "github.com/eiffel-community/etos-api/internal/configs/iut"
	"github.com/eiffel-community/etos-api/internal/iut/checkoutable"
	"github.com/eiffel-community/etos-api/internal/iut/contextmanager"
	"github.com/eiffel-community/etos-api/internal/iut/responses"
	"github.com/eiffel-community/etos-api/pkg/iut/application"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/eiffel-community/eiffelevents-sdk-go"
	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"
	"github.com/package-url/packageurl-go"
	"github.com/sirupsen/logrus"
)

var (
	service_version  string
	otel_sdk_version string
)

// BASEREGEX for matching /testrun/tercc-id/provider/iuts/reference.
const BASEREGEX = "/testrun/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/provider/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/iuts"

type V1Alpha1Application struct {
	logger   *logrus.Entry
	cfg      config.Config
	database *clientv3.Client
	cm       *contextmanager.ContextManager
	wg       *sync.WaitGroup
}

type V1Alpha1Handler struct {
	logger   *logrus.Entry
	cfg      config.Config
	database *clientv3.Client
	cm       *contextmanager.ContextManager
	wg       *sync.WaitGroup
}

type StartRequest struct {
	MinimumAmount     int                                                 `json:"minimum_amount"`
	MaximumAmount     int                                                 `json:"maximum_amount"`
	ArtifactIdentity  string                                              `json:"identity"`
	ArtifactID        string                                              `json:"artifact_id"`
	ArtifactCreated   eiffelevents.ArtifactCreatedV3                      `json:"artifact_created,omitempty"`
	ArtifactPublished eiffelevents.ArtifactPublishedV3                    `json:"artifact_published,omitempty"`
	TERCC             eiffelevents.TestExecutionRecipeCollectionCreatedV4 `json:"tercc,omitempty"`
	Context           uuid.UUID                                           `json:"context,omitempty"`
	Dataset           Dataset                                             `json:"dataset,omitempty"`
}

type Dataset struct {
	Greed interface{} `json:"greed"`
}

type StartResponse struct {
	Id uuid.UUID `json:"id"`
}

type statusResponse struct {
	Id          uuid.UUID `json:"id"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
}

type StopRequest []*checkoutable.Iut

type StatusRequest struct {
	Id uuid.UUID `json:"id"`
}

// Close does nothing atm. Present for interface coherence
func (a *V1Alpha1Application) Close() {
	a.wg.Wait()
}

// New returns a new V1Alpha1Application object/struct
func New(cfg config.Config, log *logrus.Entry, ctx context.Context, cm *contextmanager.ContextManager, cli *clientv3.Client) application.Application {
	return &V1Alpha1Application{
		logger:   log,
		cfg:      cfg,
		database: cli,
		cm:       cm,
		wg:       &sync.WaitGroup{},
	}
}

// isOtelEnabled returns true if OpenTelemetry is enabled, otherwise false
func isOtelEnabled() bool {
	_, endpointSet := os.LookupEnv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
	return endpointSet
}

// initTracer initializes the OpenTelemetry instrumentation for trace collection
func (a V1Alpha1Application) initTracer() {
	if !isOtelEnabled() {
		a.logger.Infof("No OpenTelemetry collector is set. OpenTelemetry traces will not be available.")
		return
	}
	collector := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
	a.logger.Infof("Using OpenTelemetry collector: %s", collector)
	// Create OTLP exporter to export traces
	exporter, err := otlptrace.New(context.Background(), otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(collector),
	))
	if err != nil {
		log.Fatal(err)
	}

	// Create a resource with service name attribute
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String("iut-provider"),
			semconv.ServiceNamespaceKey.String(os.Getenv("OTEL_SERVICE_NAMESPACE")),
			semconv.ServiceVersionKey.String(service_version),
			semconv.TelemetrySDKLanguageGo.Key.String("go"),
			semconv.TelemetrySDKNameKey.String("opentelemetry"),
			semconv.TelemetrySDKVersionKey.String(otel_sdk_version),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	// Create a TraceProvider with the exporter and resource
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	// Set the global TracerProvider
	otel.SetTracerProvider(tp)
	// Set the global propagator to TraceContext (W3C Trace Context)
	otel.SetTextMapPropagator(propagation.TraceContext{})
}

// LoadRoutes loads all the v1alpha1 routes.
func (a V1Alpha1Application) LoadRoutes(router *httprouter.Router) {
	handler := &V1Alpha1Handler{a.logger, a.cfg, a.database, a.cm, a.wg}
	router.GET("/v1alpha1/selftest/ping", handler.Selftest)
	router.POST("/start", handler.panicRecovery(handler.timeoutHandler(handler.Start)))
	router.GET("/status", handler.panicRecovery(handler.timeoutHandler(handler.Status)))
	router.POST("/stop", handler.panicRecovery(handler.timeoutHandler(handler.Stop)))

	a.initTracer()
}

// getOtelTracer returns the current OpenTelemetry tracer
func (h V1Alpha1Handler) getOtelTracer() trace.Tracer {
	return otel.Tracer("iut-provider")
}

// getOtelContext returns OpenTelemetry context from the given HTTP request object
func (h V1Alpha1Handler) getOtelContext(ctx context.Context, r *http.Request) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))
}

// recordOtelException records an error to the given span
func (h V1Alpha1Handler) recordOtelException(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// Selftest is a handler to just return 204.
func (h V1Alpha1Handler) Selftest(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	responses.RespondWithError(w, http.StatusNoContent, "")
}

// Start handles the start request and checks out IUTs and stores the status in a database.
func (h V1Alpha1Handler) Start(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	h.wg.Add(1)
	defer h.wg.Done()
	ctx := context.Background()
	identifier := r.Header.Get("X-Etos-Id")
	logger := h.logger.WithField("identifier", identifier).WithContext(ctx)
	checkOutID := uuid.New()
	startReq, err := h.verifyStartInput(ctx, logger, r)
	if err != nil {
		logger.Error(err)
		sendError(w, err)
		return
	}

	if startReq.MaximumAmount == 0 {
		startReq.MaximumAmount = 100
	}

	ctx = h.getOtelContext(ctx, r)

	_, span := h.getOtelTracer().Start(ctx, "start", trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()

	if isOtelEnabled() && os.Getenv("OTEL_JAEGER_URL") != "" {
		traceID := span.SpanContext().TraceID().String()
		jaegerURL, _ := url.Parse(os.Getenv("OTEL_JAEGER_URL"))
		jaegerURL.Path = path.Join(jaegerURL.Path, traceID)
		logger.WithField("user_log", true).Infof("Jaeger trace URL: %s", jaegerURL)
	}

	time.Sleep(1 * time.Second) // Allow ETOS environment provider to send logs first.
	logger.WithField("user_log", true).Infof("Check out ID: %s", checkOutID)

	if startReq.MaximumAmount == 0 {
		startReq.MaximumAmount = 100
	}

	go h.DoCheckout(trace.ContextWithSpan(ctx, span), logger, startReq, checkOutID, identifier)

	span.SetAttributes(attribute.Int("etos.iut_provider.checkout.start_request.maximum_amount", startReq.MaximumAmount))
	span.SetAttributes(attribute.Int("etos.iut_provider.checkout.start_request.minimum_amount", startReq.MinimumAmount))
	span.SetAttributes(attribute.String("etos.iut_provider.checkout.start_request.artifact_id", fmt.Sprintf("%v", startReq.ArtifactID)))
	span.SetAttributes(attribute.String("etos.iut_provider.checkout.start_request.artifact_identity", fmt.Sprintf("%v", startReq.ArtifactIdentity)))
	span.SetAttributes(attribute.String("etos.iut_provider.checkout.checkout_id", fmt.Sprintf("%v", checkOutID)))

	responses.RespondWithJSON(w, http.StatusOK, StartResponse{Id: checkOutID})
}

// Status handles the status request and queries the database for the Iut checkout status
func (h V1Alpha1Handler) Status(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	h.wg.Add(1)
	defer h.wg.Done()
	// TimeoutHandler adds a Timeout on r.Context().
	ctx := h.getOtelContext(r.Context(), r)
	identifier := r.Header.Get("X-Etos-Id")
	logger := h.logger.WithField("identifier", identifier).WithContext(ctx)

	statusReq, err := h.verifyStatusInput(ctx, r)
	if err != nil {
		msg := fmt.Errorf("Failed to verify input for Status() request: %v - Reason: %s", r, err.Error())
		_, span := h.getOtelTracer().Start(ctx, "status", trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		h.recordOtelException(span, msg)
		logger.Error(msg)
		sendError(w, msg)
		return
	}

	key := fmt.Sprintf("/testrun/%s", identifier)
	status := NewStatus(key, statusReq.Id.String(), h.database)
	if err := status.Load(ctx); err != nil {
		msg := fmt.Errorf("Failed to read status, request: %v - Reason: %s", statusReq, err.Error())
		_, span := h.getOtelTracer().Start(ctx, "status", trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		h.recordOtelException(span, msg)
		logger.Error(msg)
		failStatus := statusResponse{
			Id:          statusReq.Id,
			Status:      "FAILED",
			Description: msg.Error(),
		}
		responses.RespondWithJSON(w, http.StatusInternalServerError, failStatus)
		return
	}

	if status.Status != "PENDING" { // avoid creating too many spans while during wait loop
		_, span := h.getOtelTracer().Start(ctx, "status", trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		span.SetAttributes(attribute.String("etos.iut_provider.status.iut_refs", fmt.Sprintf("%s", status.IutReferences)))
		span.SetAttributes(attribute.String("etos.iut_provider.status.status", status.Status))
		span.SetAttributes(attribute.String("etos.iut_provider.status.description", status.Description))
	}
	responses.RespondWithJSON(w, http.StatusOK, status)
}

// Stop handles the stop request and checks in all the provided Iuts
func (h V1Alpha1Handler) Stop(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	h.wg.Add(1)
	defer h.wg.Done()
	// TimeoutHandler adds a Timeout on r.Context().
	ctx := h.getOtelContext(r.Context(), r)
	identifier := r.Header.Get("X-Etos-Id")
	logger := h.logger.WithField("identifier", identifier).WithContext(ctx)

	_, span := h.getOtelTracer().Start(ctx, "stop", trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()

	iuts, err := h.verifyStopInput(ctx, r)
	if err != nil {
		msg := fmt.Errorf("Failed to verify input for Stop() request: %v - Reason: %s", r, err.Error())
		h.recordOtelException(span, msg)
		sendError(w, msg)
		return
	}

	iut_refs := ""
	refs_regex := ""
	for _, iut := range iuts {
		iut.AddLogger(logger)
		logger.WithField("user_log", true).Infof("Checking in IUT with reference %s", iut.Reference)
		if iut_refs != "" {
			iut_refs += ", "
			refs_regex += "|"
		}
		iut_refs += iut.Reference
		refs_regex += iut.Reference
	}
	regex := regexp.MustCompile(fmt.Sprintf("%s/(%s)", BASEREGEX, refs_regex))

	// Due to the way ETOS checks in IUTs (it does not provide the checkout ID), we need to iterate
	// over all checkouts for a testrun and check them against the references that we receive from
	// ETOS.
	key := fmt.Sprintf("/testrun/%s/provider", identifier)
	response, err := h.database.Get(ctx, key, clientv3.WithPrefix())
	if err != nil {
		msg := fmt.Errorf(fmt.Sprintf("Failed to check-in iuts: %s", iut_refs))
		logger.WithError(err).Error(msg)
		h.recordOtelException(span, msg)
		responses.RespondWithError(w, http.StatusInternalServerError, msg.Error())
		return
	}
	statuses := map[string]*Status{}
	for _, kv := range response.Kvs {
		// Verify that 'ev.Value' is an actual IUT definition and not another
		// field in the ETCD database. Since we are prefix searching on /testrun/suiteid/provider/
		// it is very possible that more data will arrive than we are interested in.
		if !regex.Match(kv.Key) {
			continue
		}
		splitKey := strings.Split(string(kv.Key), "/")
		// We know for a fact that the key is /testrun/suiteid/provider/ID/iuts/reference
		// so the 4th key will always be ID.
		id := splitKey[4]
		_, ok := statuses[id]
		if !ok {
			key = fmt.Sprintf("/testrun/%s", identifier)
			statuses[id] = NewStatus(key, id, h.database)
		}
	}

	var multiErr error
	for _, status := range statuses {
		if err = status.Load(ctx); err != nil {
			multiErr = errors.Join(multiErr, err)
			continue
		}
		if err = h.checkIn(ctx, logger, iuts, status, false); err != nil {
			multiErr = errors.Join(multiErr, err)
			continue
		}
		status.Save(r.Context())
	}
	if multiErr != nil {
		msg := fmt.Errorf(fmt.Sprintf("Failed to check-in iuts: %s", iut_refs))
		logger.WithError(err).Error(msg)
		h.recordOtelException(span, msg)
		responses.RespondWithError(w, http.StatusInternalServerError, msg.Error())
		return
	}

	iuts_json := make([]map[string]interface{}, len(iuts))
	for i, iut := range iuts {
		iuts_json[i] = map[string]interface{}{
			"reference": iut.Reference,
		}
	}
	iuts_json_str, _ := json.Marshal(iuts_json)

	span.SetAttributes(attribute.String("etos.iut_provider.stop.iuts", string(iuts_json_str)))
	responses.RespondWithJSON(w, http.StatusNoContent, "")
}

// verifyStartInput verify input (json body) from a start request
func (h V1Alpha1Handler) verifyStartInput(ctx context.Context, logger *logrus.Entry, r *http.Request) (StartRequest, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return StartRequest{}, NewHTTPError(
			fmt.Errorf("failed read request body - Reason: %s", err.Error()),
			http.StatusBadRequest,
		)
	}
	defer r.Body.Close()

	request, err := h.tryLoadStartRequest(ctx, logger, body)
	if err != nil {
		return request, NewHTTPError(err, http.StatusBadRequest)
	}

	if request.ArtifactID == "" || request.ArtifactIdentity == "" {
		return request, NewHTTPError(
			errors.New("both 'artifact_identity' and 'artifact_id' are required"),
			http.StatusBadRequest,
		)
	}

	if request.MinimumAmount == 0 {
		return request, NewHTTPError(
			errors.New("minimumAmount parameter is mandatory"),
			http.StatusBadRequest,
		)
	}

	_, purlErr := packageurl.FromString(request.ArtifactIdentity)
	if purlErr != nil {
		return request, NewHTTPError(purlErr, http.StatusBadRequest)
	}

	return request, ctx.Err()
}

// verifyStatusInput verify input (url parameters) from the status request
func (h V1Alpha1Handler) verifyStatusInput(ctx context.Context, r *http.Request) (StatusRequest, error) {
	id, err := uuid.Parse(r.URL.Query().Get("id"))
	if err != nil {
		return StatusRequest{}, NewHTTPError(
			fmt.Errorf("Error parsing id parameter in status request - Reason: %s", err.Error()),
			http.StatusBadRequest)
	}
	request := StatusRequest{Id: id}

	return request, ctx.Err()
}

// verifyStopInput verify input (json body) from the stop request
func (h V1Alpha1Handler) verifyStopInput(ctx context.Context, r *http.Request) (StopRequest, error) {
	request := StopRequest{}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return request, NewHTTPError(fmt.Errorf("unable to decode post body %+v", err), http.StatusBadRequest)
	}
	return request, ctx.Err()
}

// sendError sends an error HTTP response depending on which error has been returned.
func sendError(w http.ResponseWriter, err error) {
	httpError, ok := err.(*HTTPError)
	if !ok {
		responses.RespondWithError(w, http.StatusInternalServerError, fmt.Sprintf("unknown error %+v", err))
	} else {
		responses.RespondWithError(w, httpError.Code, httpError.Message)
	}
}

// timeoutHandler will change the request context to a timeout context.
func (h V1Alpha1Handler) timeoutHandler(
	fn func(http.ResponseWriter, *http.Request, httprouter.Params),
) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		ctx, cancel := context.WithTimeout(r.Context(), h.cfg.Timeout())
		defer cancel()
		newRequest := r.WithContext(ctx)
		fn(w, newRequest, ps)
	}
}

// panicRecovery tracks panics from the service, logs them and returns an error response to the user.
func (h V1Alpha1Handler) panicRecovery(
	fn func(http.ResponseWriter, *http.Request, httprouter.Params),
) func(http.ResponseWriter, *http.Request, httprouter.Params) {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		defer func() {
			if err := recover(); err != nil {
				buf := make([]byte, 2048)
				n := runtime.Stack(buf, false)
				buf = buf[:n]
				h.logger.WithField(
					"identifier", ps.ByName("identifier"),
				).WithContext(
					r.Context(),
				).Errorf("recovering from err %+v\n %s", err, buf)
				identifier := ps.ByName("identifier")
				responses.RespondWithError(
					w,
					http.StatusInternalServerError,
					fmt.Sprintf("unknown error: contact server admin with id '%s'", identifier),
				)
			}
		}()
		fn(w, r, ps)
	}
}
