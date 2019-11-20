module github.com/projectcalico/node

go 1.13

require (
	github.com/coreos/etcd v3.3.13+incompatible // indirect
	github.com/kelseyhightower/confd v0.16.0
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/projectcalico/felix v0.0.0-20191119064945-bc9f3fb41aa4
	github.com/projectcalico/libcalico-go v1.7.2-0.20191119183141-c072e7a2fae4
	github.com/sirupsen/logrus v1.4.2
	github.com/vishvananda/netns v0.0.0-20180720170159-13995c7128cc // indirect
	gopkg.in/fsnotify/fsnotify.v1 v1.4.7
	k8s.io/api v0.0.0-20190718183219-b59d8169aab5
	k8s.io/apimachinery v0.0.0-20190612205821-1799e75a0719
	k8s.io/client-go v12.0.0+incompatible
)

replace (
	github.com/kelseyhightower/confd => github.com/projectcalico/confd v0.0.0-20191119041309-8fb60a3e1669
	github.com/sirupsen/logrus => github.com/projectcalico/logrus v1.0.4-calico
)
