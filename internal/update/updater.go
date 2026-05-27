package update

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const githubRepo = "kudryavtsevmakar/valovoting"

// Check compares currentVersion against the latest GitHub release.
// If a newer version exists, it downloads and atomically replaces the binary,
// then exits so the user restarts with the new version.
// The .env file is never touched — all settings are preserved.
// Skips silently when currentVersion == "dev" (local builds).
func Check(currentVersion string) {
	if currentVersion == "dev" {
		fmt.Printf("  %s[dev build — проверка обновлений пропущена]%s\n", dimCode, resetCode)
		return
	}

	fmt.Print("  Проверка обновлений...")

	latest, err := fetchLatest()
	if err != nil {
		fmt.Printf(" %sнедоступно%s\n", dimCode, resetCode)
		log.Printf("update: %v", err)
		return
	}

	if !isNewer(currentVersion, latest.TagName) {
		fmt.Printf(" %s✓%s актуальная версия (%s)\n", greenCode, resetCode, currentVersion)
		return
	}

	fmt.Printf("\n  %s↑ Найдена новая версия %s → %s%s\n", greenCode, currentVersion, latest.TagName, resetCode)

	assetURL, assetName := pickAsset(latest.Assets)
	if assetURL == "" {
		fmt.Printf("  Нет готового бинаря для %s/%s — пропускаем.\n", runtime.GOOS, runtime.GOARCH)
		return
	}

	fmt.Printf("  Скачиваем %s...", assetName)

	exePath, err := resolvedExePath()
	if err != nil {
		fmt.Printf(" ошибка: %v\n", err)
		return
	}

	if err := downloadAndReplace(assetURL, exePath); err != nil {
		fmt.Printf(" ошибка: %v\n", err)
		return
	}

	fmt.Printf(" %s✓%s\n\n", greenCode, resetCode)
	fmt.Printf("  %sОбновление установлено.%s .env сохранён — настройки не изменились.\n", greenCode, resetCode)
	fmt.Printf("  Перезапустите программу для применения обновления.\n\n")
	os.Exit(0)
}

// CleanupOldBinary removes the .old leftover from a previous update.
func CleanupOldBinary() {
	exePath, err := resolvedExePath()
	if err != nil {
		return
	}
	_ = os.Remove(exePath + ".old")
}

// ── Internal ─────────────────────────────────────────────────────────────────

const (
	greenCode = "\033[38;2;0;240;160m"
	resetCode = "\033[0m"
	dimCode   = "\033[2m"
)

type release struct {
	TagName string  `json:"tag_name"`
	Draft   bool    `json:"draft"`
	Assets  []asset `json:"assets"`
}

type asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func fetchLatest() (*release, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	// Use the list endpoint — /releases/latest skips prereleases and returns
	// 404 if only prerelease tags exist, causing missed updates.
	url := "https://api.github.com/repos/" + githubRepo + "/releases?per_page=10"

	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "valovoting-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub недоступен: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("GitHub rate limit превышен — попробуйте позже")
	}
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("репозиторий не найден")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API: HTTP %d", resp.StatusCode)
	}

	var releases []release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("парсинг ответа: %w", err)
	}
	// Pick the first non-draft release (prereleases are fine for auto-update)
	for i := range releases {
		if !releases[i].Draft {
			return &releases[i], nil
		}
	}
	return nil, fmt.Errorf("релизов пока нет")
}

func pickAsset(assets []asset) (url, name string) {
	// Expected naming: valovoting.exe (Windows), valovoting-linux, valovoting-darwin
	var suffix string
	switch runtime.GOOS {
	case "windows":
		suffix = ".exe"
	case "darwin":
		suffix = "-darwin"
	default:
		suffix = "-linux"
	}

	for _, a := range assets {
		if strings.HasSuffix(a.Name, suffix) {
			return a.BrowserDownloadURL, a.Name
		}
	}
	// Fallback: bare "valovoting" on Linux
	if runtime.GOOS == "linux" {
		for _, a := range assets {
			if a.Name == "valovoting" {
				return a.BrowserDownloadURL, a.Name
			}
		}
	}
	return "", ""
}

func downloadAndReplace(url, exePath string) error {
	tmpPath := exePath + ".new"
	oldPath := exePath + ".old"

	// Download to .new
	if err := downloadFile(url, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("download: %w", err)
	}
	if err := os.Chmod(tmpPath, 0755); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	// Atomically rotate: current → .old, .new → current
	// Works on Windows even for a running exe (rename changes the directory
	// entry; the running process holds the file open by inode, not by name).
	_ = os.Remove(oldPath)
	if err := os.Rename(exePath, oldPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename old: %w", err)
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		_ = os.Rename(oldPath, exePath) // restore on failure
		return fmt.Errorf("rename new: %w", err)
	}
	return nil
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func resolvedExePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

func isNewer(current, latest string) bool {
	c := parseVer(current)
	l := parseVer(latest)
	for i := range c {
		if l[i] > c[i] {
			return true
		}
		if l[i] < c[i] {
			return false
		}
	}
	return false
}

func parseVer(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var r [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		r[i], _ = strconv.Atoi(parts[i])
	}
	return r
}
