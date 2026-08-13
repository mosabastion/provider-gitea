package clients

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A branch-protection rule name is a GLOB, and a glob for an environment branch
// layout contains a slash: `stage/*`, `release/*`. That name is interpolated
// into a URL path segment, so an unescaped slash silently becomes a path
// SEPARATOR and the request lands on a route that does not exist.
//
// The failure is not a 500 — it is a 404, which the client maps to "not found"
// and the controller reads as "the resource does not exist". So Observe reports
// absent for a rule it created itself, Create runs again, and the API rejects it
// with "Branch protection already exist". Forever.
//
// Seen on a live platform: `external-create-succeeded` and
// `external-create-failed` annotations four hours apart on the same object, the
// rule present and working in Gitea the whole time. Confirmed against a real
// Forgejo before this fix: raw `stage/*` → 404, escaped `stage%2F*` → 200.
//
// These tests assert the REQUEST PATH rather than a returned value, because the
// path is the defect — a test that only checked the parsed response would pass
// against the broken client if the server were lenient about routing.

const escapedRulePath = "/api/v1/repos/testorg/testrepo/branch_protections/stage%2F%2A"

// rawPath reports the request target as the server received it on the wire,
// before Go's decoding — `r.URL.Path` would show the *decoded* form and hide
// exactly the bug under test.
func rawPath(r *http.Request) string {
	if r.URL.EscapedPath() != "" {
		return r.URL.EscapedPath()
	}
	return r.URL.Path
}

func TestGetBranchProtectionEscapesRuleNameWithSlash(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = rawPath(r)
		if got != escapedRulePath {
			// Mirror the real server: an unescaped slash routes elsewhere.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"rule_name":"stage/*","enable_push":true}`))
	}))
	defer server.Close()

	c := &giteaClient{httpClient: &http.Client{}, baseURL: server.URL + "/api/v1", token: "t"}
	bp, err := c.GetBranchProtection(context.Background(), "testorg/testrepo", "stage/*")
	if err != nil {
		t.Fatalf("GetBranchProtection: %v (requested %q)", err, got)
	}
	if got != escapedRulePath {
		t.Errorf("request path = %q, want %q", got, escapedRulePath)
	}
	if bp.RuleName != "stage/*" {
		t.Errorf("rule name = %q, want %q", bp.RuleName, "stage/*")
	}
}

func TestUpdateBranchProtectionEscapesRuleNameWithSlash(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = rawPath(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"rule_name":"stage/*"}`))
	}))
	defer server.Close()

	c := &giteaClient{httpClient: &http.Client{}, baseURL: server.URL + "/api/v1", token: "t"}
	if _, err := c.UpdateBranchProtection(
		context.Background(), "testorg/testrepo", "stage/*", &UpdateBranchProtectionRequest{},
	); err != nil {
		t.Fatalf("UpdateBranchProtection: %v", err)
	}
	if got != escapedRulePath {
		t.Errorf("request path = %q, want %q", got, escapedRulePath)
	}
}

func TestDeleteBranchProtectionEscapesRuleNameWithSlash(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = rawPath(r)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := &giteaClient{httpClient: &http.Client{}, baseURL: server.URL + "/api/v1", token: "t"}
	if err := c.DeleteBranchProtection(context.Background(), "testorg/testrepo", "stage/*"); err != nil {
		t.Fatalf("DeleteBranchProtection: %v", err)
	}
	if got != escapedRulePath {
		t.Errorf("request path = %q, want %q", got, escapedRulePath)
	}
}

// A name with no special characters must be unchanged — escaping is not allowed
// to alter the common case.
func TestGetBranchProtectionLeavesPlainNameUnescaped(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = rawPath(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"rule_name":"main"}`))
	}))
	defer server.Close()

	c := &giteaClient{httpClient: &http.Client{}, baseURL: server.URL + "/api/v1", token: "t"}
	if _, err := c.GetBranchProtection(context.Background(), "testorg/testrepo", "main"); err != nil {
		t.Fatalf("GetBranchProtection: %v", err)
	}
	if want := "/api/v1/repos/testorg/testrepo/branch_protections/main"; got != want {
		t.Errorf("request path = %q, want %q", got, want)
	}
}
