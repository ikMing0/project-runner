//go:build windows

package main

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// Check before Maven writes its intermediate JAR. A backend in another IDEA or
// runner window may hold it open even when this instance uses a different port.
func checkMavenArtifactUnlocked(root, module string) error {
	jar, err := mavenArtifactPath(root, module)
	if err != nil {
		return err
	}
	for _, path := range []string{jar, jar + ".original"} {
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		handle, err := windows.CreateFile(name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			continue
		}
		if err != nil {
			return fmt.Errorf("无法重新构建，启动产物被占用或不可访问；请先停止同一工作树的其他运行实例 (%s): %w", path, err)
		}
		windows.CloseHandle(handle)
	}
	return nil
}
