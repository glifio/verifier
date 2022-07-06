module github.com/openworklabs/oauthserver

go 1.14

require (
	contrib.go.opencensus.io/exporter/jaeger v0.2.1 // indirect
	github.com/aws/aws-sdk-go v1.40.45
	github.com/caarlos0/env v3.5.0+incompatible
	github.com/cockroachdb/pebble v0.0.0-20201001221639-879f3bfeef07 // indirect
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/filecoin-project/go-address v0.0.6
	github.com/filecoin-project/go-jsonrpc v0.1.5
	github.com/filecoin-project/go-multistore v0.0.3 // indirect
	github.com/filecoin-project/go-state-types v0.1.10
	github.com/filecoin-project/lotus v1.16.0
	github.com/filecoin-project/specs-actors/v8 v8.0.1
	github.com/gin-contrib/cors v1.3.1
	github.com/gin-gonic/gin v1.6.3
	github.com/glifio/go-logger v0.7.0
	github.com/go-redis/redis/v8 v8.11.4
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/uuid v1.3.0
	github.com/guregu/dynamo v1.10.2
	github.com/ipfs/go-cid v0.1.0
	github.com/ipfs/go-ds-pebble v0.0.2-0.20200921225637-ce220f8ac459 // indirect
	github.com/ipfs/go-hamt-ipld v0.1.1
	github.com/ipfs/go-ipld-cbor v0.0.6
	github.com/ipld/go-ipld-prime-proto v0.1.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/streadway/quantile v0.0.0-20150917103942-b0c588724d25 // indirect
	github.com/whyrusleeping/cbor-gen v0.0.0-20220323183124-98fa8256a799
	github.com/whyrusleeping/pubsub v0.0.0-20190708150250-92bcb0691325 // indirect
	github.com/zondax/ledger-go v0.12.1 // indirect
	gopkg.in/robfig/cron.v2 v2.0.0-20150107220207-be2e0b0deed5
)

replace github.com/filecoin-project/filecoin-ffi => ./filecoin-ffi
