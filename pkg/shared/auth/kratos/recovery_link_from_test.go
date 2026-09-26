package kratos

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"ctoup.com/coreapp/pkg/shared/auth"
)

// The sign-up magic link must land the new user back where they started (a
// course page), yet still show the password form — so the destination rides
// as `from`, never as `return_to`, which switches the recovery page into the
// invitation "continue" gate.
func TestPasswordResetLinkCarriesFrom(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/admin/identities"):
			_, _ = w.Write([]byte(`[{"id":"3c66043e-4d4e-4009-b73f-2d4d483746af","schema_id":"default","schema_url":"","traits":{}}]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/admin/recovery/link"):
			_, _ = w.Write([]byte(`{"recovery_link":"http://kratos:4433/self-service/recovery?flow=F1&token=T1"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	client := newTestClient(t, h)

	for _, tc := range []struct {
		name     string
		from     string
		wantFrom string
	}{
		{"with from", "/lms/courses/intro?x=1", "/lms/courses/intro?x=1"},
		{"without from", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			link, err := client.PasswordResetLinkWithSettings(context.Background(), "a@b.example",
				&auth.ActionCodeSettings{URL: "http://corpa.ctoup.localhost:5173/signin", From: tc.from})
			if err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(link)
			if err != nil {
				t.Fatal(err)
			}
			if got := u.Scheme + "://" + u.Host + u.Path; got != "http://corpa.ctoup.localhost:5173/recovery" {
				t.Errorf("link base = %q", got)
			}
			q := u.Query()
			if q.Get("flow") != "F1" || q.Get("token") != "T1" {
				t.Errorf("flow/token lost: %q", link)
			}
			if q.Get("from") != tc.wantFrom {
				t.Errorf("from = %q, want %q (link %q)", q.Get("from"), tc.wantFrom, link)
			}
			if q.Has("return_to") {
				t.Errorf("from must not ride as return_to: %q", link)
			}
		})
	}
}
