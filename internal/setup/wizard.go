package setup

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
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
func Run() error {
	PrintBanner()
	fmt.Printf("  Первый запуск %s— давайте войдём в Twitch-аккаунт.%s\n", cDim, cReset)

	// ── Step 1: Twitch login ─────────────────────────────────────────────────
	step(1, 3, "Вход в Twitch")

	authURL := fmt.Sprintf(
		"https://id.twitch.tv/oauth2/authorize"+
			"?client_id=%s"+
			"&redirect_uri=http://localhost:8080"+
			"&response_type=token"+
			"&scope=channel%%3Aread%%3Apolls+channel%%3Amanage%%3Apolls+chat%%3Aread",
		config.TwitchClientID,
	)

	info("Открываем браузер для авторизации...")
	openBrowser(authURL)
	fmt.Println()
	info("Если браузер не открылся, откройте эту ссылку вручную:")
	fmt.Printf("\n  %s%s%s\n\n", cGreen, authURL, cReset)
	info("После авторизации вы попадёте на страницу вида:")
	info("  http://localhost:8080/#access_token=XXXXX&...")
	info("Скопируйте значение access_token из адресной строки.")

	var token, broadcasterID, channelLogin string
	for {
		token = mustInput("Access Token")
		id, login, err := fetchUser(token)
		if err != nil {
			fail("Не удалось проверить токен: " + err.Error())
			info("Убедитесь, что токен скопирован полностью, и попробуйте снова.")
			continue
		}
		broadcasterID = id
		channelLogin = login
		ok(fmt.Sprintf("Вы вошли как  %s%s%s  (ID: %s)", cBold+cWhite, login, cReset, id))
		break
	}

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
		broadcasterID, token,
		channelLogin, chatCmd,
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

// ── Browser ──────────────────────────────────────────────────────────────────

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start() // best-effort — user can open manually
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
