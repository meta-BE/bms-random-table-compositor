//go:build windows

package singleinstance

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

type windowsLock struct {
	handle windows.Handle
}

func (l *windowsLock) Release() error {
	if l.handle == 0 {
		return nil
	}
	defer func() {
		_ = windows.CloseHandle(l.handle)
		l.handle = 0
	}()
	// LockFileEx で取得したロックは CloseHandle で解放される
	return nil
}

// Acquire は指定パスのファイルへ排他ロックを取得する。
func Acquire(path string) (Lock, error) {
	// 親ディレクトリは存在する想定（実行ファイル隣）。なければ作成。
	if dir := pathDir(path); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}

	utf16Path, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("utf16 path: %w", err)
	}

	handle, err := windows.CreateFile(
		utf16Path,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("create lock file: %w", err)
	}

	overlapped := &windows.Overlapped{}
	if err := windows.LockFileEx(
		handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, overlapped,
	); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, ErrAlreadyRunning
	}

	return &windowsLock{handle: handle}, nil
}

func pathDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[:i]
		}
	}
	return ""
}
