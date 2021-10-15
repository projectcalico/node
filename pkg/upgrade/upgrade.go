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
	"strings"
	"syscall"
	"time"

	image "github.com/distribution/distribution/reference"
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
	CalicoUpgradeDir         = "c:\\CalicoUpgrade"
	CalicoBaseDir            = "c:\\CalicoWindows"
	EnterpriseBaseDir        = "c:\\TigeraCalico"
	CalicoUpgradeScriptLabel = "projectcalico.org/CalicoWindowsUpgradeScript"
	CalicoVersionAnnotation  = "projectcalico.org/CalicoWindowsVersion"
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
	version := getVersionString()

	// Determine the name for this node.
	nodeName := utils.DetermineNodeName()
	log.Infof("Starting Calico upgrade service on node: %s. Version: %s, baseDir: %s", nodeName, version, baseDir())

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

	// Configure node labels.
	// Set Version annotation to indicate what is running on this node.
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

		script := filepath.Join(CalicoUpgradeDir, fileName)

		if _, err := os.Stat(script); err == nil || os.IsExist(err) {
			return script, nil
		} else {
			return script, err
		}
	}

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		case <-ticker.C:
			script, err := getScript()
			if err != nil {
				log.Info("Calico upgrade script is not available yet.")
				// Use fast ticker
				ticker.Stop()
				ticker = time.NewTicker(3 * time.Second)
				break
			}
			if len(script) == 0 {
				// No upgrade script label yet, continue
				log.Info("Node's upgrade script label does not exist yet")
				break
			}

			// Before executing the script, verify host path volume mount.
			err = verifyPodImageWithHostPathVolume(cs, nodeName, CalicoUpgradeDir)
			if err != nil {
				log.WithError(err).Fatal("Failed to verify windows-upgrade pod image")
			}

			err = uninstall()
			if err != nil {
				log.WithError(err).Error("Uninstall failed, will retry")
				break
			}

			time.Sleep(3 * time.Second)
			err = execScript(script)
			if err != nil {
				log.WithError(err).Fatal("Failed to upgrade to new version")
			}

			// Upgrade will run in another process. The running
			// calico-upgrade service is done. The new calico-upgrade
			// service will clean the old service up.
			date := time.Now().Format("2006-01-02")
			log.Info(fmt.Sprintf("Upgrade is in progress. Upgrade log is in c:\\calico-upgrade.%v.log", date))
			time.Sleep(3 * time.Second)
			return
		}
	}
}

// Return the base directory for Calico upgrade service.
func baseDir() string {
	dir := filepath.Dir(os.Args[0])
	return "c:\\" + filepath.Base(dir)
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
	// After the uninstall completes, move the existing calico-node.exe to
	// a temporary file. The calico-upgrade service is still running so not
	// doing this means we cannot replace calico-node.exe with the upgrade.
	stdout, stderr, err = powershell(fmt.Sprintf(`mv %v\calico-node.exe %v\calico-node.exe.to-be-replaced`, baseDir(), baseDir()))
	fmt.Println(stdout, stderr)
	if err != nil {
		return err
	}

	return nil
}

func execScript(script string) error {
	log.Infof("Start script %s\n", script)

	// This has to be done in a separate process because when the new calico services are started, the existing
	// calico-upgrade service is removed so the new calico-upgrade service can be started.
	// However, removing the existing calico-upgrade service means the powershell
	// process running the upgrade script is killed and the installation is left
	// incomplete.
	cmd := fmt.Sprintf(`Start-Process powershell -argument %q -WindowStyle hidden`, script)
	stdout, stderr, err := powershell(cmd)

	if err != nil {
		return err
	}
	fmt.Println(stdout, stderr)
	return nil
}

func verifyImagesShareRegistryPath(first, second string) error {
	n1, err := image.ParseNamed(first)
	if err != nil {
		return err
	}
	n2, err := image.ParseNamed(second)
	if err != nil {
		return err
	}
	if image.Domain(n1) != image.Domain(n2) {
		return fmt.Errorf("images %q and %q do not share the same domain", first, second)
	}

	// Remove the last segment of the image path. The last segment will contain
	// the component name.
	n1PathParts := strings.Split(image.Path(n1), "/")
	n2PathParts := strings.Split(image.Path(n2), "/")

	n1PathPrefix := n1PathParts[:len(n1PathParts)-1]
	n2PathPrefix := n2PathParts[:len(n2PathParts)-1]

	for i := range n1PathPrefix {
		if n1PathPrefix[i] != n2PathPrefix[i] {
			return fmt.Errorf("images %q and %q are not from the same registry and path", first, second)
		}
	}
	return nil
}

func verifyPodImageWithHostPathVolume(cs kubernetes.Interface, nodeName string, hostPath string) error {
	// Get pod list for all pods on this node.
	list, err := cs.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName,
	})
	if err != nil {
		return err
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

	// Get the calico node image
	nodeDs := daemonset("calico-node")
	nodeImage, err := nodeDs.getContainerImage(cs, "calico-system", "calico-node")
	if err != nil {
		return err
	}
	log.Infof("Found node container image is %v\n", nodeImage)

	// Walk through pods
	for _, pod := range list.Items {
		if !hasHostPathVolume(pod, hostPath) {
			continue
		}

		if len(pod.Spec.Containers) != 1 {
			return fmt.Errorf("Pod with hostpath volume has more than one container")
		}

		upgradeImage := pod.Spec.Containers[0].Image
		log.Infof("Found upgrade image: %v", upgradeImage)

		err = verifyImagesShareRegistryPath(nodeImage, upgradeImage)
		if err != nil {
			return err
		}

		return nil
	}

	return fmt.Errorf("Failed to find calico-windows-upgrade pod")
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
