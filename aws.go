package main

import (
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/aws"
	awscreds "github.com/aws/aws-sdk-go/aws/credentials"
	awssession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/google/uuid"
	"github.com/guregu/dynamo"
)

type User struct {
	ID                          string
	Accounts                    map[string]AccountData
	MostRecentAllocation        time.Time
	MostRecentMinerFaucetGrant  time.Time
	MostRecentFaucetGrantCid    string
	MostRecentFaucetAddress     string
	ReceivedNonMinerFaucetGrant bool
	VerifiedFilecoinAddress     string
}

type AccountData struct {
	UniqueID  string    `json:"unique_id"`
	Username  string    `json:"username"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func (user User) HasAccountOlderThan(threshold time.Duration) bool {
	for _, account := range user.Accounts {
		if time.Now().Sub(account.CreatedAt).Hours() >= threshold.Hours() {
			return true
		}
	}
	return false
}

func (user User) HasRequestedFromFaucetAsMiner() bool {
	return !user.MostRecentMinerFaucetGrant.IsZero()
}

func (user User) ChangedMinerAddress(newMinerAddress address.Address) bool {
	return user.MostRecentFaucetAddress != newMinerAddress.String()
}

func dynamoTable(name string) dynamo.Table {
	awsConfig := aws.NewConfig().
		WithRegion(env.AWSRegion).
		WithCredentials(awscreds.NewStaticCredentials(env.AWSAccessKey, env.AWSSecretKey, ""))

	return dynamo.New(awssession.New(), awsConfig).Table(env.DynamodbTableName)
}

func getUserByID(userID string) (User, error) {
	table := dynamoTable(env.DynamodbTableName)

	var user User
	err := table.Get("ID", userID).One(&user)
	return user, err
}

func getUserWithProviderUniqueID(providerName, uniqueID string) (User, error) {
	table := dynamoTable(env.DynamodbTableName)

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

func lockUser(userID string, lock UserLock) error {
	table := dynamoTable(env.DynamodbTableName)
	return table.Update("ID", userID).
		Set("Locked_"+string(lock), true).
		If("'Locked_"+string(lock)+"' = ? OR attribute_not_exists(Locked_"+string(lock)+")", false).
		Run()
}

func unlockUser(userID string, lock UserLock) error {
	table := dynamoTable(env.DynamodbTableName)
	return table.Update("ID", userID).
		Set("Locked_"+string(lock), false).
		If("'Locked_"+string(lock)+"' = ?", true).
		Run()
}

func saveUser(user User) error {
	table := dynamoTable(env.DynamodbTableName)
	return table.Put(user).Run()
}

func getUserByVerifiedFilecoinAddress(filecoinAddr string) (User, error) {
	table := dynamoTable(env.DynamodbTableName)

	var users []User
	err := table.Scan().
		Filter("VerifiedFilecoinAddress = ?", filecoinAddr).
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
