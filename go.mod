module github.com/cresta/helm-s3-proxy

go 1.16

require (
	github.com/aws/aws-sdk-go v1.40.9
	github.com/cresta/gitops-autobot v0.0.0-20210809072243-470482ea070c
	github.com/cresta/gotracing v0.0.3
	github.com/cresta/httpsimple v0.0.1
	github.com/cresta/magehelper v0.0.56
	github.com/cresta/zapctx v0.0.1
	github.com/go-git/go-git/v5 v5.4.2
	github.com/gorilla/mux v1.8.0
	github.com/signalfx/golib/v3 v3.3.35
	go.uber.org/zap v1.18.1
)

exclude github.com/go-logr/logr v1.0.0
