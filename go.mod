module github.com/openworklabs/oauthserver

go 1.14

replace github.com/filecoin-project/filecoin-ffi => ./filecoin-ffi

replace github.com/filecoin-project/fil-blst => ./fil-blst

replace github.com/supranational/blst => ./fil-blst/blst

require (
	github.com/aws/aws-sdk-go v1.32.11
	github.com/caarlos0/env v3.5.0+incompatible
	github.com/davidlazar/go-crypto v0.0.0-20200604182044-b73af7476f6c // indirect
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/filecoin-project/go-address v0.0.5-0.20201013120618-9dfb096d952e
	github.com/filecoin-project/go-jsonrpc v0.1.2-0.20201008195726-68c6a2704e49
	github.com/filecoin-project/go-state-types v0.0.0-20201003010437-c33112184a2b
	github.com/filecoin-project/lotus v0.10.1-0.20201013201124-d5cea9f402b0
	github.com/filecoin-project/specs-actors v0.9.12
	github.com/filecoin-project/specs-actors/v2 v2.1.0
	github.com/gin-contrib/cors v1.3.1
	github.com/gin-gonic/gin v1.6.3
	github.com/go-redis/redis/v8 v8.4.4
	github.com/google/uuid v1.1.1
	github.com/guregu/dynamo v1.8.0
	github.com/ipfs/go-cid v0.0.7
	github.com/ipfs/go-hamt-ipld v0.1.1
	github.com/ipfs/go-ipld-cbor v0.0.5-0.20200428170625-a0bd04d3cbdf
	github.com/pkg/errors v0.9.1
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/whyrusleeping/cbor-gen v0.0.0-20200826160007-0b9f6c5fb163
	golang.org/x/lint v0.0.0-20200302205851-738671d3881b // indirect
	gopkg.in/robfig/cron.v2 v2.0.0-20150107220207-be2e0b0deed5
	honnef.co/go/tools v0.0.1-2020.1.3 // indirect
)
