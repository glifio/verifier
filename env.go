package main

import (
	"reflect"
	"strconv"
	"time"

	envpkg "github.com/caarlos0/env"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
)

type Env struct {
	Port               string        `env:"PORT" envDefault:"8080"`
	AWSRegion          string        `env:"AWS_REGION" envDefault:"us-east-1"`
	AWSAccessKey       string        `env:"AWS_ACCESS_KEY,required"`
	AWSSecretKey       string        `env:"AWS_SECRET_KEY,required"`
	LotusAPIMultiaddr  string        `env:"LOTUS_API_MULTIADDR" envDefault:"/ip4/127.0.0.1/tcp/1234"`
	LotusAPIToken      string        `env:"LOTUS_API_TOKEN"`
	GithubClientID     string        `env:"GITHUB_CLIENT_ID,required"`
	GithubClientSecret string        `env:"GITHUB_CLIENT_SECRET,required"`
	MinAccountAge      time.Duration `env:"MIN_ACCOUNT_AGE_DAYS" envDefault:"180"`
	MaxAllowanceBytes  big.Int       `env:"MAX_ALLOWANCE_BYTES"`
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
	})
	if err != nil {
		panic(err)
	}
}
