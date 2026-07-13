package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hapax-systems/reins/internal/api"
	"github.com/hapax-systems/reins/internal/grammar"
	"github.com/hapax-systems/reins/internal/model"
)

const contextCarrierManifestSchema = "hapax.reins-context-wire-conformance.v2"
const currentContextFixtureSHA256 = "594b6b96656cea2a46e4d50c2201152523bcf4530f0afcb0360425e76c17fae9"

type contextCarrierManifest struct {
	Schema              string               `json:"schema"`
	FixtureSHA256       string               `json:"fixture_sha256"`
	WheelSHA256         *string              `json:"wheel_sha256"`
	PackageState        string               `json:"package_state"`
	MaxContextReadBytes int                  `json:"max_context_read_bytes"`
	Cases               []contextCarrierCase `json:"cases"`
}

type contextCarrierCase struct {
	Name                string   `json:"name"`
	BodyPath            string   `json:"body_path"`
	BodySHA256          string   `json:"body_sha256"`
	BodyBytes           int      `json:"body_bytes"`
	ProducerPath        *string  `json:"producer_path"`
	ProducerSHA256      *string  `json:"producer_sha256"`
	ProducerBytes       int      `json:"producer_bytes"`
	ExpectedState       string   `json:"expected_state"`
	ExpectedReasonCodes []string `json:"expected_reason_codes"`
	ActiveField         string   `json:"active_field"`
	Query               string   `json:"query"`
	ValidationMode      string   `json:"validation_mode"`
}

func TestContextCarrierWireConformance(t *testing.T) {
	manifestPath := os.Getenv("REINS_CONTEXT_CARRIER_CONFORMANCE_MANIFEST")
	if manifestPath == "" {
		t.Skip("opt-in exact Python-to-HTTP-to-Go carrier proof")
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	var manifest contextCarrierManifest
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatalf("decode conformance manifest: %v", err)
	}
	if manifest.Schema != contextCarrierManifestSchema {
		t.Fatalf("manifest schema = %q", manifest.Schema)
	}
	if manifest.MaxContextReadBytes != 16<<20 {
		t.Fatalf("manifest byte limit = %d", manifest.MaxContextReadBytes)
	}
	if manifest.FixtureSHA256 != currentContextFixtureSHA256 {
		t.Fatalf("fixture SHA-256 = %q", manifest.FixtureSHA256)
	}
	if manifest.WheelSHA256 != nil || manifest.PackageState != "hold_missing_exact_current_artifact" {
		t.Fatalf("manifest falsely claims a current package artifact: %+v", manifest)
	}

	required := map[string]bool{
		"operator_escaped_pretty_hold": false,
		"lifecycle_possibility_hold":   false,
		"operation_hold":               false,
		"compatibility_hold":           false,
		"yard_dark":                    false,
		"query_dark":                   false,
		"absent_dark":                  false,
		"malformed_dark":               false,
		"duplicate_key_dark":           false,
		"invalid_utf8_dark":            false,
		"edge_whitespace_dark":         false,
		"unsupported_schema_dark":      false,
		"bool_integer_dark":            false,
		"oversized_producer_dark":      false,
		"outer_exact_limit_hold":       false,
		"outer_over_limit_dark":        false,
		"no_wheel_dark":                false,
	}
	seen := make(map[string]struct{}, len(manifest.Cases))
	root := filepath.Dir(manifestPath)
	var exactLimitBody []byte
	for _, testCase := range manifest.Cases {
		if _, duplicate := seen[testCase.Name]; duplicate {
			t.Fatalf("duplicate manifest case %q", testCase.Name)
		}
		seen[testCase.Name] = struct{}{}
		if _, ok := required[testCase.Name]; !ok {
			t.Fatalf("unexpected manifest case %q", testCase.Name)
		}
		required[testCase.Name] = true
		if testCase.Name == "operator_escaped_pretty_hold" &&
			testCase.ValidationMode != "test_only_current_fixture_contract" {
			t.Fatalf("current source fixture has ambiguous validation mode %q", testCase.ValidationMode)
		}
		t.Run(testCase.Name, func(t *testing.T) {
			body := readContextCarrierArtifact(
				t,
				root,
				testCase.BodyPath,
				testCase.BodySHA256,
				testCase.BodyBytes,
				manifest.MaxContextReadBytes,
			)
			if len(body) > manifest.MaxContextReadBytes {
				t.Fatalf("Python body exceeds bound: %d", len(body))
			}
			if testCase.Name == "outer_exact_limit_hold" {
				if len(body) != manifest.MaxContextReadBytes {
					t.Fatalf("exact-limit body is %d bytes", len(body))
				}
				exactLimitBody = append([]byte(nil), body...)
			}
			if strings.HasPrefix(testCase.Name, "outer_") && testCase.ValidationMode != "test_only_boundary_stub" {
				t.Fatalf("boundary case lacks explicit test-only validator label")
			}

			var producer []byte
			if testCase.ProducerPath != nil {
				if testCase.ProducerSHA256 == nil {
					t.Fatal("producer path has no SHA-256")
				}
				producer = readContextCarrierArtifact(
					t,
					root,
					*testCase.ProducerPath,
					*testCase.ProducerSHA256,
					testCase.ProducerBytes,
					manifest.MaxContextReadBytes+1,
				)
			}

			readout, fetchErr := fetchContextCarrierBody(t, body)
			if fetchErr != nil {
				t.Fatalf("FetchContext rejected Python body: %v", fetchErr)
			}
			assertContextCarrierReadout(t, readout, testCase, body, producer)

			folded := model.New("REINS").FoldContext(readout, "")
			assertFoldedContextCarrier(t, folded, testCase, body, producer)
			wiped := folded.SetAIR(true)
			assertContextCarrierBytesAbsent(t, wiped.ContextReadout)

			if testCase.ExpectedState == string(grammar.ContextReadHold) {
				staleReadout, staleErr := fetchContextCarrierBody(t, body)
				if staleErr != nil {
					t.Fatalf("second FetchContext failed: %v", staleErr)
				}
				base := model.New("REINS")
				base.ContextEpoch = 9
				updated, _ := base.Update(model.ContextMsg{Readout: staleReadout, Epoch: 8})
				staleFold := updated.(model.Model)
				if staleFold.ContextReadout.State == grammar.ContextReadHold {
					t.Fatal("stale epoch rehydrated a HOLD carrier")
				}
				assertContextCarrierBytesAbsent(t, staleFold.ContextReadout)
			}
		})
	}
	for name, present := range required {
		if !present {
			t.Fatalf("required manifest case %q is absent", name)
		}
	}
	if len(exactLimitBody) != manifest.MaxContextReadBytes {
		t.Fatal("exact-limit body was not retained for the Go over-limit probe")
	}
	oversized := append(exactLimitBody, '\n')
	if _, err := fetchContextCarrierBody(t, oversized); err == nil {
		t.Fatal("FetchContext accepted a body one byte over the limit")
	}
	clear(oversized)
}

func TestCurrentContextCarrierSourceFixture(t *testing.T) {
	path := filepath.Join("..", "..", "api", "fixtures", "context-canon-gate0-carriers.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(payload)); got != currentContextFixtureSHA256 {
		t.Fatalf("current fixture SHA-256 = %s", got)
	}
	var fixture struct {
		Compatibility       json.RawMessage `json:"compatibility"`
		LifecycleProjection json.RawMessage `json:"lifecycle_projection"`
		OperatorProjection  json.RawMessage `json:"operator_projection"`
		OperationProjection json.RawMessage `json:"operation_projection"`
		YardProjection      json.RawMessage `json:"yard_projection"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(grammar.ContextReadout{
		Schema:      grammar.ContextReadSchema,
		State:       grammar.ContextReadHold,
		Audience:    grammar.ContextReadAudience,
		ReasonCodes: []string{grammar.ContextReadReasonCanonUnverified, "producer_receipt_missing"},
		Projection:  &fixture.OperatorProjection,
	})
	if err != nil {
		t.Fatal(err)
	}
	readout, err := fetchContextCarrierBody(t, body)
	if err != nil {
		t.Fatal(err)
	}
	index, err := readout.ProjectionIndexAt(time.Date(2026, 7, 10, 17, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Events) != 16 || len(index.Actions) != 2 ||
		index.Actions[0].Operation != "source_mutation" ||
		index.Actions[1].Operation != "context.inspect" {
		t.Fatalf("strict current index is incomplete: events=%d actions=%+v", len(index.Events), index.Actions)
	}
	folded := model.New("REINS").FoldContext(readout, "")
	wiped := folded.SetAIR(true)
	assertContextCarrierBytesAbsent(t, wiped.ContextReadout)
}

func readContextCarrierArtifact(
	t *testing.T,
	root string,
	relative string,
	wantSHA string,
	wantBytes int,
	maxBytes int,
) []byte {
	t.Helper()
	clean := filepath.Clean(relative)
	if relative == "" || filepath.IsAbs(relative) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		t.Fatalf("unsafe artifact path %q", relative)
	}
	if wantBytes < 0 || wantBytes > maxBytes {
		t.Fatalf("%s declared byte count %d exceeds bound %d", relative, wantBytes, maxBytes)
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(root, clean)
	linkInfo, err := os.Lstat(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 || !linkInfo.Mode().IsRegular() {
		t.Fatalf("%s is not a regular non-symlink artifact", relative)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		t.Fatal(err)
	}
	resolvedRelative, err := filepath.Rel(rootResolved, resolved)
	if err != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) {
		t.Fatalf("%s resolves outside the artifact root", relative)
	}
	file, err := os.Open(resolved)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Size() != int64(wantBytes) || info.Size() > int64(maxBytes) {
		t.Fatalf("%s size/mode changed before bounded read", relative)
	}
	payload, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != wantBytes || len(payload) > maxBytes {
		t.Fatalf("%s byte count = %d, want %d", relative, len(payload), wantBytes)
	}
	observed := fmt.Sprintf("%x", sha256.Sum256(payload))
	if observed != wantSHA {
		t.Fatalf("%s SHA-256 = %s, want %s", relative, observed, wantSHA)
	}
	return payload
}

func fetchContextCarrierBody(t *testing.T, body []byte) (grammar.ContextReadout, error) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/read/context" || r.URL.RawQuery != "" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(body)
	}))
	defer server.Close()
	return api.FetchContext(server.URL)
}

func assertContextCarrierReadout(
	t *testing.T,
	readout grammar.ContextReadout,
	testCase contextCarrierCase,
	body []byte,
	producer []byte,
) {
	t.Helper()
	if readout.Schema != grammar.ContextReadSchema || readout.Audience != grammar.ContextReadAudience {
		t.Fatalf("wrong schema/audience: %+v", readout)
	}
	if readout.State == grammar.ContextReadPresent || string(readout.State) != testCase.ExpectedState {
		t.Fatalf("state = %q, want %q", readout.State, testCase.ExpectedState)
	}
	if !slices.Equal(readout.ReasonCodes, testCase.ExpectedReasonCodes) {
		t.Fatalf("reasons = %v, want %v", readout.ReasonCodes, testCase.ExpectedReasonCodes)
	}
	if !bytes.Equal(readout.RawEnvelope, body) {
		t.Fatal("Go RawEnvelope differs from exact FastAPI response.content")
	}
	switch testCase.ActiveField {
	case "":
		if readout.Projection != nil || readout.Compatibility != nil {
			t.Fatal("DARK readout retained a nested payload")
		}
	case "projection":
		if readout.Projection == nil || readout.Compatibility != nil || !bytes.Equal(*readout.Projection, producer) {
			t.Fatal("Go projection RawMessage differs from accepted producer bytes")
		}
	case "compatibility":
		if readout.Projection != nil || readout.Compatibility == nil || !bytes.Equal(*readout.Compatibility, producer) {
			t.Fatal("Go compatibility RawMessage differs from accepted producer bytes")
		}
	default:
		t.Fatalf("unknown active field %q", testCase.ActiveField)
	}
}

func assertFoldedContextCarrier(
	t *testing.T,
	folded model.Model,
	testCase contextCarrierCase,
	body []byte,
	producer []byte,
) {
	t.Helper()
	readout := folded.ContextReadout
	if readout.Schema != grammar.ContextReadSchema || readout.Audience != grammar.ContextReadAudience {
		t.Fatalf("model changed schema/audience: %+v", readout)
	}
	if string(readout.State) != testCase.ExpectedState {
		t.Fatalf("model state = %q, want %q", readout.State, testCase.ExpectedState)
	}
	if !slices.Equal(readout.ReasonCodes, testCase.ExpectedReasonCodes) {
		t.Fatalf("model reasons = %v, want %v", readout.ReasonCodes, testCase.ExpectedReasonCodes)
	}
	if testCase.ExpectedState != string(grammar.ContextReadHold) {
		assertContextCarrierBytesAbsent(t, readout)
		return
	}
	if !bytes.Equal(readout.RawEnvelope, body) {
		t.Fatal("model did not retain the exact HOLD envelope")
	}
	if testCase.ActiveField == "projection" {
		if readout.Projection == nil || !bytes.Equal(*readout.Projection, producer) {
			t.Fatal("model did not retain the exact projection bytes")
		}
	} else if readout.Compatibility == nil || !bytes.Equal(*readout.Compatibility, producer) {
		t.Fatal("model did not retain the exact compatibility bytes")
	}
}

func assertContextCarrierBytesAbsent(t *testing.T, readout grammar.ContextReadout) {
	t.Helper()
	if readout.Projection != nil || readout.Compatibility != nil || len(readout.RawEnvelope) != 0 {
		t.Fatalf("private context bytes remain resident: %+v", readout)
	}
}
