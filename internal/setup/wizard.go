package setup

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kudryavtsevmakar/valovoting/internal/config"
)

const (
	cReset = "\033[0m"
	cGreen = "\033[38;2;0;240;160m"
	cBold  = "\033[1m"
	cDim   = "\033[2m"
	cRed   = "\033[91m"
	cWhite = "\033[97m"
)

var stdinReader = bufio.NewReader(os.Stdin)

func readLine() string {
	s, _ := stdinReader.ReadString('\n')
	return strings.TrimSpace(strings.TrimRight(s, "\r\n"))
}

// ── Public API ───────────────────────────────────────────────────────────────

// IsNeeded returns true when .env is missing or any required token field is blank.
func IsNeeded() bool {
	data, err := os.ReadFile(".env")
	if err != nil {
		return true
	}
	for _, key := range []string{"TWITCH_BROADCASTER_ID", "TWITCH_ACCESS_TOKEN", "TWITCH_CHANNEL"} {
		prefix := key + "="
		idx := strings.Index(string(data), prefix)
		if idx < 0 {
			return true
		}
		val := string(data)[idx+len(prefix):]
		if nl := strings.IndexAny(val, "\r\n"); nl >= 0 {
			val = val[:nl]
		}
		if strings.TrimSpace(val) == "" || strings.HasPrefix(val, "your_") {
			return true
		}
	}
	return false
}

// PrintBanner prints the styled application header.
func PrintBanner() {
	enableColor()
	const inner = 51
	bar := strings.Repeat("═", inner)
	fmt.Println()
	fmt.Printf("  %s╔%s╗%s\n", cGreen, bar, cReset)
	title := "  ▸ VALORANT POLL OVERLAY"
	pad := strings.Repeat(" ", inner-len(title)-1)
	fmt.Printf("  %s║%s%s%s%s%s║%s\n", cGreen, cReset, cBold+cWhite, title, cReset+cGreen, pad, cReset)
	fmt.Printf("  %s╚%s╝%s\n", cGreen, bar, cReset)
	fmt.Println()
}

// Run executes the interactive setup wizard and writes .env on success.
// PrintBanner must be called by the caller before Run.
func Run() error {
	fmt.Printf("  Первый запуск %s— давайте войдём в Twitch-аккаунт.%s\n", cDim, cReset)

	// ── Step 1: Twitch OAuth (automatic capture) ─────────────────────────────
	step(1, 3, "Вход в Twitch")
	info("Открываем браузер. После входа страница закроется автоматически.")
	info("Если браузер не открылся — скопируйте ссылку ниже вручную.")
	fmt.Println()

	authURL := buildAuthURL()
	fmt.Printf("  %s%s%s\n\n", cDim, authURL, cReset)

	fmt.Printf("  %sОжидаем авторизацию в браузере...%s\n", cDim, cReset)

	// Start a temporary local server to capture the OAuth token automatically.
	// The browser is redirected to http://localhost:8080, our page reads the
	// #fragment with JS and POSTs the token back to /oauth-token.
	token, err := captureToken(authURL)
	if err != nil {
		// Fallback: let the user paste it manually
		fmt.Printf("  %sАвтоматический захват не сработал (%v).%s\n", cRed, err, cReset)
		info("Скопируйте access_token из адресной строки браузера вручную.")
		token = mustInput("Access Token")
	}

	id, login, err := fetchUser(token)
	if err != nil {
		return fmt.Errorf("не удалось проверить токен: %w", err)
	}
	ok(fmt.Sprintf("Вы вошли как  %s%s%s  (ID: %s)", cBold+cWhite, login, cReset, id))

	// ── Step 2: Bot options ──────────────────────────────────────────────────
	step(2, 3, "Настройки бота")
	info("Нажмите Enter, чтобы оставить значение по умолчанию.")

	chatCmd := inputDefault("Команда в чате", "!mapvote")

	var duration int
	for {
		raw := inputDefault("Длительность опроса (сек)", "60")
		n, err := strconv.Atoi(raw)
		if err != nil || n < 15 || n > 1800 {
			fail("Введите число от 15 до 1800.")
			continue
		}
		duration = n
		break
	}

	var port string
	for {
		port = inputDefault("HTTP-порт", "8080")
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			fail("Введите корректный номер порта (1–65535).")
			continue
		}
		_ = n
		break
	}

	// ── Step 3: Save ────────────────────────────────────────────────────────
	step(3, 3, "Сохранение")

	envContent := fmt.Sprintf(
		"TWITCH_BROADCASTER_ID=%s\nTWITCH_ACCESS_TOKEN=%s\n"+
			"TWITCH_CHANNEL=%s\nCHAT_COMMAND=%s\n"+
			"DEFAULT_POLL_DURATION=%d\nPORT=%s\n",
		id, token,
		login, chatCmd,
		duration, port,
	)

	if err := os.WriteFile(".env", []byte(envContent), 0600); err != nil {
		return fmt.Errorf("не удалось записать .env: %w", err)
	}
	ok(".env сохранён")

	// OBS instructions
	fmt.Println()
	div := cDim + strings.Repeat("─", 49) + cReset
	fmt.Println("  " + div)
	fmt.Printf("  %sOBS Browser Source%s\n", cBold+cWhite, cReset)
	fmt.Println("  " + div)
	fmt.Println()
	highlight(fmt.Sprintf("URL        http://localhost:%s/overlay", port))
	highlight("Размер     1920 × 1080")
	highlight("Custom CSS body { background: transparent !important; }")
	fmt.Println()
	fmt.Printf("  %sКоманда запуска голосования: %s%s%s\n", cDim, cReset+cGreen+cBold, chatCmd, cReset)
	fmt.Printf("  %sТолько стример и модераторы могут использовать команду.%s\n", cDim, cReset)
	fmt.Println()
	fmt.Printf("  Нажмите %sEnter%s для запуска сервера...", cGreen+cBold, cReset)
	readLine()
	fmt.Println()

	return nil
}

// ── OAuth capture ─────────────────────────────────────────────────────────────

// callbackHTML is served at http://localhost:8080 after the Twitch redirect.
// JS reads the token from the URL fragment (never sent to the server) and
// POSTs it to /oauth-token so our Go code can receive it.
const callbackHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<title>Valorant Poll Overlay</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:#0f0f14;color:#fff;font-family:sans-serif;
     display:flex;align-items:center;justify-content:center;height:100vh}
.box{text-align:center}
.icon{font-size:56px;margin-bottom:16px}
.title{font-size:22px;font-weight:700;letter-spacing:1px}
.sub{font-size:14px;color:rgba(255,255,255,.45);margin-top:8px}
.green{color:#00f0a0}
</style>
</head>
<body>
<div class="box" id="box">
  <div class="icon">⏳</div>
  <div class="title">Авторизация...</div>
</div>
<script>
const p = new URLSearchParams(window.location.hash.slice(1));
const token = p.get("access_token");
const box = document.getElementById("box");
if (token) {
  fetch("/oauth-token?t=" + encodeURIComponent(token))
    .then(() => {
      box.innerHTML =
        '<div class="icon green">✓</div>' +
        '<div class="title">Вы вошли в Twitch!</div>' +
        '<div class="sub">Вернитесь в терминал — настройка продолжается.</div>';
    })
    .catch(() => {
      box.innerHTML = '<div class="icon" style="color:red">✗</div><div class="title">Ошибка отправки токена.</div>';
    });
} else {
  box.innerHTML = '<div class="icon" style="color:red">✗</div><div class="title">Токен не найден.</div><div class="sub">Попробуйте ещё раз.</div>';
}
</script>
</body>
</html>`

// captureToken starts a temporary HTTP server on :8080, opens the auth URL
// in the browser, waits for the JS callback with the token, then shuts down.
func captureToken(authURL string) (string, error) {
	tokenCh := make(chan string, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, callbackHTML)
	})
	mux.HandleFunc("/oauth-token", func(w http.ResponseWriter, r *http.Request) {
		t := r.URL.Query().Get("t")
		w.WriteHeader(http.StatusOK)
		if t != "" {
			select {
			case tokenCh <- t:
			default:
			}
		}
	})

	srv := &http.Server{Addr: ":8080", Handler: mux}
	srvErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErr <- err
		}
	}()
	// Give the server a moment to bind
	time.Sleep(100 * time.Millisecond)

	// Open browser via a temp HTML redirect file — avoids & being parsed
	// by xdg-open / cmd.exe as a shell operator
	openViaTempFile(authURL)

	select {
	case token := <-tokenCh:
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		return token, nil
	case err := <-srvErr:
		return "", fmt.Errorf("локальный сервер: %w", err)
	case <-time.After(5 * time.Minute):
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		return "", fmt.Errorf("timeout (5 мин)")
	}
}

// openViaTempFile writes a tiny HTML redirect page to a temp file and opens
// that file in the browser. This avoids passing the auth URL (which contains
// & characters) directly to xdg-open / cmd.exe where & is a shell operator.
func openViaTempFile(authURL string) {
	// In HTML, & inside an attribute value must be &amp;
	htmlSafe := strings.ReplaceAll(authURL, "&", "&amp;")
	html := fmt.Sprintf(`<!DOCTYPE html><html><head>
<meta charset="utf-8">
<meta http-equiv="refresh" content="0;url=%s">
</head><body>Открываем Twitch...</body></html>`, htmlSafe)

	tmp, err := os.CreateTemp("", "valovoting_auth_*.html")
	if err != nil {
		// Fallback: try direct open anyway
		openDirect(authURL)
		return
	}
	tmp.WriteString(html)
	tmp.Close()

	go func() {
		time.Sleep(15 * time.Second)
		os.Remove(tmp.Name())
	}()

	switch runtime.GOOS {
	case "windows":
		exec.Command("cmd", "/c", "start", "", tmp.Name()).Start()
	case "darwin":
		exec.Command("open", tmp.Name()).Start()
	default:
		exec.Command("xdg-open", tmp.Name()).Start()
	}
}

// openDirect is the last-resort fallback for opening URLs directly.
func openDirect(u string) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		exec.Command("open", u).Start()
	default:
		exec.Command("xdg-open", u).Start()
	}
}

// ── URL builder ───────────────────────────────────────────────────────────────

func buildAuthURL() string {
	p := url.Values{}
	p.Set("client_id", config.TwitchClientID)
	p.Set("redirect_uri", "http://localhost:8080")
	p.Set("response_type", "token")
	p.Set("scope", "channel:read:polls channel:manage:polls chat:read")
	return "https://id.twitch.tv/oauth2/authorize?" + p.Encode()
}

// ── Layout helpers ───────────────────────────────────────────────────────────

func step(n, total int, title string) {
	sep := cDim + strings.Repeat("─", 49) + cReset
	fmt.Printf("\n  %s[ %d / %d ]  %s%s%s\n", cGreen+cBold, n, total, cReset+cBold+cWhite, title, cReset)
	fmt.Println("  " + sep)
}

func info(msg string)      { fmt.Printf("  %s%s%s\n", cDim, msg, cReset) }
func highlight(msg string) { fmt.Printf("  %s▸%s %s\n", cGreen+cBold, cReset, msg) }
func ok(msg string)        { fmt.Printf("  %s✓%s  %s\n", cGreen+cBold, cReset, msg) }
func fail(msg string)      { fmt.Printf("  %s✗%s  %s%s%s\n", cRed+cBold, cReset, cRed, msg, cReset) }

func mustInput(label string) string {
	for {
		fmt.Printf("\n  %s%s%s\n  %s›%s ", cBold+cWhite, label, cReset, cGreen, cReset)
		v := readLine()
		if v != "" {
			return v
		}
		fail("Это поле обязательно.")
	}
}

func inputDefault(label, def string) string {
	fmt.Printf("\n  %s%s%s %s[%s]%s\n  %s›%s ", cBold+cWhite, label, cReset, cDim, def, cReset, cGreen, cReset)
	v := readLine()
	if v == "" {
		return def
	}
	return v
}

// ── Twitch API ────────────────────────────────────────────────────────────────

func fetchUser(token string) (id, login string, err error) {
	client := &http.Client{Timeout: 8 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, "https://api.twitch.tv/helix/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-Id", config.TwitchClientID)

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var body struct {
		Data []struct {
			ID    string `json:"id"`
			Login string `json:"login"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", err
	}
	if len(body.Data) == 0 {
		return "", "", fmt.Errorf("пользователь не найден")
	}
	return body.Data[0].ID, body.Data[0].Login, nil
}
