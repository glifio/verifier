package main

import (
	"reflect"
	"time"

	envpkg "github.com/caarlos0/env"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/chain/types"
)

// Mode allows the backend to run in only verifier or faucet mode
type Mode string

const (
	// FaucetMode runs just the faucet
	FaucetMode Mode = "FAUCET"
	// VerifierMode runs just the verifier
	VerifierMode Mode = "VERIFIER"
)

// Env exports
type Env struct {
	Port                      string          `env:"PORT" envDefault:"8080"`
	JWTSecret                 string          `env:"JWT_SECRET,required"`
	AWSRegion                 string          `env:"AWS_REGION" envDefault:"us-east-1"`
	AWSAccessKey              string          `env:"AWS_ACCESS_KEY,required"`
	AWSSecretKey              string          `env:"AWS_SECRET_KEY,required"`
	DynamodbTableName         string          `env:"DYNAMODB_TABLE_NAME,required"`
	LotusAPIDialAddr          string          `env:"LOTUS_API_DIAL_ADDR,required"`
	LotusAPIToken             string          `env:"LOTUS_API_TOKEN"`
	BlockedAddresses          string          `env:"BLOCKED_ADDRESSES"`
	GithubClientID            string          `env:"GITHUB_CLIENT_ID,required"`
	GithubClientSecret        string          `env:"GITHUB_CLIENT_SECRET,required"`
	FilplusApiKey             string          `env:"FILPLUS_API_KEY,required"`
	SentryDsn                 string          `env:"SENTRY_DSN"`
	SentryEnv                 string          `env:"SENTRY_ENV"`
	MaxFee                    types.FIL       `env:"MAX_FEE" envDefault:"0afil"`
	Mode                      Mode            `env:"MODE"`
	// verifier specific env vars
	VerifierPrivateKey        string          `env:"VERIFIER_PK"`
	VerifierMinAccountAgeDays uint            `env:"VERIFIER_MIN_ACCOUNT_AGE_DAYS" envDefault:"180"`
	VerifierRateLimit         time.Duration   `env:"VERIFIER_RATE_LIMIT" envDefault:"730h"`
	BaseAllowanceBytes        big.Int         `env:"BASE_ALLOWANCE_BYTES"`
	MaxTotalAllocations       uint            `env:"MAX_TOTAL_ALLOCATIONS" envDefault:"0"`
	AllocationsCounterResetPword string       `env:"ALLOCATIONS_COUNTER_PWD"`
	RedisEndpoint             string          `env:"REDIS_ENDPOINT"`
	RedisPwd                  string          `env:"REDIS_PASSWORD"`
	// faucet specific env vars
	FaucetPrivateKey          string          `env:"FAUCET_PK"`
	FaucetRateLimit           time.Duration   `env:"FAUCET_RATE_LIMIT" envDefault:"24h"`
	FaucetGrantSize           types.FIL       `env:"FAUCET_GRANT_SIZE" envDefault:"10fil"`
	FaucetMinAccountAgeDays   uint            `env:"FAUCET_MIN_ACCOUNT_AGE" envDefault:"180"`
}

var env Env

func init() {
	err := envpkg.ParseWithFuncs(&env, map[reflect.Type]envpkg.ParserFunc{
		reflect.TypeOf(big.Int{}): func(v string) (interface{}, error) {
			n, err := big.FromString(v)
			if err != nil {
				return nil, err
			}
			return n, nil
		},

		reflect.TypeOf(types.FIL{}): func(v string) (interface{}, error) {
			n, err := types.ParseFIL(v)
			if err != nil {
				return nil, err
			}
			return n, nil
		},

		reflect.TypeOf(address.Address{}): func(v string) (interface{}, error) {
			return address.NewFromString(v)
		},
	})
	if err != nil {
		panic(err)
	}
}
