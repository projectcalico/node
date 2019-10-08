module github.com/projectcalico/node

go 1.12

require (
	github.com/Azure/go-autorest v11.1.2+incompatible // indirect
	github.com/coreos/etcd v3.3.13+incompatible // indirect
	github.com/gophercloud/gophercloud v0.0.0-20190126172459-c818fa66e4c8 // indirect
	github.com/imdario/mergo v0.3.5 // indirect
	github.com/kelseyhightower/confd v0.16.0
	github.com/mattn/go-isatty v0.0.8 // indirect
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/projectcalico/felix v0.0.0-20191003065011-e01caf688c90
	github.com/projectcalico/libcalico-go v0.0.0-20191008175127-399044ecb659
	github.com/projectcalico/typha v0.0.0-20191003041517-d59f99ba14c5
	github.com/sirupsen/logrus v1.4.2
	github.com/ugorji/go/codec v1.1.7
	github.com/vishvananda/netns v0.0.0-20180720170159-13995c7128cc // indirect
	golang.org/x/oauth2 v0.0.0-20190402181905-9f3314589c9a // indirect
	k8s.io/api v0.0.0-20190620084959-7cf5895f2711
	k8s.io/apimachinery v0.0.0-20190612205821-1799e75a0719
	k8s.io/client-go v8.0.0+incompatible
)

replace github.com/sirupsen/logrus => github.com/projectcalico/logrus v0.0.0-20180627202928-fc9bbf2f57995271c5cd6911ede7a2ebc5ea7c6f

replace github.com/kelseyhightower/confd => github.com/projectcalico/confd v0.0.0-20191003042443-c6a88e7c8ac8
