//go:build windows

package ui

import "github.com/NetMoose/Golphi/internal/backend/win32"

var newDriver = win32.New
