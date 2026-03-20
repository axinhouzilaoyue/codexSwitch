package selfmanage

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	binName           = "ccodex"
	defaultGitHubRepo = "axinhouzilaoyue/codexSwitch"
)

var legacyNames = []string{"cccodex", "codexswitch", "ccswitch"}

func RunUpdate() error {
	executablePath, err := currentExecutablePath()
	if err != nil {
		return err
	}
	archiveURL, err := archiveURL()
	if err != nil {
		return err
	}

	fmt.Printf("Updating %s\n", executablePath)
	fmt.Printf("Downloading %s\n", archiveURL)

	response, err := (&http.Client{Timeout: 60 * time.Second}).Get(archiveURL)
	if err != nil {
		return fmt.Errorf("download update: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("download update: unexpected status %s: %s", response.Status, strings.TrimSpace(string(snippet)))
	}

	targetDir := filepath.Dir(executablePath)
	tempFile, err := os.CreateTemp(targetDir, ".ccodex-update-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() { _ = os.Remove(tempPath) }()

	if err := extractBinary(response.Body, tempFile); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tempPath, 0o755); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tempPath, executablePath); err != nil {
		return fmt.Errorf("replace executable: %w", err)
	}
	if err := removeLegacyBinaries(targetDir); err != nil {
		return err
	}

	fmt.Printf("Updated %s\n", executablePath)
	fmt.Println("Run `ccodex version` to verify the new version.")
	return nil
}

func RunUninstall() error {
	executablePath, err := currentExecutablePath()
	if err != nil {
		return err
	}
	targetDir := filepath.Dir(executablePath)

	removed := make([]string, 0, 1+len(legacyNames))
	for _, path := range uninstallTargets(executablePath) {
		if err := os.Remove(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("remove %s: %w", path, err)
		}
		removed = append(removed, path)
	}

	if len(removed) == 0 {
		fmt.Println("Nothing to uninstall.")
	} else {
		for _, path := range removed {
			fmt.Printf("Removed %s\n", path)
		}
	}
	fmt.Println("Saved profiles remain in ~/.codex-switch")
	fmt.Printf("Current executable directory: %s\n", targetDir)
	return nil
}

func extractBinary(reader io.Reader, out *os.File) error {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("archive does not contain %s", binName)
			}
			return fmt.Errorf("read archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(header.Name) != binName {
			continue
		}
		if _, err := io.Copy(out, tarReader); err != nil {
			return fmt.Errorf("extract binary: %w", err)
		}
		return nil
	}
}

func archiveURL() (string, error) {
	if directURL := strings.TrimSpace(os.Getenv("CCODEX_ARCHIVE_URL")); directURL != "" {
		return directURL, nil
	}

	repo := strings.TrimSpace(os.Getenv("CCODEX_GITHUB_REPO"))
	if repo == "" {
		repo = defaultGitHubRepo
	}

	osName, archName, err := platformNames()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://github.com/%s/releases/latest/download/%s-%s-%s.tar.gz", repo, binName, osName, archName), nil
}

func platformNames() (string, string, error) {
	var osName string
	switch runtime.GOOS {
	case "darwin":
		osName = "darwin"
	case "linux":
		osName = "linux"
	default:
		return "", "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	var archName string
	switch runtime.GOARCH {
	case "arm64":
		archName = "arm64"
	case "amd64":
		archName = "amd64"
	default:
		return "", "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}
	return osName, archName, nil
}

func currentExecutablePath() (string, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	if resolvedPath, err := filepath.EvalSymlinks(executablePath); err == nil {
		executablePath = resolvedPath
	}
	return executablePath, nil
}

func removeLegacyBinaries(dir string) error {
	for _, name := range legacyNames {
		path := filepath.Join(dir, name)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove legacy binary %s: %w", path, err)
		}
	}
	return nil
}

func uninstallTargets(executablePath string) []string {
	targetDir := filepath.Dir(executablePath)
	targets := []string{executablePath}
	for _, name := range legacyNames {
		path := filepath.Join(targetDir, name)
		if path != executablePath {
			targets = append(targets, path)
		}
	}
	return targets
}
