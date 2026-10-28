// CalendarInstaller installs/updates/removes nguyer's reMarkable Calendar (AppLoad)
// and can run the one-time Google OAuth desktop login for the tablet.
package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

const (
	version    = "0.8.0"
	appName    = "reCalendar"
	appID      = "remarkable-calendar" // folder under AppLoad
	defaultIP  = "10.11.99.1"
	defaultUsr = "root"
	apploadDir = "/home/root/xovi/exthome/appload"
	appDir     = apploadDir + "/" + appID
	configDir  = "/home/root/.config/remarkable-calendar"
	dataDir    = "/home/root/.local/share/remarkable-calendar"
	tmpDir     = "/home/root/.calendar-installer-tmp"
	entryPath  = appDir + "/backend/entry"
	googlePath = configDir + "/google.json"
	oauthScope = "https://www.googleapis.com/auth/calendar.readonly https://www.googleapis.com/auth/calendar.events"
	defaultTZ  = "Europe/London"
)

//go:embed payload
var payload embed.FS

type payloadFile struct {
	src  string
	dest string
	mode os.FileMode
}

var files = []payloadFile{
	{"payload/manifest.json", "manifest.json", 0644},
	{"payload/resources.rcc", "resources.rcc", 0644},
	{"payload/icon.png", "icon.png", 0644},
	{"payload/backend/entry", "backend/entry", 0755},
}

var in = bufio.NewReader(os.Stdin)

// openBrowserFn is swapped in tests.
var openBrowserFn = openBrowser

// httpClient is swapped in tests for the token exchange.
var httpClient = http.DefaultClient

func main() {
	code := run()
	fmt.Println()
	prompt("Press Enter to exit.")
	os.Exit(code)
}

func run() int {
	fmt.Printf("%s installer, version %s\n", appName, version)
	fmt.Println(strings.Repeat("-", 40))
	fmt.Println("This installs, updates or removes the reCalendar app on a reMarkable")
	fmt.Println("tablet that already has XOVI and AppLoad. The app is")
	fmt.Println("https://github.com/nguyer/remarkable-calendar by nguyer.")
	fmt.Println()
	fmt.Println("Before you start:")
	fmt.Println("  - Connect the tablet to this computer with its USB cable, switch it on and unlock it.")
	fmt.Println("  - Close reCalendar on the tablet if it is open.")
	fmt.Println("  - You need the tablet's SSH password. On the tablet go to")
	fmt.Println("    Settings > General > Help > About > Copyrights and licences and scroll down;")
	fmt.Println("    the password is shown there (developer mode is needed for SSH).")
	fmt.Println()

	host := ask("Tablet IP address", defaultIP)
	user := ask("Username", defaultUsr)
	pass := askPassword("Password (it won't show as you type): ")
	fmt.Println()

	fmt.Printf("Connecting to %s@%s ...\n", user, host)
	client, err := connect(host, user, pass)
	if err != nil {
		fmt.Println()
		fmt.Println(friendlyConnError(err, host))
		return 1
	}
	defer client.Close()
	fmt.Println("Connected.")

	if err := checkAppLoad(client); err != nil {
		fmt.Println()
		fmt.Println(err)
		return 1
	}

	for {
		fmt.Println()
		fmt.Println("What would you like to do?")
		fmt.Println("  1  Install or update reCalendar")
		fmt.Println("  2  Set up Google sync")
		fmt.Println("  3  Uninstall reCalendar")
		fmt.Println("  4  Quit")
		switch strings.TrimSpace(prompt("Choose 1, 2, 3 or 4: ")) {
		case "1":
			if err := install(client); err != nil {
				fmt.Println()
				fmt.Println("Installation failed:", err)
				fmt.Println("Your calendar data and Google credentials have not been touched.")
				return 1
			}
			return 0
		case "2":
			if err := googleSetup(client); err != nil {
				fmt.Println()
				fmt.Println("Google setup failed:", err)
				return 1
			}
			return 0
		case "3":
			if err := uninstall(client); err != nil {
				fmt.Println()
				fmt.Println("Uninstall failed:", err)
				return 1
			}
			return 0
		case "4", "q", "Q":
			fmt.Println("Nothing was changed.")
			return 0
		default:
			fmt.Println("Please type 1, 2, 3 or 4.")
		}
	}
}

// ---------- prompts ----------

func prompt(msg string) string {
	fmt.Print(msg)
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return ""
	}
	return strings.TrimRight(line, "\r\n")
}

func ask(label, def string) string {
	v := strings.TrimSpace(prompt(fmt.Sprintf("%s [%s]: ", label, def)))
	if v == "" {
		return def
	}
	return v
}

func askPassword(label string) string {
	fmt.Print(label)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Println()
		if err == nil {
			return string(b)
		}
	}
	return prompt("")
}

func yesNo(q string, def bool) bool {
	d := "y/N"
	if def {
		d = "Y/n"
	}
	for {
		a := strings.ToLower(strings.TrimSpace(prompt(fmt.Sprintf("%s [%s]: ", q, d))))
		switch a {
		case "":
			return def
		case "y", "yes":
			return true
		case "n", "no":
			return false
		}
		fmt.Println("Please type y or n.")
	}
}

// ---------- SSH ----------

func sshConfig(user, pass string) *ssh.ClientConfig {
	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
			ssh.KeyboardInteractive(func(_, _ string, qs []string, _ []bool) ([]string, error) {
				ans := make([]string, len(qs))
				for i := range ans {
					ans[i] = pass
				}
				return ans, nil
			}),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	sup, insec := ssh.SupportedAlgorithms(), ssh.InsecureAlgorithms()
	cfg.KeyExchanges = append(append([]string{}, sup.KeyExchanges...), insec.KeyExchanges...)
	cfg.Ciphers = append(append([]string{}, sup.Ciphers...), insec.Ciphers...)
	cfg.MACs = append(append([]string{}, sup.MACs...), insec.MACs...)
	cfg.HostKeyAlgorithms = append(append([]string{}, sup.HostKeys...), insec.HostKeys...)
	return cfg
}

func connect(host, user, pass string) (*ssh.Client, error) {
	addr := host
	if _, _, err := net.SplitHostPort(host); err != nil {
		addr = net.JoinHostPort(host, "22")
	}
	return ssh.Dial("tcp", addr, sshConfig(user, pass))
}

func friendlyConnError(err error, host string) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "unable to authenticate") || strings.Contains(msg, "no supported methods remain"):
		return "The tablet rejected the password (or username).\n" +
			"Check the password on the tablet under Settings > General > Help > About >\n" +
			"Copyrights and licences. It is case-sensitive, and it can change after a software update.\n" +
			"The username is normally root."
	case strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "no route to host") ||
		strings.Contains(msg, "network is unreachable") || strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connectex") || strings.Contains(msg, "timed out"):
		return fmt.Sprintf("Couldn't reach the tablet at %s.\n"+
			"  - Check the USB cable is plugged in at both ends and the tablet is switched on and unlocked.\n"+
			"  - Over USB the address is normally 10.11.99.1.\n"+
			"  - If you're using Wi-Fi instead, make sure SSH over Wi-Fi is enabled and the address is right.\n"+
			"(Technical detail: %v)", host, err)
	case strings.Contains(msg, "no such host"):
		return fmt.Sprintf("'%s' isn't a valid address. Over USB the tablet is normally 10.11.99.1.", host)
	}
	return fmt.Sprintf("Couldn't connect to the tablet: %v", err)
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func runCmd(c *ssh.Client, cmd string) (string, error) {
	s, err := c.NewSession()
	if err != nil {
		return "", err
	}
	defer s.Close()
	var buf lockedBuffer
	s.Stdout = &buf
	s.Stderr = &buf
	err = s.Run(cmd)
	return strings.TrimSpace(buf.String()), err
}

func checkAppLoad(c *ssh.Client) error {
	out, err := runCmd(c, "if [ ! -d /home/root/xovi ]; then echo NOXOVI; elif [ ! -d "+apploadDir+" ]; then echo NOAPPLOAD; else echo OK; fi")
	if err != nil {
		return fmt.Errorf("couldn't run a check on the tablet: %v %s", err, out)
	}
	switch out {
	case "OK":
		return nil
	case "NOXOVI":
		return errors.New("XOVI isn't installed on this tablet (/home/root/xovi wasn't found).\n" +
			"reCalendar needs XOVI and the AppLoad extension. Install those first, then run this again.\n" +
			"Nothing was changed.")
	default:
		return errors.New("XOVI is installed, but AppLoad isn't (" + apploadDir + " wasn't found).\n" +
			"reCalendar runs inside AppLoad. Install the AppLoad extension for XOVI first, then run this again.\n" +
			"Nothing was changed.")
	}
}

// ---------- install ----------

func install(c *ssh.Client) error {
	fmt.Println()
	fmt.Println("[1/4] Closing Calendar's helper if it's running and preparing folders...")
	// Preserve existing DB/config; only touch the AppLoad app folder.
	if out, err := runCmd(c, "pkill -f 'remarkable-calendar/backend/[e]ntry'; rm -rf "+tmpDir+" && mkdir -p "+
		tmpDir+"/backend "+appDir+"/backend"); err != nil {
		return fmt.Errorf("couldn't create folders: %v %s", err, out)
	}

	fmt.Println("[2/4] Copying files to the tablet...")
	if err := uploadApp(c); err != nil {
		runCmd(c, "rm -rf "+tmpDir)
		return err
	}

	fmt.Println("[3/4] Moving files into place...")
	// Match deploy.sh: replace entry via a .new file so a half-written binary is never left behind.
	cmd := fmt.Sprintf(
		"mv -f %s/manifest.json %s/manifest.json && "+
			"mv -f %s/resources.rcc %s/resources.rcc && "+
			"mv -f %s/icon.png %s/icon.png && "+
			"mv -f %s/backend/entry %s/backend/entry.new && "+
			"chmod 755 %s/backend/entry.new && "+
			"mv -f %s/backend/entry.new %s/backend/entry && "+
			"rm -rf %s",
		tmpDir, appDir, tmpDir, appDir, tmpDir, appDir, tmpDir, appDir, appDir, appDir, appDir, tmpDir)
	if out, err := runCmd(c, cmd); err != nil {
		return fmt.Errorf("couldn't move the files into place: %v %s", err, out)
	}

	fmt.Println("[4/4] Checking the helper program runs on this tablet...")
	selftest := "rm -f /tmp/.rmcal-selftest.sqlite3 && " +
		"REMARKABLE_CALENDAR_DB=/tmp/.rmcal-selftest.sqlite3 " + entryPath + " 2000-01-01 >/dev/null && " +
		"rm -f /tmp/.rmcal-selftest.sqlite3"
	if out, err := runCmd(c, selftest); err != nil {
		return fmt.Errorf("the helper's self-test failed: %v %s", err, out)
	}

	fmt.Println()
	fmt.Printf("Success! %s %s is installed.\n", appName, version)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. On the tablet open AppLoad and tap Reload (the project's README says a full")
	fmt.Println("     UI restart isn't needed). If reCalendar still doesn't appear, restart the UI")
	fmt.Println("     with mods in reManager.")
	fmt.Println("  2. For Google sync, keep reCalendar closed and choose option 2 in this installer")
	fmt.Println("     (or run it again later). Local-only events work without Google.")
	fmt.Println("Your event database and any existing Google credentials were left alone.")
	return nil
}

func uploadApp(c *ssh.Client) error {
	sc, err := sftp.NewClient(c)
	if err == nil {
		defer sc.Close()
		for _, f := range files {
			if err := sftpPut(sc, f.src, path.Join(tmpDir, f.dest), f.mode); err != nil {
				return fmt.Errorf("copying %s failed: %v", f.dest, err)
			}
			fmt.Println("      copied", f.dest)
		}
		return nil
	}
	for _, f := range files {
		if err := catPut(c, f.src, path.Join(tmpDir, f.dest), f.mode); err != nil {
			return fmt.Errorf("copying %s failed: %v", f.dest, err)
		}
		fmt.Println("      copied", f.dest)
	}
	return nil
}

func sftpPut(sc *sftp.Client, src, dst string, mode os.FileMode) error {
	data, err := fs.ReadFile(payload, src)
	if err != nil {
		return err
	}
	return sftpPutBytes(sc, data, dst, mode)
}

func sftpPutBytes(sc *sftp.Client, data []byte, dst string, mode os.FileMode) error {
	if err := sc.MkdirAll(path.Dir(dst)); err != nil {
		return err
	}
	w, err := sc.Create(dst)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	if err := sc.Chmod(dst, mode); err != nil {
		return err
	}
	st, err := sc.Stat(dst)
	if err != nil {
		return err
	}
	if st.Size() != int64(len(data)) {
		return fmt.Errorf("size mismatch (%d of %d bytes)", st.Size(), len(data))
	}
	return nil
}

func catPut(c *ssh.Client, src, dst string, mode os.FileMode) error {
	data, err := fs.ReadFile(payload, src)
	if err != nil {
		return err
	}
	return catPutBytes(c, data, dst, mode)
}

func catPutBytes(c *ssh.Client, data []byte, dst string, mode os.FileMode) error {
	s, err := c.NewSession()
	if err != nil {
		return err
	}
	defer s.Close()
	s.Stdin = bytes.NewReader(data)
	var out lockedBuffer
	s.Stdout, s.Stderr = &out, &out
	cmd := fmt.Sprintf("mkdir -p '%s' && cat > '%s' && chmod %o '%s' && wc -c < '%s'",
		path.Dir(dst), dst, mode, dst, dst)
	if err := s.Run(cmd); err != nil {
		return fmt.Errorf("%v %s", err, out.String())
	}
	if strings.TrimSpace(out.String()) != fmt.Sprint(len(data)) {
		return fmt.Errorf("size mismatch (%s of %d bytes)", strings.TrimSpace(out.String()), len(data))
	}
	return nil
}

// ---------- uninstall ----------

func uninstall(c *ssh.Client) error {
	fmt.Println()
	if !yesNo("Remove the reCalendar app from this tablet?", false) {
		fmt.Println("Nothing was changed.")
		return nil
	}
	wipeData := yesNo("Also delete calendar data (Google credentials in "+configDir+" and the event database in "+dataDir+")?", false)

	cmd := "pkill -f 'remarkable-calendar/backend/[e]ntry'; rm -rf " + appDir + " " + tmpDir
	if wipeData {
		cmd += " " + configDir + " " + dataDir
	}
	if out, err := runCmd(c, cmd); err != nil {
		return fmt.Errorf("%v %s", err, out)
	}
	fmt.Println()
	fmt.Println("reCalendar has been removed.")
	if wipeData {
		fmt.Println("Google credentials and the event database were deleted.")
	} else {
		fmt.Println("Your Google credentials and event database were kept.")
	}
	fmt.Println("On the tablet open AppLoad and tap Reload (or restart the UI with mods in reManager)")
	fmt.Println("so AppLoad stops listing Calendar.")
	return nil
}

// ---------- Google OAuth ----------

type desktopClient struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	AuthURI      string `json:"auth_uri"`
	TokenURI     string `json:"token_uri"`
}

type clientJSON struct {
	Installed desktopClient `json:"installed"`
}

// googleCredentials is exactly what the Rust backend deserialises from google.json.
type googleCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
}

func googleSetup(c *ssh.Client) error {
	fmt.Println()
	fmt.Println("Google sync needs your own Google Cloud OAuth client (a one-time setup).")
	fmt.Println("The tablet then syncs on its own. Short steps:")
	fmt.Println()
	fmt.Println("  1. Open https://console.cloud.google.com/ and create or select a project.")
	fmt.Println("  2. In the API Library, enable \"Google Calendar API\".")
	fmt.Println("  3. Configure the OAuth consent screen: choose External, set it to Testing,")
	fmt.Println("     and add the Gmail account you will sign in with as a test user.")
	fmt.Println("  4. Create an OAuth client of type \"Desktop app\" and download its JSON file.")
	fmt.Println()
	fmt.Println("Keep reCalendar closed on the tablet until this finishes.")
	fmt.Println()

	clientPath := strings.Trim(prompt("Path to the downloaded client JSON: "), `"' `)
	if clientPath == "" {
		return errors.New("no client JSON path given")
	}
	clientPath = filepath.Clean(clientPath)
	raw, err := os.ReadFile(clientPath)
	if err != nil {
		return fmt.Errorf("couldn't read that file: %w", err)
	}
	var cj clientJSON
	if err := json.Unmarshal(raw, &cj); err != nil {
		return errors.New("that file isn't valid JSON")
	}
	if cj.Installed.ClientID == "" || cj.Installed.ClientSecret == "" {
		return errors.New("download a Desktop app OAuth client JSON from Google Cloud (it should contain an \"installed\" section with client_id and client_secret)")
	}

	fmt.Println()
	fmt.Println("A browser window should open so you can sign in. Grant BOTH Calendar")
	fmt.Println("permissions (read and edit). Waiting for the sign-in (up to five minutes)...")
	creds, err := runOAuth(cj.Installed)
	if err != nil {
		return err
	}

	body, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')

	fmt.Println()
	fmt.Println("Uploading credentials to the tablet (file permissions set to private)...")
	if out, err := runCmd(c, "umask 077; mkdir -p "+configDir+" && chmod 700 "+configDir); err != nil {
		return fmt.Errorf("couldn't create the credentials folder: %v %s", err, out)
	}
	if err := uploadBytes(c, body, googlePath, 0600); err != nil {
		return fmt.Errorf("couldn't upload google.json: %w", err)
	}
	// Double-check mode (Rust refuses world/group-readable credentials).
	if out, err := runCmd(c, "chmod 600 "+googlePath+" && stat -c '%a' "+googlePath); err != nil || out != "600" {
		return fmt.Errorf("couldn't set private permissions on google.json (got %q): %v", out, err)
	}
	fmt.Println("Credentials are on the tablet at " + googlePath + ".")
	fmt.Println("(Secrets are never printed by this installer.)")

	if !yesNo("Run the initial sync now? (tablet needs Wi-Fi; keep reCalendar closed)", true) {
		fmt.Println()
		fmt.Println("You can sync later from AppLoad → reCalendar → Settings, or re-run this option.")
		return nil
	}
	tz := ask("Display timezone (IANA name)", defaultTZ)
	start, end := syncWindow(time.Now())
	fmt.Printf("Syncing primary calendar from %s to %s (%s)...\n", start, end, tz)
	fmt.Println("This can take a minute. Keep the tablet on Wi-Fi and reCalendar closed.")
	cmd := fmt.Sprintf("%s google-sync %s %s %s", entryPath, shellQuote(start), shellQuote(end), shellQuote(tz))
	out, err := runCmd(c, cmd)
	if err != nil {
		// Don't print raw out if it might contain tokens; google-sync shouldn't, but be careful.
		safe := redactSecrets(out)
		return fmt.Errorf("initial sync failed: %v %s", err, safe)
	}
	if safe := redactSecrets(out); safe != "" {
		fmt.Println(safe)
	}
	fmt.Println()
	fmt.Println("Initial sync finished.")
	fmt.Println("Next: on the tablet open AppLoad → Reload → Calendar. Check Settings for the")
	fmt.Println("timezone, pick which calendars to show, then Save & sync.")
	return nil
}

func uploadBytes(c *ssh.Client, data []byte, dst string, mode os.FileMode) error {
	sc, err := sftp.NewClient(c)
	if err == nil {
		defer sc.Close()
		return sftpPutBytes(sc, data, dst, mode)
	}
	return catPutBytes(c, data, dst, mode)
}

func syncWindow(now time.Time) (start, end string) {
	// README: first of this month → first of this month + 7 months (end exclusive).
	s := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	e := s.AddDate(0, 7, 0)
	return s.Format("2006-01-02"), e.Format("2006-01-02")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func redactSecrets(s string) string {
	// Belt-and-braces: never echo anything that looks like an OAuth secret.
	out := s
	for _, p := range []string{"refresh_token", "client_secret", "access_token", "code_verifier"} {
		if strings.Contains(strings.ToLower(out), p) {
			return "(output omitted because it may contain secrets)"
		}
	}
	return out
}

// runOAuth performs the desktop loopback PKCE flow (same as scripts/google-auth.py).
func runOAuth(client desktopClient) (googleCredentials, error) {
	state, err := tokenURLSafe(32)
	if err != nil {
		return googleCredentials{}, err
	}
	verifier, err := tokenURLSafe(64)
	if err != nil {
		return googleCredentials{}, err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return googleCredentials{}, fmt.Errorf("couldn't open a local callback port: %w", err)
	}
	redirect := fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "Invalid callback", 400)
			return
		}
		if e := q.Get("error"); e != "" {
			errCh <- errors.New("authorisation was denied. Run the login again when ready")
			io.WriteString(w, "Authorisation was denied. You can close this tab.")
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "Missing authorisation code", 400)
			return
		}
		io.WriteString(w, "You can close this tab and return to the reCalendar installer.")
		select {
		case codeCh <- code:
		default:
		}
	})

	go srv.Serve(ln)
	defer func() {
		srv.Close()
		ln.Close()
	}()

	authURL := "https://accounts.google.com/o/oauth2/v2/auth?" + url.Values{
		"client_id":             {client.ClientID},
		"redirect_uri":          {redirect},
		"response_type":         {"code"},
		"scope":                 {oauthScope},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	fmt.Println()
	fmt.Println("If the browser doesn't open, copy this link into it:")
	fmt.Println(authURL)
	fmt.Println()
	_ = openBrowserFn(authURL)

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return googleCredentials{}, err
	case <-time.After(5 * time.Minute):
		return googleCredentials{}, errors.New("login timed out after five minutes; run again")
	}

	tokenURL := client.TokenURI
	if tokenURL == "" {
		tokenURL = "https://oauth2.googleapis.com/token"
	}
	form := url.Values{
		"client_id":     {client.ClientID},
		"client_secret": {client.ClientSecret},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirect},
		"grant_type":    {"authorization_code"},
	}
	resp, err := httpClient.PostForm(tokenURL, form)
	if err != nil {
		return googleCredentials{}, fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return googleCredentials{}, fmt.Errorf("token exchange failed (HTTP %d); run login again", resp.StatusCode)
	}
	var tok struct {
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return googleCredentials{}, errors.New("token exchange returned invalid JSON")
	}
	if tok.RefreshToken == "" {
		return googleCredentials{}, errors.New("Google returned no refresh token; repeat login and grant offline access")
	}
	got := map[string]bool{}
	for _, s := range strings.Fields(tok.Scope) {
		got[s] = true
	}
	for _, need := range strings.Fields(oauthScope) {
		if !got[need] {
			return googleCredentials{}, errors.New("Calendar reading/editing permission was not granted; repeat login and allow both permissions")
		}
	}
	return googleCredentials{
		ClientID:     client.ClientID,
		ClientSecret: client.ClientSecret,
		RefreshToken: tok.RefreshToken,
	}, nil
}

func tokenURLSafe(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func openBrowser(u string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		return exec.Command("open", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}
