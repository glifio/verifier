package main

import (
	"reflect"
	"strconv"
	"time"

	envpkg "github.com/caarlos0/env"
	"github.com/filecoin-project/go-address"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
)

type Env struct {
	Port               string          `env:"PORT" envDefault:"8080"`
	AWSRegion          string          `env:"AWS_REGION" envDefault:"us-east-1"`
	AWSAccessKey       string          `env:"AWS_ACCESS_KEY,required"`
	AWSSecretKey       string          `env:"AWS_SECRET_KEY,required"`
	LotusAPIDialAddr   string          `env:"LOTUS_API_DIAL_ADDR,required"`
	LotusAPIToken      string          `env:"LOTUS_API_TOKEN,required"`
	LotusVerifierAddr  address.Address `env:"LOTUS_VERIFIER_ADDR,required"`
	GithubClientID     string          `env:"GITHUB_CLIENT_ID,required"`
	GithubClientSecret string          `env:"GITHUB_CLIENT_SECRET,required"`
	MinAccountAge      time.Duration   `env:"MIN_ACCOUNT_AGE_DAYS" envDefault:"180"`
	MaxAllowanceBytes  big.Int         `env:"MAX_ALLOWANCE_BYTES"`
}

var env Env

func init() {
	err := envpkg.ParseWithFuncs(&env, map[reflect.Type]envpkg.ParserFunc{
		reflect.TypeOf(time.Duration(0)): func(v string) (interface{}, error) {
			minAccountAgeDays, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return nil, err
			}
			return time.Duration(minAccountAgeDays) * 24 * time.Hour, nil
		},

		reflect.TypeOf(big.Int{}): func(v string) (interface{}, error) {
			n, err := big.FromString(v)
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
