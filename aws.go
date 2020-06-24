package main

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	awssession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/google/uuid"
	"github.com/guregu/dynamo"
)

type User struct {
	ID              string
	FilecoinAddress string
	Accounts        map[string]AccountData
}

type AccountData struct {
	Username  string    `json:"login"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

func dynamoTable(name string) dynamo.Table {
	awsConfig := aws.NewConfig().
		WithRegion(env.AWSRegion).
		WithCredentials(awscreds.NewStaticCredentials(env.AWSAccessKey, env.AWSSecretKey, ""))

	return dynamo.New(awssession.New(), awsConfig).Table("filecoin-verified-addresses")
}

func fetchUserWithProviderEmail(providerName, email string) (User, error) {
	table := dynamoTable("filecoin-verified-addresses")

	var users []User
	err := table.Scan().
		Filter("Accounts."+providerName+".Email = ?", email).
		Limit(1).
		All(&users)
	if err != nil {
		return User{}, err
	}

	var user User
	if len(users) > 0 {
		user = users[0]
	} else {
		user.ID = uuid.New().String()
		user.Accounts = make(map[string]AccountData)
	}
	return user, nil
}

func saveUser(user User) error {
	table := dynamoTable("filecoin-verified-addresses")
	return table.Put(user).Run()
}

func getUserByFilecoinAddress(filecoinAddr string) (User, error) {
	table := dynamoTable("filecoin-verified-addresses")
	var user User
	err := table.Get("FilecoinAddress", filecoinAddr).One(&user)
	return user, err
}
