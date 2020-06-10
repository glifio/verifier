package main

import (
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

func main() {
	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"POST"},
		AllowHeaders:     []string{"Origin", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// # API wrapper for Lotus commands:
	//
	// how much data do i have
	// give me an allowance
	//     - check age of GH account
	//     - check current allowance
	router.POST("/oauth/:provider", serveOauth)

	router.Run(":" + env.Port)
}

var (
	ErrUnsupportedProvider = errors.New("unsupported oauth provider")
	ErrUserTooNew          = errors.New("user account is too new")
	ErrSufficientAllowance = errors.New("allowance is already sufficient")
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

func serveOauth(c *gin.Context) {
	providerName := c.Param("provider")
	provider, exists := oauthProviders[providerName]
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrUnsupportedProvider.Error()})
		return
	}

	type Request struct {
		Code            string `json:"code" binding:"required"`
		State           string `json:"state" binding:"required"`
		FilecoinAddress string `json:"filecoinAddress" binding:"required"`
	}

	var body Request
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Exchange the `code` for an `access_token`
	token, err := OAuthExchangeCodeForToken(provider, body.Code, body.State)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Fetch the user's profile
	accountData, err := provider.FetchAccountData(token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure that the user meets our criteria for being granted an allowance
	if time.Now().Sub(accountData.CreatedAt).Hours() < env.MinAccountAge.Hours() {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserTooNew.Error()})
		return
	}

	// Update user record in Dynamo
	user, err := fetchUserWithProviderEmail(providerName, accountData.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "fetching DynamoDB user: " + err.Error()})
		return
	}

	user.FilecoinAddress = body.FilecoinAddress
	user.Accounts[providerName] = accountData

	err = saveUser(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "saving DynamoDB user: " + err.Error()})
		return
	}

	// @@TODO: Verify Filecoin address
}
