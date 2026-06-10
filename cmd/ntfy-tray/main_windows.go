//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/getlantern/systray"
	"golang.org/x/sys/windows/registry"
	"heckel.io/ntfy/v2/client"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/tray"
)

const (
	appName        = "ntfy tray"
	runValueName   = "ntfy-tray"
	appUserModelID = "ntfy"
)

// These variables are set during build time using -ldflags.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

type app struct {
	mu       sync.Mutex
	listener *tray.Listener
	starting bool
	status   *systray.MenuItem
	toggle   *systray.MenuItem
	config   string
	icon     string
}

func main() {
	setAppUserModelID(appUserModelID)
	config := client.DefaultConfigFile
	icon := defaultIconPath()
	a := &app{config: config, icon: icon}
	systray.Run(a.onReady, a.onExit)
}

func (a *app) onReady() {
	if iconBytes, err := os.ReadFile(a.icon); err == nil {
		systray.SetIcon(iconBytes)
	}
	systray.SetTitle(appName)
	systray.SetTooltip(appName)

	a.status = systray.AddMenuItem("Status: stopped", "Current listener status")
	a.status.Disable()
	a.toggle = systray.AddMenuItem("Start listener", "Start or stop notification listening")
	openConfig := systray.AddMenuItem("Open config file", "Open client.yml")
	openConfigDir := systray.AddMenuItem("Open config folder", "Open ntfy config folder")
	startup := systray.AddMenuItemCheckbox("Start on login", "Toggle per-user Windows startup", startupEnabled())
	systray.AddSeparator()
	about := systray.AddMenuItem(fmt.Sprintf("ntfy tray %s", version), fmt.Sprintf("commit %s, built at %s, runtime %s", commit, date, runtime.Version()))
	about.Disable()
	quit := systray.AddMenuItem("Quit", "Exit ntfy tray")

	go a.start()
	go func() {
		for {
			select {
			case <-a.toggle.ClickedCh:
				if a.isRunning() {
					a.stop()
				} else {
					a.start()
				}
			case <-openConfig.ClickedCh:
				a.ensureConfig()
				openPath(a.config)
			case <-openConfigDir.ClickedCh:
				a.ensureConfigDir()
				openPath(filepath.Dir(a.config))
			case <-startup.ClickedCh:
				if startup.Checked() {
					if err := disableStartup(); err != nil {
						log.Warn("Failed to disable startup: %s", err.Error())
					}
					startup.Uncheck()
				} else {
					if err := enableStartup(); err != nil {
						log.Warn("Failed to enable startup: %s", err.Error())
					} else {
						startup.Check()
					}
				}
			case <-quit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func (a *app) onExit() {
	a.stop()
}

func (a *app) start() {
	a.mu.Lock()
	if a.listener != nil || a.starting {
		a.mu.Unlock()
		return
	}
	a.starting = true
	a.mu.Unlock()

	conf, err := a.loadConfig()
	if err != nil {
		a.markStartDone(nil)
		a.setStopped(fmt.Sprintf("Status: %s", err.Error()))
		return
	}

	listener := tray.NewListener(tray.NewClientSubscriber(conf), tray.NewToastNotifier(a.icon))
	if err := listener.Start(conf); err != nil {
		a.markStartDone(nil)
		a.setStopped(fmt.Sprintf("Status: %s", err.Error()))
		return
	}

	a.markStartDone(listener)
	a.setRunning()
}

func (a *app) stop() {
	a.mu.Lock()
	listener := a.listener
	a.listener = nil
	a.starting = false
	a.mu.Unlock()
	if listener != nil {
		listener.Stop()
	}
	a.setStopped("Status: stopped")
}

func (a *app) isRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.listener != nil
}

func (a *app) markStartDone(listener *tray.Listener) {
	a.mu.Lock()
	a.listener = listener
	a.starting = false
	a.mu.Unlock()
}

func (a *app) loadConfig() (*client.Config, error) {
	if a.config == "" {
		return nil, errors.New("client config path unavailable")
	}
	if err := a.ensureConfig(); err != nil {
		return nil, err
	}
	conf, err := client.LoadConfig(a.config)
	if err != nil {
		return nil, err
	}
	if len(conf.Subscribe) == 0 {
		return nil, errors.New("no subscriptions configured")
	}
	return conf, nil
}

func (a *app) ensureConfig() error {
	if err := a.ensureConfigDir(); err != nil {
		return err
	}
	if _, err := os.Stat(a.config); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(a.config, []byte(defaultClientConfig()), 0600)
}

func (a *app) ensureConfigDir() error {
	if a.config == "" {
		return errors.New("client config path unavailable")
	}
	return os.MkdirAll(filepath.Dir(a.config), 0700)
}

func (a *app) setRunning() {
	a.status.SetTitle("Status: listening")
	a.toggle.SetTitle("Stop listener")
	systray.SetTooltip("ntfy tray: listening")
}

func (a *app) setStopped(status string) {
	a.status.SetTitle(status)
	a.toggle.SetTitle("Start listener")
	systray.SetTooltip("ntfy tray: stopped")
}

func defaultIconPath() string {
	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		for _, candidate := range []string{
			filepath.Join(exeDir, "favicon.ico"),
			filepath.Join(exeDir, "web", "public", "static", "images", "favicon.ico"),
		} {
			if _, statErr := os.Stat(candidate); statErr == nil {
				return candidate
			}
		}
	}
	return filepath.Join("web", "public", "static", "images", "favicon.ico")
}

func defaultClientConfig() string {
	return `# ntfy tray listens to subscriptions configured here.
# Edit this file and add one or more topics, then use "Start listener" from the tray menu.
#
# default-host: https://ntfy.sh
#
# subscribe:
#   - topic: mytopic
`
}

func startupEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetStringValue(runValueName)
	if err != nil || value == "" {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.Trim(value, `"`), exe)
}

func enableStartup() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(runValueName, fmt.Sprintf(`"%s"`, exe))
}

func disableStartup() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.DeleteValue(runValueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

func openPath(path string) {
	if path == "" {
		return
	}
	if err := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", path).Start(); err != nil {
		log.Warn("Failed to open %s: %s", path, err.Error())
	}
}

func setAppUserModelID(appID string) {
	shell32 := syscall.NewLazyDLL("shell32.dll")
	setCurrentProcessExplicitAppUserModelID := shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
	appIDPtr, err := syscall.UTF16PtrFromString(appID)
	if err != nil {
		log.Warn("Failed to encode AppUserModelID: %s", err.Error())
		return
	}
	ret, _, err := setCurrentProcessExplicitAppUserModelID.Call(uintptr(unsafe.Pointer(appIDPtr)))
	if ret != 0 {
		log.Warn("Failed to set AppUserModelID: %s", err.Error())
	}
}
