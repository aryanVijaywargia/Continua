package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformdb "github.com/continua-ai/continua/db/gen/go/platform"
	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestEngineMutations_AcceptRequestsWithoutPreviewHeader(t *testing.T) {
	router, apiKey := setupEnginePreviewSunsetRouter(t)

	startRec := postEngineStartRunThroughRouter(
		t,
		router,
		apiKey,
		"sunset-headerless-instance-"+uuid.NewString(),
		"sunset-headerless-request-"+uuid.NewString(),
		false,
	)
	require.Equal(t, http.StatusOK, startRec.Code, startRec.Body.String())
	started := decodeJSONBody[EngineStartRunResponse](t, startRec)
	require.NotEqual(t, uuid.Nil, started.RunId)

	cancelReq := httptest.NewRequest(
		http.MethodPost,
		"/v1/engine/runs/"+started.RunId.String()+"/cancel",
		nil,
	)
	cancelReq.Header.Set("X-API-Key", apiKey)
	cancelRec := httptest.NewRecorder()
	router.ServeHTTP(cancelRec, cancelReq)

	assert.NotEqual(t, http.StatusBadRequest, cancelRec.Code, cancelRec.Body.String())
	var cancelBody map[string]any
	require.NoError(t, json.Unmarshal(cancelRec.Body.Bytes(), &cancelBody))
	assert.NotEqual(t, "preview_header_required", cancelBody["code"])
}

func TestEngineMutations_PreviewHeaderStillAcceptedDuringSunset(t *testing.T) {
	router, apiKey := setupEnginePreviewSunsetRouter(t)

	startRec := postEngineStartRunThroughRouter(
		t,
		router,
		apiKey,
		"sunset-present-instance-"+uuid.NewString(),
		"sunset-present-request-"+uuid.NewString(),
		true,
	)
	require.Equal(t, http.StatusOK, startRec.Code, startRec.Body.String())
	started := decodeJSONBody[EngineStartRunResponse](t, startRec)
	assert.NotEqual(t, uuid.Nil, started.RunId)
}

func TestEngineMutations_HeaderlessPostLogsDeprecationWarning(t *testing.T) {
	previousLogger := slog.Default()
	capture := newEnginePreviewSunsetCaptureHandler()
	slog.SetDefault(slog.New(capture))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	router, apiKey := setupEnginePreviewSunsetRouter(t)
	capture.clear()

	postEngineStartRunThroughRouter(
		t,
		router,
		apiKey,
		"sunset-warning-instance-"+uuid.NewString(),
		"sunset-warning-request-"+uuid.NewString(),
		false,
	)

	assert.Truef(
		t,
		capture.hasWarningMentioning(enginePreviewHeader),
		"expected a warn-level slog record mentioning %s",
		enginePreviewHeader,
	)

	capture.clear()
	withHeaderRec := postEngineStartRunThroughRouter(
		t,
		router,
		apiKey,
		"sunset-no-warning-instance-"+uuid.NewString(),
		"sunset-no-warning-request-"+uuid.NewString(),
		true,
	)
	require.Equal(t, http.StatusOK, withHeaderRec.Code, withHeaderRec.Body.String())
	assert.Falsef(
		t,
		capture.hasWarningMentioning(enginePreviewHeader),
		"did not expect a warn-level slog record mentioning %s when the header is present",
		enginePreviewHeader,
	)
}

func TestEngineRoutes_AvailabilityFlagStillGates(t *testing.T) {
	ctx := context.Background()
	platformStore := store.New(testutil.TestDB(t))
	apiKey := "engine-sunset-disabled-" + uuid.NewString()
	_, err := platformStore.Queries().CreateProject(ctx, platformdb.CreateProjectParams{
		Name:       "engine-sunset-disabled-" + uuid.NewString()[:8],
		ApiKeyHash: hashTestAPIKey(apiKey),
	})
	require.NoError(t, err)

	server := NewServer(platformStore, nil)
	router := newAuthenticatedRouter(t, server, platformStore)

	getReq := httptest.NewRequest(http.MethodGet, "/v1/engine/runs/"+uuid.NewString(), nil)
	getReq.Header.Set("X-API-Key", apiKey)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	require.Equal(t, http.StatusNotFound, getRec.Code)

	postRec := postEngineStartRunThroughRouter(
		t,
		router,
		apiKey,
		"sunset-disabled-instance-"+uuid.NewString(),
		"sunset-disabled-request-"+uuid.NewString(),
		false,
	)
	require.Equal(t, http.StatusNotFound, postRec.Code)
}

func TestOpenAPIContract_PreviewHeaderDeprecatedAndOptional(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", ".."))
	contractPath := filepath.Join(repoRoot, "contracts", "openapi", "openapi.yaml")
	contents, err := os.ReadFile(contractPath)
	require.NoError(t, err)

	lines := strings.Split(string(contents), "\n")
	const headerParameterName = "name: X-Continua-Engine-Preview"
	found := 0
	for lineIndex, line := range lines {
		if !strings.Contains(line, headerParameterName) {
			continue
		}

		found++
		headerIndent := len(line) - len(strings.TrimLeft(line, " "))
		end := lineIndex + 1
		for end < len(lines) {
			if strings.TrimSpace(lines[end]) != "" && len(lines[end])-len(strings.TrimLeft(lines[end], " ")) <= headerIndent {
				break
			}
			end++
		}
		block := strings.Join(lines[lineIndex:end], "\n")
		assert.NotContainsf(
			t,
			block,
			"required: true",
			"preview header parameter block %d must be optional:\n%s",
			found,
			block,
		)
		assert.Containsf(
			t,
			block,
			"deprecated: true",
			"preview header parameter block %d must be deprecated:\n%s",
			found,
			block,
		)
	}

	assert.GreaterOrEqual(t, found, 10, "expected at least 10 preview header parameter blocks")
}

func setupEnginePreviewSunsetRouter(t *testing.T) (http.Handler, string) {
	t.Helper()

	ctx := context.Background()
	pool := testutil.TestDB(t)
	platformStore := store.New(pool)
	engineQueries := enginedb.New(pool)

	apiKey := "engine-sunset-" + uuid.NewString()
	_, err := platformStore.Queries().CreateProject(ctx, platformdb.CreateProjectParams{
		Name:       "engine-sunset-" + uuid.NewString()[:8],
		ApiKeyHash: hashTestAPIKey(apiKey),
	})
	require.NoError(t, err)
	require.NoError(t, publishCheckoutDefinition(ctx, engineQueries))

	server := NewServer(platformStore, nil)
	server.engineControl = newEngineControlService(platformStore)
	server.enginePublicAPIEnabled = true

	return newAuthenticatedRouter(t, server, platformStore), apiKey
}

func postEngineStartRunThroughRouter(
	t *testing.T,
	router http.Handler,
	apiKey, instanceKey, requestKey string,
	withPreviewHeader bool,
) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(map[string]string{
		"definition_name":    "checkout",
		"definition_version": "v1",
		"instance_key":       instanceKey,
		"request_key":        requestKey,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/v1/engine/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	if withPreviewHeader {
		req.Header.Set(enginePreviewHeader, "1")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

type enginePreviewSunsetCapturedRecord struct {
	record       slog.Record
	handlerAttrs []slog.Attr
	groups       []string
}

type enginePreviewSunsetCaptureState struct {
	mu      sync.Mutex
	records []enginePreviewSunsetCapturedRecord
}

type enginePreviewSunsetCaptureHandler struct {
	state  *enginePreviewSunsetCaptureState
	attrs  []slog.Attr
	groups []string
}

func newEnginePreviewSunsetCaptureHandler() *enginePreviewSunsetCaptureHandler {
	return &enginePreviewSunsetCaptureHandler{state: &enginePreviewSunsetCaptureState{}}
}

func (h *enginePreviewSunsetCaptureHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *enginePreviewSunsetCaptureHandler) Handle(_ context.Context, record slog.Record) error {
	h.state.mu.Lock()
	defer h.state.mu.Unlock()

	h.state.records = append(h.state.records, enginePreviewSunsetCapturedRecord{
		record:       record.Clone(),
		handlerAttrs: append([]slog.Attr(nil), h.attrs...),
		groups:       append([]string(nil), h.groups...),
	})
	return nil
}

func (h *enginePreviewSunsetCaptureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &enginePreviewSunsetCaptureHandler{
		state:  h.state,
		attrs:  append(append([]slog.Attr(nil), h.attrs...), attrs...),
		groups: append([]string(nil), h.groups...),
	}
}

func (h *enginePreviewSunsetCaptureHandler) WithGroup(name string) slog.Handler {
	return &enginePreviewSunsetCaptureHandler{
		state:  h.state,
		attrs:  append([]slog.Attr(nil), h.attrs...),
		groups: append(append([]string(nil), h.groups...), name),
	}
}

func (h *enginePreviewSunsetCaptureHandler) clear() {
	h.state.mu.Lock()
	defer h.state.mu.Unlock()
	h.state.records = nil
}

func (h *enginePreviewSunsetCaptureHandler) hasWarningMentioning(needle string) bool {
	h.state.mu.Lock()
	defer h.state.mu.Unlock()

	for _, captured := range h.state.records {
		if captured.record.Level < slog.LevelWarn {
			continue
		}
		if strings.Contains(captured.record.Message, needle) || slicesContainString(captured.groups, needle) {
			return true
		}
		for _, attr := range captured.handlerAttrs {
			if slogAttrMentions(attr, needle) {
				return true
			}
		}
		mentioned := false
		captured.record.Attrs(func(attr slog.Attr) bool {
			mentioned = slogAttrMentions(attr, needle)
			return !mentioned
		})
		if mentioned {
			return true
		}
	}
	return false
}

func slogAttrMentions(attr slog.Attr, needle string) bool {
	if strings.Contains(attr.Key, needle) {
		return true
	}

	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		for _, child := range value.Group() {
			if slogAttrMentions(child, needle) {
				return true
			}
		}
		return false
	}
	return strings.Contains(value.String(), needle)
}

func slicesContainString(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
