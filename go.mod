module github.com/Cray-HPE/cray-powerdns-manager

go 1.19

require (
	github.com/Cray-HPE/hms-sls v1.19.0
	github.com/Cray-HPE/hms-smd v1.62.0
	github.com/gin-gonic/gin v1.7.2
	github.com/hashicorp/go-retryablehttp v0.7.0
	github.com/joeig/go-powerdns/v2 v2.4.1
	github.com/mitchellh/mapstructure v1.4.1
	github.com/namsral/flag v1.7.4-pre
	github.com/xlab/treeprint v1.1.0
	go.uber.org/zap v1.18.1
)

require (
	github.com/Cray-HPE/hms-base v1.15.0 // indirect
	github.com/Cray-HPE/hms-certs v1.3.2 // indirect
	github.com/Cray-HPE/hms-securestorage v1.12.2 // indirect
	github.com/Cray-HPE/hms-xname v1.0.1 // indirect
	github.com/cenkalti/backoff/v3 v3.2.2 // indirect
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/gin-contrib/sse v0.1.0 // indirect
	github.com/go-playground/locales v0.13.0 // indirect
	github.com/go-playground/universal-translator v0.17.0 // indirect
	github.com/go-playground/validator/v10 v10.7.0 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.2 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hashicorp/vault/api v1.1.1 // indirect
	github.com/hashicorp/vault/sdk v0.2.1 // indirect
	github.com/json-iterator/go v1.1.11 // indirect
	github.com/leodido/go-urn v1.2.1 // indirect
	github.com/mattn/go-isatty v0.0.13 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/ugorji/go/codec v1.2.6 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	golang.org/x/crypto v0.0.0-20210711020723-a769d52b0f97 // indirect
	golang.org/x/net v0.0.0-20210614182718-04defd469f4e // indirect
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect
	golang.org/x/text v0.3.6 // indirect
	golang.org/x/time v0.0.0-20210611083556-38a9dc6acbc6 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
	gopkg.in/square/go-jose.v2 v2.6.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

// Temporary until I can get a PR opened to the parent project for the CryptoKey and TSIGKey support we need.
replace github.com/joeig/go-powerdns/v2 => github.com/SeanWallace/go-powerdns/v2 v2.4.1-0.20210914015402-5c6aa3160920
