package nodeinit

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/projectcalico/node/pkg/startup"
	"github.com/sirupsen/logrus"
)

const bpfFSMountPoint = "/sys/fs/bpf"

func Run() {
	// Check $CALICO_STARTUP_LOGLEVEL to capture early log statements
	startup.ConfigureLogging()

	err := ensureBPFFilesystem()
	if err != nil {
		logrus.WithError(err).Error("Failed to mount BPF filesystem.")
		os.Exit(2)
	}
}

func ensureBPFFilesystem() error {
	// Check if the BPF filesystem is mounted at the expected location.
	logrus.Info("Checking if BPF filesystem is mounted.")
	mounts, err := os.Open("/proc/mounts")
	if err != nil {
		return fmt.Errorf("failed to open /proc/mounts: %w", err)
	}
	scanner := bufio.NewScanner(mounts)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), " ")
		mountPoint := parts[1]
		fs := parts[2]

		if mountPoint == bpfFSMountPoint && fs == "bpf" {
			logrus.Info("BPF filesystem is mounted.")
			return nil
		}
	}
	if scanner.Err() != nil {
		return fmt.Errorf("failed to read /proc/mounts: %w", scanner.Err())
	}

	// If we get here, the BPF filesystem is not mounted.  Try to mount it.
	logrus.Info("BPF filesystem is not mounted.  Trying to mount it...")
	err = syscall.Mount(bpfFSMountPoint, bpfFSMountPoint, "bpf", 0, "")
	if err != nil {
		return fmt.Errorf("failed to mount BPF filesystem: %w", err)
	}
	logrus.Info("Mounted BPF filesystem.")
	return nil
}
