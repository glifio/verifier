module github.com/openworklabs/oauthserver

go 1.14

replace github.com/filecoin-project/filecoin-ffi => ./filecoin-ffi

replace github.com/supranational/blst => github.com/supranational/blst v0.1.2-alpha.1

replace github.com/filecoin-project/sector-storage => github.com/filecoin-project/sector-storage v0.0.0-20200812222704-c3077fb85119

require (
	github.com/caarlos0/env v3.5.0+incompatible // indirect
	github.com/filecoin-project/go-fil-markets v0.5.6 // indirect
	github.com/filecoin-project/lotus v0.4.3-0.20200820203717-d1718369a182 // indirect
	github.com/gin-contrib/cors v1.3.1 // indirect
	github.com/gin-gonic/gin v1.6.3 // indirect
	github.com/guregu/dynamo v1.8.0 // indirect
)
