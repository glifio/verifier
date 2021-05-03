module github.com/openworklabs/oauthserver

go 1.14

require (
	github.com/Jeffail/gabs v1.4.0 // indirect
	github.com/aws/aws-sdk-go v1.37.24
	github.com/caarlos0/env v3.5.0+incompatible
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/filecoin-project/go-address v0.0.5
	github.com/filecoin-project/go-jsonrpc v0.1.4-0.20210217175800-45ea43ac2bec
	github.com/filecoin-project/go-state-types v0.1.0
	github.com/filecoin-project/lotus v1.8.0
	github.com/filecoin-project/specs-actors/v4 v4.0.0
	github.com/gin-contrib/cors v1.3.1
	github.com/gin-gonic/gin v1.6.3
	github.com/go-redis/redis/v8 v8.6.0
	github.com/google/uuid v1.2.0
	github.com/guregu/dynamo v1.10.2
	github.com/ipfs/go-cid v0.0.7
	github.com/ipfs/go-hamt-ipld v0.1.1
	github.com/ipfs/go-ipld-cbor v0.0.5
	github.com/pkg/errors v0.9.1
	github.com/whyrusleeping/cbor-gen v0.0.0-20210303213153-67a261a1d291
	gopkg.in/robfig/cron.v2 v2.0.0-20150107220207-be2e0b0deed5
)

replace github.com/filecoin-project/filecoin-ffi => ./filecoin-ffi
