package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func executablePath() (string, error) {
	image, err := os.Executable()
	if err != nil {
		return "", err
	}
	// Keep the launch path when it identifies the running image. Homebrew
	// switches this symlink to a new keg during an upgrade.
	launch, err := exec.LookPath(os.Args[0])
	if err != nil {
		return image, nil
	}
	imageInfo, imageErr := os.Stat(image)
	launchInfo, launchErr := os.Stat(launch)
	if imageErr == nil && launchErr == nil && os.SameFile(imageInfo, launchInfo) {
		return filepath.Abs(launch)
	}
	return image, nil
}

type executableRevision struct {
	info   os.FileInfo
	digest string
}

func readExecutable(path string) (executableRevision, error) {
	file, err := os.Open(path)
	if err != nil {
		return executableRevision{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return executableRevision{}, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return executableRevision{}, err
	}
	return executableRevision{info: info, digest: hex.EncodeToString(hash.Sum(nil))}, nil
}

func watchExecutable(ctx context.Context, path string, known executableRevision) <-chan executableRevision {
	if known.info == nil {
		return nil
	}
	updates := make(chan executableRevision, 1)
	go func() {
		ticker := time.NewTicker(750 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				info, err := os.Stat(path)
				if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
					continue
				}
				if os.SameFile(info, known.info) && info.Size() == known.info.Size() &&
					info.ModTime().Equal(known.info.ModTime()) {
					continue
				}
				revision, err := readExecutable(path)
				if err != nil {
					continue
				}
				if revision.digest != known.digest {
					updates <- revision
					return
				}
				known = revision
			}
		}
	}()
	return updates
}
