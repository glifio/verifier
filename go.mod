module github.com/openworklabs/oauthserver

go 1.14

require (
	github.com/aws/aws-sdk-go v1.40.45
	github.com/caarlos0/env v3.5.0+incompatible
	github.com/dgraph-io/badger/v2 v2.2007.3 // indirect
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/filecoin-project/go-address v1.0.0
	github.com/filecoin-project/go-jsonrpc v0.1.8
	github.com/filecoin-project/go-state-types v0.9.8
	github.com/filecoin-project/lotus v1.18.1
	github.com/filecoin-project/specs-actors v0.9.15
	github.com/filecoin-project/specs-actors/v8 v8.0.1 // indirect
	github.com/filecoin-project/specs-storage v0.4.1 // indirect
	github.com/gin-contrib/cors v1.3.1
	github.com/gin-gonic/gin v1.6.3
	github.com/glifio/go-logger v0.7.0
	github.com/go-redis/redis/v8 v8.11.4
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/uuid v1.3.0
	github.com/guregu/dynamo v1.10.2
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/ipfs/go-cid v0.2.0
	github.com/ipfs/go-ds-leveldb v0.5.0 // indirect
	github.com/ipfs/go-hamt-ipld v0.1.1
	github.com/ipfs/go-ipld-cbor v0.0.6
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.12.1 // indirect
	github.com/whyrusleeping/cbor-gen v0.0.0-20220514204315-f29c37e9c44c
	go.uber.org/fx v1.15.0 // indirect
	google.golang.org/genproto v0.0.0-20210917145530-b395a37504d4 // indirect
	gopkg.in/robfig/cron.v2 v2.0.0-20150107220207-be2e0b0deed5
)

replace github.com/filecoin-project/filecoin-ffi => ./filecoin-ffi
