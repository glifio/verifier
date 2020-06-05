package main

import (
	"reflect"
	"strconv"
	"time"

	envpkg "github.com/caarlos0/env"
)

type Env struct {
	Port               string        `env:"PORT" envDefault:"8080"`
	AWSRegion          string        `env:"AWS_REGION" envDefault:"us-east-1"`
	AWSAccessKey       string        `env:"AWS_ACCESS_KEY,required"`
	AWSSecretKey       string        `env:"AWS_SECRET_KEY,required"`
	GithubClientID     string        `env:"GITHUB_CLIENT_ID,required"`
	GithubClientSecret string        `env:"GITHUB_CLIENT_SECRET,required"`
	MinAccountAge      time.Duration `env:"MIN_ACCOUNT_AGE_DAYS" envDefault:"180"`
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
	})
	if err != nil {
		panic(err)
	}
}
