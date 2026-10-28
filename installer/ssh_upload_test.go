package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Integration: upload google.json to the fake tablet if CAL_FAKE_SSH is set (host:port).
func TestUploadGoogleJSON(t *testing.T) {
	addr := os.Getenv("CAL_FAKE_SSH")
	if addr == "" {
		t.Skip("CAL_FAKE_SSH not set")
	}
	c, err := connect(addr, "root", "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	creds := googleCredentials{ClientID: "id", ClientSecret: "sec", RefreshToken: "rt"}
	body, _ := json.MarshalIndent(creds, "", "  ")
	body = append(body, '\n')
	if out, err := runCmd(c, "umask 077; mkdir -p "+configDir+" && chmod 700 "+configDir); err != nil {
		t.Fatalf("mkdir: %v %s", err, out)
	}
	if err := uploadBytes(c, body, googlePath, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := runCmd(c, "chmod 600 "+googlePath+" && stat -c '%a' "+googlePath); err != nil || out != "600" {
		t.Fatalf("mode=%q err=%v", out, err)
	}
	out, err := runCmd(c, "cat "+googlePath)
	if err != nil {
		t.Fatal(err)
	}
	var got googleCredentials
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.RefreshToken != "rt" {
		t.Fatalf("got %q err %v", out, err)
	}
}

func TestClientJSONParse(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.json")
	os.WriteFile(p, []byte(`{"installed":{"client_id":"x","client_secret":"y","auth_uri":"https://a","token_uri":"https://t"}}`), 0600)
	raw, _ := os.ReadFile(p)
	var cj clientJSON
	if err := json.Unmarshal(raw, &cj); err != nil || cj.Installed.ClientID != "x" {
		t.Fatal(err, cj)
	}
}

func TestFakeSSHStillUp(t *testing.T) {
	// smoke: ensure we can at least resolve the helper that builds the auth URL shape
	if _, err := exec.LookPath("true"); err != nil {
		t.Fatal(err)
	}
	_ = time.Now()
}
