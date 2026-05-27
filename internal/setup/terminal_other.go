//go:build !windows

package setup

func enableColor() {} // ANSI works natively on Unix terminals
