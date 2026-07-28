//go:build windows

package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	winpmem "github.com/Velocidex/WinPmem/go-winpmem"
)

func openRAM(ctx context.Context, provider, path, toolPath string) (*Handle, error) {
	switch strings.ToLower(provider) {
	case "", "auto", "winpmem":
		logger := winpmem.NewLogger(false)
		driver := toolPath
		temporaryDriver := false
		var err error
		if driver == "" {
			driverCode, codeErr := winpmem.Winpmem_x64()
			if codeErr != nil {
				return nil, fmt.Errorf("extract signed WinPmem driver: %w", codeErr)
			}
			f, createErr := os.CreateTemp("", "imajer-winpmem-*.sys")
			if createErr != nil {
				return nil, createErr
			}
			driver = f.Name()
			temporaryDriver = true
			if _, err = f.Write([]byte(driverCode)); err == nil {
				err = f.Sync()
			}
			err = errors.Join(err, f.Close())
			if err != nil {
				_ = os.Remove(driver)
				return nil, fmt.Errorf("write signed WinPmem driver: %w", err)
			}
		}
		service := "imajer_winpmem_" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatInt(time.Now().UnixNano(), 36)
		if err := winpmem.InstallDriver(driver, service, logger); err != nil {
			if temporaryDriver {
				_ = os.Remove(driver)
			}
			return nil, fmt.Errorf("install WinPmem driver: %w", err)
		}
		imager, err := winpmem.NewImager(`\\.\pmem`, logger)
		if err != nil {
			_ = winpmem.UninstallDriver(driver, service, logger)
			if temporaryDriver {
				_ = os.Remove(driver)
			}
			return nil, fmt.Errorf("open WinPmem device: %w", err)
		}
		if err := imager.SetMode(winpmem.PMEM_MODE_PTE); err != nil {
			imager.Close()
			_ = winpmem.UninstallDriver(driver, service, logger)
			if temporaryDriver {
				_ = os.Remove(driver)
			}
			return nil, fmt.Errorf("set WinPmem acquisition mode: %w", err)
		}
		pr, pw := io.Pipe()
		writeCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() {
			err := imager.WriteTo(writeCtx, pw)
			_ = pw.CloseWithError(err)
			done <- err
		}()
		return &Handle{
			Reader: pr, Provider: "winpmem",
			Close: func() error {
				cancel()
				_ = pr.Close()
				writeErr := <-done
				imager.Close()
				uninstallErr := winpmem.UninstallDriver(driver, service, logger)
				if temporaryDriver {
					_ = os.Remove(driver)
				}
				if strings.Contains(strings.ToLower(fmt.Sprint(writeErr)), "cancel") {
					writeErr = nil
				}
				return errors.Join(writeErr, uninstallErr)
			},
		}, nil
	case "direct":
		if path == "" {
			return nil, errors.New("direct RAM provider requires path")
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		return &Handle{Reader: f, Provider: "direct", Close: f.Close}, nil
	default:
		return nil, fmt.Errorf("unsupported Windows RAM provider %q", provider)
	}
}
