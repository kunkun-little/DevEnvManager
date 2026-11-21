//go:build !windows

package main

import (
	"errors"
	"io/fs"
	"os"
	"time"
)

var onlyWindows = errors.New("仅在 Windows 上运行")

func reparse(st fs.FileInfo) bool                                 { return st.Mode()&os.ModeSymlink != 0 }
func fixedDrive(string) bool                                      { return false }
func (a *App) DriveStats() []Drive                                { return []Drive{} }
func regValue(bool, string) (prior, error)                        { return prior{}, onlyWindows }
func regSet(string, prior) error                                  { return onlyWindows }
func resolve(string) (string, error)                              { return "", onlyWindows }
func probe(string, string, time.Duration) (string, string, error) { return "", "", onlyWindows }
func probePath(string, string, string, time.Duration) (string, string, error) {
	return "", "", onlyWindows
}
func runWinget(string, []string) (string, error) { return "", onlyWindows }

func (a *App) ChooseDirectory() (string, error) { return "", onlyWindows }

func regSetRaw(string, prior) error { return onlyWindows }
func notifyEnv()                    {}

func holdConfigLock() (func(), error) { return func() {}, nil }
