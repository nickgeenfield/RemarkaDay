package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSyncWindow(t *testing.T) {
	start, end := syncWindow(time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC))
	if start != "2026-10-01" || end != "2027-05-01" {
		t.Fatalf("got %s .. %s", start, end)
	}
}

func TestRedactSecrets(t *testing.T) {
	if redactSecrets("all fine") != "all fine" {
		t.Fatal("clean text changed")
	}
	if !strings.Contains(redactSecrets("refresh_token=abc"), "omitted") {
		t.Fatal("secret not redacted")
	}
}

func TestRunOAuthSuccess(t *testing.T) {
	var gotForm url.Values
	tokSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(b))
		_ = json.NewEncoder(w).Encode(map[string]string{
			"refresh_token": "rt-test-value",
			"scope":         oauthScope,
			"access_token":  "at-should-not-be-stored",
			"token_type":    "Bearer",
		})
	}))
	defer tokSrv.Close()

	oldClient, oldBrowser := httpClient, openBrowserFn
	httpClient = tokSrv.Client()
	defer func() { httpClient, openBrowserFn = oldClient, oldBrowser }()

	openBrowserFn = func(u string) error {
		go func() {
			time.Sleep(50 * time.Millisecond)
			pu, err := url.Parse(u)
			if err != nil {
				t.Error(err)
				return
			}
			cb := pu.Query().Get("redirect_uri") + "?state=" + url.QueryEscape(pu.Query().Get("state")) + "&code=auth-code-xyz"
			resp, err := http.Get(cb)
			if err != nil {
				t.Error(err)
				return
			}
			resp.Body.Close()
		}()
		return nil
	}

	creds, err := runOAuth(desktopClient{
		ClientID:     "cid",
		ClientSecret: "csecret",
		TokenURI:     tokSrv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if creds.ClientID != "cid" || creds.ClientSecret != "csecret" || creds.RefreshToken != "rt-test-value" {
		t.Fatalf("bad creds: %+v", creds)
	}
	if gotForm.Get("code") != "auth-code-xyz" || gotForm.Get("grant_type") != "authorization_code" {
		t.Fatalf("bad form: %v", gotForm)
	}
	if gotForm.Get("code_verifier") == "" {
		t.Fatal("missing PKCE verifier")
	}
}

func TestRunOAuthDenied(t *testing.T) {
	oldBrowser := openBrowserFn
	defer func() { openBrowserFn = oldBrowser }()
	openBrowserFn = func(u string) error {
		go func() {
			time.Sleep(50 * time.Millisecond)
			pu, _ := url.Parse(u)
			cb := pu.Query().Get("redirect_uri") + "?state=" + url.QueryEscape(pu.Query().Get("state")) + "&error=access_denied"
			resp, _ := http.Get(cb)
			if resp != nil {
				resp.Body.Close()
			}
		}()
		return nil
	}
	_, err := runOAuth(desktopClient{ClientID: "c", ClientSecret: "s"})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("expected denied, got %v", err)
	}
}

func TestGoogleJSONShape(t *testing.T) {
	b, _ := json.Marshal(googleCredentials{ClientID: "a", ClientSecret: "b", RefreshToken: "c"})
	var m map[string]string
	_ = json.Unmarshal(b, &m)
	if len(m) != 3 || m["client_id"] != "a" || m["refresh_token"] != "c" {
		t.Fatalf("%v", m)
	}
}
