module github.com/Cray-HPE/cray-powerdns-manager

go 1.16

require (
	github.com/Cray-HPE/hms-sls v1.19.0
	github.com/Cray-HPE/hms-smd v1.30.9
	github.com/cenkalti/backoff/v3 v3.2.2 // indirect
	github.com/gin-gonic/gin v1.8.2
	github.com/golang/snappy v0.0.4 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.0
	github.com/hashicorp/vault/api v1.1.1 // indirect
	github.com/joeig/go-powerdns/v2 v2.4.1
	github.com/mitchellh/mapstructure v1.4.1
	github.com/namsral/flag v1.7.4-pre
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/xlab/treeprint v1.1.0
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	go.uber.org/zap v1.18.1
	golang.org/x/time v0.0.0-20210611083556-38a9dc6acbc6 // indirect
	gopkg.in/square/go-jose.v2 v2.6.0 // indirect
)

// Temporary until I can get a PR opened to the parent project for the CryptoKey and TSIGKey support we need.
replace github.com/joeig/go-powerdns/v2 => github.com/SeanWallace/go-powerdns/v2 v2.4.1-0.20210914015402-5c6aa3160920
