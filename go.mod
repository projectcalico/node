module github.com/projectcalico/node

go 1.13

require (
	github.com/coreos/etcd v3.3.13+incompatible // indirect
	github.com/kelseyhightower/confd v0.0.0-00010101000000-000000000000
	github.com/onsi/ginkgo v1.10.1
	github.com/onsi/gomega v1.7.0
	github.com/projectcalico/felix v0.0.0-20191127172636-00b97ba18e04
	github.com/projectcalico/libcalico-go v1.7.2-0.20191127160841-0216c08ec5bf
	github.com/sirupsen/logrus v1.4.2
	github.com/vishvananda/netns v0.0.0-20180720170159-13995c7128cc // indirect
	gopkg.in/fsnotify/fsnotify.v1 v1.4.7
	k8s.io/api v0.0.0-20191121175643-4ed536977f46
	k8s.io/apimachinery v0.0.0-20191121175448-79c2a76c473a
	k8s.io/client-go v0.0.0-20190620085101-78d2af792bab
)

replace (
	github.com/kelseyhightower/confd => github.com/projectcalico/confd v1.0.1-0.20191127173538-50ade12359a5
	github.com/sirupsen/logrus => github.com/projectcalico/logrus v1.0.4-calico
)
