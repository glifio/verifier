package main

import (
	"time"

	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/aws"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	awssession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/google/uuid"
	"github.com/guregu/dynamo"
)

type User struct {
	ID                    string
	Accounts              map[string]AccountData
	MostRecentAllocation  time.Time
	MostRecentFaucetGrant time.Time
	FilecoinAddress       string
}

type AccountData struct {
	UniqueID  string    `json:"unique_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func dynamoTable(name string) dynamo.Table {
	awsConfig := aws.NewConfig().
		WithRegion(env.AWSRegion).
		WithCredentials(awscreds.NewStaticCredentials(env.AWSAccessKey, env.AWSSecretKey, ""))

	return dynamo.New(awssession.New(), awsConfig).Table("filecoin-verified-addresses")
}

func getUserByID(userID string) (User, error) {
	table := dynamoTable("filecoin-verified-addresses")

	var user User
	err := table.Get("ID", userID).One(&user)
	return user, err
}

func getUserWithProviderUniqueID(providerName, uniqueID string) (User, error) {
	table := dynamoTable("filecoin-verified-addresses")

	var users []User
	err := table.Scan().
		Filter("Accounts."+providerName+".UniqueID = ?", uniqueID).
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

func lockUser(userID string) error {
	table := dynamoTable("filecoin-verified-addresses")
	return table.Update("ID", userID).
		Set("Locked", true).
		If("'Locked' = ? OR attribute_not_exists(Locked)", false).
		Run()
}

func unlockUser(userID string) error {
	table := dynamoTable("filecoin-verified-addresses")
	return table.Update("ID", userID).
		Set("Locked", false).
		If("'Locked' = ?", true).
		Run()
}

func saveUser(user User) error {
	table := dynamoTable("filecoin-verified-addresses")
	return table.Put(user).Run()
}

func getUserByFilecoinAddress(filecoinAddr string) (User, error) {
	table := dynamoTable("filecoin-verified-addresses")

	var users []User
	err := table.Scan().
		Filter("FilecoinAddress = ?", filecoinAddr).
		Limit(1).
		All(&users)
	if err != nil {
		return User{}, err
	}

	if len(users) == 0 {
		return User{}, errors.New("user not found")
	}
	return users[0], nil
}
