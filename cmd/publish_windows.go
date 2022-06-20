package cmd

import (
	"os"
)

func processExists(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}
