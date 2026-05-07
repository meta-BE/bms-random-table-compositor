//go:build !windows

package singleinstance

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

type unixLock struct {
	file *os.File
}

func (l *unixLock) Release() error {
	if l.file == nil {
		return nil
	}
	defer func() {
		_ = l.file.Close()
		l.file = nil
	}()
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("flock unlock: %w", err)
	}
	return nil
}

// Acquire は指定パスのファイルへ排他ロックを取得する。
// 既に他プロセスがロックしていれば ErrAlreadyRunning を返す。
func Acquire(path string) (Lock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("flock: %w", err)
	}

	return &unixLock{file: file}, nil
}
