package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBookingOpsActorRoles(t *testing.T) {
	auth, err := newBookingAuth("merchant-token", "second-merchant-token",
		bookingActorCredential{"demo-merchant-a", "alice", bookingRoleAnalyst, "alice-token"},
		bookingActorCredential{"demo-merchant-b", "bob", bookingRoleViewer, "bob-token"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		token    string
		role     bookingRole
		want     bookingActor
		response int
	}{
		{"merchant-token", bookingRoleMerchant, bookingActor{"demo-merchant-a", "merchant", bookingRoleMerchant}, http.StatusOK},
		{"alice-token", bookingRoleAnalyst, bookingActor{"demo-merchant-a", "alice", bookingRoleAnalyst}, http.StatusOK},
		{"bob-token", bookingRoleViewer, bookingActor{"demo-merchant-b", "bob", bookingRoleViewer}, http.StatusOK},
		{"bob-token", bookingRoleAnalyst, bookingActor{}, http.StatusForbidden},
		{"alice-token", bookingRoleMerchant, bookingActor{}, http.StatusForbidden},
		{"invalid", bookingRoleAnalyst, bookingActor{}, http.StatusUnauthorized},
	} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Authorization", "Bearer "+test.token)
		response := httptest.NewRecorder()
		if actor := auth.requireBookingActor(response, request, test.role); actor != test.want || response.Code != test.response {
			t.Fatalf("role %q: got actor %+v status %d; want %+v status %d", test.role, actor, response.Code, test.want, test.response)
		}
	}
	if _, err := newBookingAuth("merchant-token", "",
		bookingActorCredential{"demo-merchant-b", "bob", bookingRoleViewer, "merchant-token"}); err == nil {
		t.Fatal("a reused bearer token must be rejected across tenants and roles")
	}
}

func TestSyntheticBookingContact(t *testing.T) {
	secret := []byte("test-key")
	eventID := "498ab33a-c10d-45f9-8db9-7715f2e0f8b4"
	first := syntheticBookingEmail(secret, "demo-merchant-a", eventID)
	if !strings.HasSuffix(first, "@checkout.test") || syntheticBookingEmail(secret, "demo-merchant-a", eventID) != first ||
		syntheticBookingEmail(secret, "demo-merchant-b", eventID) == first {
		t.Fatal("synthetic contact must be stable, tenant-scoped and use the reserved domain")
	}
}
