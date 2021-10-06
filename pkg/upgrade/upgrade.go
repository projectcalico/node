// Copyright (c) 2021 Tigera, Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package upgrade

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/projectcalico/node/pkg/lifecycle/startup"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/projectcalico/node/pkg/lifecycle/utils"

	log "github.com/sirupsen/logrus"
)

const (
	CalicoKubeConfigFile     = "calico-kube-config"
	CalicoUpdateDir          = "c:\\CalicoUpdate"
	CalicoBaseDir            = "c:\\CalicoWindows"
	EnterpriseBaseDir        = "c:\\TigeraCalico"
	CalicoUpgradeScriptLabel = "projectcalico.org/CalicoWindowsUpgradeScript"
	CalicoVersionAnnotation  = "projectcalico.org/CalicoWindowsVersion"
)

var (
	calicoUpgradeImageRegex = regexp.MustCompile("[a-zA-Z0-9.-]+(?::[0-9]+)?/calico/windows-upgrade(:[a-zA-Z0-9.-]+|@sha256:[a-zA-Z0-9]+)")
)

func getVersionString() string {
	if runningEnterprise() {
		return "Enterprise-" + startup.VERSION
	}
	return "Calico-" + startup.VERSION
}

// This file contains the upgrade processing for the calico/node.  This
// includes:
// - Monitoring node labels and getting the Calico Windows upgrade script file from the label.
// - Uninstalling current Calico Windows (OSS or Enterprise) running on the node.
// - Install new version of Calico Windows.
func Run() {
	// Determine the name for this node.
	nodeName := utils.DetermineNodeName()
	log.Infof("Starting Calico upgrade service on node %s ", nodeName)

	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigFile())
	if err != nil {
		log.WithError(err).Fatal("Failed to build Kubernetes client config")
	}
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.WithError(err).Fatal("Failed to create Kubernetes client")
	}

	stdout, stderr, err := powershell("Get-ComputerInfo | select WindowsVersion, OsBuildNumber, OsHardwareAbstractionLayer")
	fmt.Println(stdout, stderr)
	if err != nil {
		log.WithError(err).Fatal("Failed to interact with powershell")
	}

	fmt.Println(stdout, stderr)

	// Configure node labels.
	// Set Version annotation to indicate what is running on this node.
	version := getVersionString()
	node := k8snode(nodeName)
	err = node.addRemoveNodeAnnotations(clientSet,
		map[string]string{CalicoVersionAnnotation: version},
		[]string{})
	if err != nil {
		log.WithError(err).Fatal("Failed to configure node labels")
	}
	log.Infof("Service setting version annotation on node %s", version)

	ctx, cancel := context.WithCancel(context.Background())

	go loop(ctx, clientSet, nodeName)

	// Trap cancellation on Windows. https://golang.org/pkg/os/signal/
	sigCh := make(chan os.Signal, 1)
	signal.Notify(
		sigCh,
		syscall.SIGTERM,
		syscall.SIGINT,
		syscall.SIGQUIT,
	)

	<-sigCh
	cancel()
	log.Info("Received system signal to exit")
}

func loop(ctx context.Context, cs kubernetes.Interface, nodeName string) {
	ticker := time.NewTicker(10 * time.Second)

	getScript := func() (string, error) {
		node, err := cs.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			// Treat error as the label does not exist.
			// Will retry by outer loop.
			log.WithError(err).Error("Unable to get node resources")
			return "", nil
		}

		fileName, ok := node.Labels[CalicoUpgradeScriptLabel]
		if !ok {
			return "", nil
		}

		script := filepath.Join(CalicoUpdateDir, fileName)

		if _, err := os.Stat(script); err == nil || os.IsExist(err) {
			return script, nil
		} else {
			return script, err
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			script, err := getScript()
			if err != nil {
				log.Info("Calico upgrade script is not available yet.")
				// Use fast ticker
				ticker.Stop()
				ticker = time.NewTicker(3 * time.Second)
			} else {
				if len(script) > 0 {
					// Before executing the script, verify host path volume mount.
					_, err := verifyPodImageWithHostPathVolume(cs, nodeName, CalicoUpdateDir, calicoUpgradeImageRegex)
					if err != nil {
						log.WithError(err).Fatal("Failed to verify windows-upgrade pod image")
					}

					err = uninstall()
					if err != nil {
						log.WithError(err).Error("Uninstall failed")
						break
					}

					time.Sleep(3 * time.Second)
					err = execScript(script)
					if err != nil {
						log.WithError(err).Fatal("Failed to upgrade to new version")
					}

					log.Info("All done.")
					if err := os.Remove(script); err != nil {
						log.WithError(err).Error("failed to remove " + script)
					}
					return
				}
				// No upgrade script label yet, continue
				log.Info("Node's upgrade script label does not exist yet")
			}
		}
	}
}

// Return the base directory for Calico upgrade service.
func baseDir() string {
	dir := filepath.Dir(os.Args[0])
	parent := "c:\\" + filepath.Base(dir)
	log.Infof("Calico upgrade service base directory: %s", parent)

	return parent
}

// Return if the monitor service is running as part of Enterprise installation.
func runningEnterprise() bool {
	return baseDir() == EnterpriseBaseDir
}

// Return kubeconfig file path for Calico
func kubeConfigFile() string {
	return baseDir() + "\\" + CalicoKubeConfigFile
}

func uninstall() error {
	path := filepath.Join(baseDir(), "uninstall-calico.ps1")
	log.Infof("Start uninstall script %s\n", path)
	stdout, stderr, err := powershell(path)
	fmt.Println(stdout, stderr)
	if err != nil {
		return err
	}
	return nil
}

func execScript(script string) error {
	log.Infof("Start script %s\n", script)
	stdout, stderr, err := powershell(script)
	if err != nil {
		return err
	}
	fmt.Println(stdout, stderr)
	return nil
}

func verifyPodImageWithHostPathVolume(cs kubernetes.Interface, nodeName string, hostPath string, imageRegex *regexp.Regexp) (string, error) {
	// Get pod list for all pods on this node.
	list, err := cs.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName,
	})
	if err != nil {
		return "", err
	}

	hasHostPathVolume := func(pod v1.Pod, hostPath string) bool {
		for _, v := range pod.Spec.Volumes {
			path := v.HostPath
			if path == nil {
				continue
			}
			if path.Path == hostPath {
				return true
			}
		}
		return false
	}

	// Walk through pods
	for _, pod := range list.Items {
		if !hasHostPathVolume(pod, hostPath) {
			continue
		}

		if len(pod.Spec.Containers) != 1 {
			return "", fmt.Errorf("Pod with hostpath volume has more than one container")
		}

		container := pod.Spec.Containers[0]
		log.Infof("container image is %v\n", container.Image)

		if !imageRegex.MatchString(container.Image) {
			return "", fmt.Errorf("Pod with hostpath volume has invalid image %s, prefix %s", container.Image, imageRegex)
		}

		return container.Image, nil
	}

	return "", fmt.Errorf("Failed to find windows-upgrade pod")
}

func powershell(args ...string) (string, string, error) {
	ps, err := exec.LookPath("powershell.exe")
	if err != nil {
		return "", "", err
	}

	args = append([]string{"-NoProfile", "-NonInteractive"}, args...)
	cmd := exec.Command(ps, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return "", "", err
	}

	return stdout.String(), stderr.String(), err
}
