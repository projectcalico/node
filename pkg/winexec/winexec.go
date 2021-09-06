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
package winexec

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/projectcalico/node/pkg/lifecycle/startup"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	ps "github.com/bhendo/go-powershell"
	"github.com/bhendo/go-powershell/backend"

	"github.com/projectcalico/node/pkg/lifecycle/utils"

	log "github.com/sirupsen/logrus"
)

const (
	TigeraImagePrefix       = "songtjiang/exec:"
	CalicoKubeConfigFile    = "calico-kube-config"
	CalicoUpdateExecDir     = "c:\\CalicoUpdateExec"
	CalicoBaseDir           = "c:\\CalicoWindows"
	CalientBaseDir          = "c:\\TigeraCalico"
	CalicoEXLabel           = "projectcalico.org/CalicoExecScript"
	CalicoVersionAnnotation = "projectcalico.org/CalicoExecVersion"
)

func getVersionString() string {
	if runningCalient() {
		return "Cailent-" + startup.VERSION
	}
	return "Cailco-" + startup.VERSION
}

// This file contains the winexec processing for the calico/node.  This
// includes:
// -  Monitoring node labels and get CailcoExec script file from the label.
// -  Uninstall current Calico/Calient running on the node.
//    Install new version of Calico/Calient.
func Run() {
	// Determine the name for this node.
	nodeName := utils.DetermineNodeName()
	log.Infof("Starting CalicoExec service on node %s ", nodeName)

	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigFile())
	if err != nil {
		log.WithError(err).Fatal("failed to build Kubernetes client config")
	}
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.WithError(err).Fatal("failed to create Kubernetes client")
	}

	// choose a backend
	back := &backend.Local{}

	// start a local powershell process
	shell, err := ps.New(back)
	if err != nil {
		log.WithError(err).Fatal("failed to create a local powershell process")
	}
	defer shell.Exit()

	// ... and interact with it
	stdout, stderr, err := shell.Execute("Get-ComputerInfo  | select WindowsVersion, OsBuildNumber, OsHardwareAbstractionLayer")
	if err != nil {
		log.WithError(err).Fatal("failed to interact with powershell")
	}

	fmt.Println(stdout, stderr)

	// Configure node labels.
	// Set Version annotation to indicate what is running on this node.
	node := k8snode(nodeName)
	err = node.addRemoveNodeAnnotations(clientSet,
		map[string]string{CalicoVersionAnnotation: getVersionString()},
		[]string{})
	if err != nil {
		log.WithError(err).Fatal("failed to configure node labels")
	}

	ctx, cancel := context.WithCancel(context.Background())

	go loop(ctx, clientSet, shell, nodeName)

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
	log.Info("Received system signal...Done.")
}

func loop(ctx context.Context, cs kubernetes.Interface, shell ps.Shell, nodeName string) {
	ticker := time.NewTicker(10 * time.Second)

	getScript := func() (string, error) {
		node, err := cs.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			// Treat error as the label does not exist.
			// Will retry by outer loop.
			log.WithError(err).Error("Unable to get node resources")
			return "", nil
		}

		fileName, ok := node.Labels[CalicoEXLabel]
		if !ok {
			return "", nil
		}

		script := filepath.Join(CalicoUpdateExecDir, fileName)

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
				log.Info("CalicoExec scripts are not available yet.")
				// Use fast ticker
				ticker.Stop()
				ticker = time.NewTicker(3 * time.Second)
			} else {
				if len(script) > 0 {
					// Before execute the script, verify host path volume mount.
					_, err := VerifyPodImageWithHostPathVolume(cs, nodeName, CalicoUpdateExecDir, TigeraImagePrefix)
					if err != nil {
						log.WithError(err).Fatal("failed to verify CalicoExecWindows pod image")
					}

					err = uninstall(shell)
					if err != nil {
						log.WithError(err).Error("Uninstall failed")
						break
					}

					time.Sleep(3 * time.Second)
					err = execScript(shell, script)
					if err != nil {
						log.WithError(err).Fatal("failed to upgrade to new version")
					}

					log.Info("All done.")
					return
				}
				// No EX label yet, continue
				log.Info("node EX label does not exist\n")
			}
		}
	}
}

// Return the base directory for CalicoExec service.
func baseDir() string {
	dir := filepath.Dir(os.Args[0])
	parent := "c:\\" + filepath.Base(dir)
	log.Infof("CalicoExec service base directory: %s\n", parent)

	return parent
}

// Return if the exec service is running as part of Calient installation.
func runningCalient() bool {
	return baseDir() == CalientBaseDir
}

// Return kubeconfig file path for Calico
func kubeConfigFile() string {
	return baseDir() + "\\" + CalicoKubeConfigFile
}

func uninstall(shell ps.Shell) error {
	path := filepath.Join(baseDir(), "uninstall-calico.ps1")
	log.Infof("Start uninstall script %s\n", path)
	stdout, stderr, err := shell.Execute(path)
	if err != nil {
		return err
	}
	fmt.Println(stdout, stderr)
	return nil
}

func execScript(shell ps.Shell, script string) error {
	log.Infof("Start exec script %s\n", script)
	stdout, stderr, err := shell.Execute(script)
	if err != nil {
		return err
	}
	fmt.Println(stdout, stderr)
	return nil
}

func VerifyPodImageWithHostPathVolume(cs kubernetes.Interface, nodeName string, hostPath string, imagePrefix string) (string, error) {
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

		if !strings.HasPrefix(container.Image, imagePrefix) {
			return "", fmt.Errorf("Pod with hostpath volume has invalid image %s, prefix %s", container.Image, imagePrefix)
		}

		return container.Image, nil
	}

	return "", fmt.Errorf("Failed to find CalicoExecPod")
}
