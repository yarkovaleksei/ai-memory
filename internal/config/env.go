package config

import (
	"os"
	"runtime"
)

// FromEnvironment собирает Env из текущего процесса. Ошибку os.UserHomeDir не
// возвращает: пустой Home превратится в ErrNoHome в Load.
func FromEnvironment() Env {
	home, _ := os.UserHomeDir()
	return Env{
		GOOS:    runtime.GOOS,
		Home:    home,
		Getenv:  os.Getenv,
		ListSep: string(os.PathListSeparator),
	}
}
