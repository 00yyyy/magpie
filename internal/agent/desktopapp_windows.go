//go:build windows

package agent

import "golang.org/x/sys/windows/registry"

const appxPackagesKey = `Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages`

// installedWindowsStorePackages returns package full names registered for the
// current user. Reading this key needs no access to the protected WindowsApps
// folder and does not start PowerShell.
func installedWindowsStorePackages() []string {
	key, err := registry.OpenKey(registry.CURRENT_USER, appxPackagesKey, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer key.Close()
	names, _ := key.ReadSubKeyNames(-1)
	return names
}

// registeredWindowsApp reads what an electron-builder installer registered
// for the current user: HKCU\Software\<guid>'s InstallLocation, and the
// DisplayVersion of its Uninstall entry.
func registeredWindowsApp(guid string) (dir, version string) {
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\`+guid, registry.QUERY_VALUE); err == nil {
		dir, _, _ = k.GetStringValue("InstallLocation")
		k.Close()
	}
	if dir == "" {
		return "", ""
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Uninstall\`+guid, registry.QUERY_VALUE); err == nil {
		version, _, _ = k.GetStringValue("DisplayVersion")
		k.Close()
	}
	return dir, version
}
