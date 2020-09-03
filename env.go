package main

import (
	"reflect"
	"time"

	envpkg "github.com/caarlos0/env"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
)

type Env struct {
	Port                      string          `env:"PORT" envDefault:"8080"`
	JWTSecret                 string          `env:"JWT_SECRET,required"`
	AWSRegion                 string          `env:"AWS_REGION" envDefault:"us-east-1"`
	AWSAccessKey              string          `env:"AWS_ACCESS_KEY,required"`
	AWSSecretKey              string          `env:"AWS_SECRET_KEY,required"`
	DynamodbTableName         string          `env:"DYNAMODB_TABLE_NAME,required"`
	LotusAPIDialAddr          string          `env:"LOTUS_API_DIAL_ADDR,required"`
	LotusAPIToken             string          `env:"LOTUS_API_TOKEN,required"`
	LotusVerifierAddr         address.Address `env:"LOTUS_VERIFIER_ADDR,required"`
	GithubClientID            string          `env:"GITHUB_CLIENT_ID,required"`
	GithubClientSecret        string          `env:"GITHUB_CLIENT_SECRET,required"`
	VerifierMinAccountAgeDays uint            `env:"VERIFIER_MIN_ACCOUNT_AGE_DAYS" envDefault:"180"`
	MaxAllowanceBytes         big.Int         `env:"MAX_ALLOWANCE_BYTES"`
	FaucetAddr                address.Address `env:"FAUCET_ADDR"`
	FaucetRateLimit           time.Duration   `env:"FAUCET_RATE_LIMIT" envDefault:"24h"`
	VerifierRateLimit         time.Duration   `env:"VERIFIER_RATE_LIMIT" envDefault:"730h"`
	FaucetNonMinerGrant       types.FIL       `env:"FAUCET_NON_MINER_RATE" envDefault:"100fil"`
	FaucetFirstTimeMinerGrant types.FIL       `env:"FAUCET_FIRST_TIME_MINER_GRANT" envDefault:"1000fil"`
	FaucetMinerGrant          types.FIL       `env:"FAUCET_MINER_GRANT" envDefault:"500fil"`
	MaxFee                    types.FIL       `env:"MAX_FEE" envDefault:"0afil"`
	FaucetMinAccountAge       time.Duration   `env:"FAUCET_MIN_ACCOUNT_AGE" envDefault:"336h"`
	PathToBlocklistTxtFile    string          `env:"PATH_TO_BLOCKLIST_TXT_FILE"`
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
