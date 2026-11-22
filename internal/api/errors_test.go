package api

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"civitai-cli/internal/testutil"
)

func TestClassifyHTTPError_StringForm(t *testing.T) {
	body := testutil.LoadFixture(t, "err_not_found.json")
	err := classifyHTTPError(404, body)
	var nfe *NotFoundError
	if !errors.As(err, &nfe) {
		t.Fatalf("expected NotFoundError, got %T: %v", err, err)
	}
	if nfe.Detail != "Model not found" {
		t.Errorf("detail = %q, want %q", nfe.Detail, "Model not found")
	}
}

func TestClassifyHTTPError_Unauthorized(t *testing.T) {
	body := testutil.LoadFixture(t, "err_unauthorized.json")
	err := classifyHTTPError(401, body)
	var ue *UnauthorizedError
	if !errors.As(err, &ue) {
		t.Fatalf("expected UnauthorizedError, got %T: %v", err, err)
	}
	if ue.Detail != "Unauthorized" {
		t.Errorf("detail = %q, want %q", ue.Detail, "Unauthorized")
	}
}

func TestClassifyHTTPError_ZodTooBig(t *testing.T) {
	body := testutil.LoadFixture(t, "err_limit_too_big.json")
	err := classifyHTTPError(400, body)
	var ae *ApiError
	if !errors.As(err, &ae) {
		t.Fatalf("expected ApiError, got %T: %v", err, err)
	}
	if ae.Status != 400 {
		t.Errorf("status = %d, want 400", ae.Status)
	}
	if !strings.Contains(ae.Detail, "Too big") || !strings.Contains(ae.Detail, "limit") {
		t.Errorf("detail missing Zod summary: %q", ae.Detail)
	}
}

func TestErrorDetail_Shapes(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"string form", `{"error":"Model not found"}`, "Model not found"},
		{"plain message form", `{"message":"Recently Added sort requires a collectionId"}`, "Recently Added sort requires a collectionId"},
		{"zod invalid value with values only", `{"error":{"name":"ZodError","message":"[{\"code\":\"invalid_value\",\"values\":[\"Hot\",\"Newest\"],\"path\":[\"sort\"]}]"}}`, "expected one of: Hot, Newest; at sort"},
		{"non-json html", `<html><head><title>502 Bad Gateway</title></head></html>`, `<html><head><title>502 Bad Gateway</title></head></html>`},
		{"empty body", ``, ``},
		{"unknown error object", `{"error":{"code":42}}`, `{"code":42}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := errorDetail([]byte(tc.body))
			if got != tc.want {
				t.Errorf("errorDetail = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestErrorDetail_ZodUnionDescends(t *testing.T) {
	// The live types probe is an invalid_union whose concrete enum leak
	// lives in errors[0][0]; the summary must surface it, not "Invalid input".
	body := testutil.LoadFixture(t, "probes/enum_types.json")
	detail := errorDetail(body)
	if !strings.Contains(detail, "LORA") || !strings.Contains(detail, "types") {
		t.Errorf("union summary lost the enum leak or path: %q", detail)
	}
	if strings.Contains(detail, "Invalid input;") {
		t.Errorf("summary should descend past the useless outer message: %q", detail)
	}
}

func TestErrorDetail_TruncatesLongBodies(t *testing.T) {
	long := strings.Repeat("x", 5000)
	got := errorDetail([]byte(long))
	if len(got) > maxDetailBytes+3 {
		t.Errorf("detail not truncated: len = %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncated detail missing ellipsis marker")
	}
}

func TestErrorDetail_TruncationKeepsUTF8(t *testing.T) {
	// A 3-byte rune straddling the 300-byte cut must be trimmed back,
	// not split into invalid UTF-8 terminal output.
	long := strings.Repeat("a", 299) + strings.Repeat("漢", 100)
	got := errorDetail([]byte(long))
	if !utf8.ValidString(got) {
		t.Errorf("truncated detail is not valid UTF-8")
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncated detail missing ellipsis marker")
	}
}

func TestNetworkError_Unwrap(t *testing.T) {
	cause := errors.New("boom")
	ne := &NetworkError{Attempts: 4, Err: cause}
	if !errors.Is(ne, cause) {
		t.Errorf("NetworkError must unwrap to its cause")
	}
	if !strings.Contains(ne.Error(), "4 attempt") {
		t.Errorf("message missing attempt count: %q", ne.Error())
	}
}
