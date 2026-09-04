package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCoreAuthenticatorLoadsProfileAndPermissions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-JMS-ORG") != "org-1" || request.Header.Get("Authorization") != "Bearer test" {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/users/profile/":
			_, _ = response.Write([]byte(`{"id":"user-1","is_active":true}`))
		case "/api/v1/users/profile/permissions/":
			_, _ = response.Write([]byte(`{"id":"user-1","perms":["assets.view_asset"]}`))
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	request := httptest.NewRequest(http.MethodGet, "/kael/api/v1/bootstrap", nil)
	request.Header.Set("X-JMS-ORG", "org-1")
	request.Header.Set("Authorization", "Bearer test")
	principal, err := NewCoreAuthenticator(server.URL, true, time.Second).Authenticate(context.Background(), request)
	if err != nil || principal.SubjectID != "user-1" || len(principal.Permissions) != 1 || principal.Permissions[0] != "assets.view_asset" {
		t.Fatalf("unexpected authentication result: principal=%+v err=%v", principal, err)
	}
}

func TestVerifyCSRFSupportsJumpServerCookiePrefix(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/kael/api/v1/conversations", nil)
	request.AddCookie(&http.Cookie{Name: "jms_csrftoken", Value: "expected"})
	request.Header.Set("X-CSRFToken", "expected")
	if err := VerifyCSRF(request); err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-CSRFToken", "wrong")
	if err := VerifyCSRF(request); err != ErrCSRFRejected {
		t.Fatalf("expected CSRF rejection, got %v", err)
	}
}
