package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

var jwtSecret = []byte("ALSKjdflakjs;dklfj;askdj;flaskdjf")

func main() {
	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"POST"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.POST("/oauth/:provider", serveOauth)
	router.POST("/make-verifier", serveMakeVerifier)
	router.POST("/verify", serveVerifyAccount)
	router.GET("/verifiers", serveListVerifiers)
	router.GET("/verified-clients", serveListVerifiedClients)
	router.GET("/account-remaining-bytes/:target_addr", serveCheckAccountRemainingBytes)
	router.GET("/verifier-remaining-bytes/:target_addr", serveCheckVerifierRemainingBytes)

	router.Run(":" + env.Port)
}

var (
	ErrUnsupportedProvider  = errors.New("unsupported oauth provider")
	ErrUserTooNew           = errors.New("user account is too new")
	ErrSufficientAllowance  = errors.New("allowance is already sufficient")
	ErrAllocatedTooRecently = errors.New("you must wait 30 days in between reallocations")
)

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

	// Update user record in Dynamo
	user, err := fetchUserWithProviderUniqueID(providerName, accountData.UniqueID)
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

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"filecoinAddress": body.FilecoinAddress,
		"nbf":             time.Date(2015, 10, 10, 12, 0, 0, 0, time.UTC).Unix(),
	})

	// Sign and get the complete encoded token as a string using the secret
	jwtTokenString, err := jwtToken.SignedString(jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generating JWT: " + err.Error()})
		return
	}

	type Response struct {
		JWT string `json:"jwt"`
	}

	c.JSON(http.StatusOK, Response{jwtTokenString})
}

func serveMakeVerifier(c *gin.Context) {
	type Request struct {
		TargetAddr string `json:"targetAddr" binding:"required"`
		Allowance  string `json:"allowance" binding:"required"`
	}

	var body Request
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := lotusMakeAccountAVerifier(body.TargetAddr, body.Allowance)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
}

func serveVerifyAccount(c *gin.Context) {
	// Fetch the targetAddr from the provided JWT
	var targetAddr string
	{
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "jwt token missing"})
			return
		}

		jwtToken := strings.TrimSpace(authHeader[len("Bearer "):])

		token, err := jwt.Parse(jwtToken, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
			}
			return jwtSecret, nil
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid jwt"})
			return
		}

		targetAddr, ok = claims["filecoinAddress"].(string)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid jwt"})
			return
		}
	}

	user, err := getUserByFilecoinAddress(targetAddr)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "user not found, have you authenticated?"})
		return
	}

	// Ensure that the user's account is old enough
	var foundOne bool
	for _, account := range user.Accounts {
		if time.Now().Sub(account.CreatedAt).Hours() >= env.MinAccountAge.Hours() {
			foundOne = true
			break
		}
	}
	if !foundOne {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserTooNew.Error()})
		return
	}

	// Ensure that the user hasn't asked for more allocation too recently
	if user.MostRecentAllocation.Add(30 * 24 * time.Hour).After(time.Now()) {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrAllocatedTooRecently.Error()})
		return
	}

	remaining, err := lotusCheckAccountRemainingBytes(targetAddr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure that the user is actually owed bytes
	owed := big.Sub(env.MaxAllowanceBytes, remaining)
	if big.Cmp(owed, big.NewInt(0)) <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "you have verified data already, Greedy McRichbags"})
		return
	}

	cid, err := lotusVerifyAccount(targetAddr, owed.String())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type Response struct {
		Cid string `json:"cid"`
	}
	c.JSON(http.StatusOK, Response{Cid: cid.String()})
}

func serveListVerifiers(c *gin.Context) {
	verifiers, err := lotusListVerifiers()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, verifiers)
}

func serveListVerifiedClients(c *gin.Context) {
	verifiedClients, err := lotusListVerifiedClients()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, verifiedClients)
}

func serveCheckAccountRemainingBytes(c *gin.Context) {
	targetAddr := c.Param("target_addr")

	dcap, err := lotusCheckAccountRemainingBytes(targetAddr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if dcap.Int == nil {
		dcap = big.NewInt(0)
	}

	user, err := getUserByFilecoinAddress(targetAddr)
	if err != nil {
		// no-op
	}

	type Response struct {
		RemainingBytes       string    `json:"remainingBytes"`
		MostRecentAllocation time.Time `json:"mostRecentAllocation"`
	}
	c.JSON(http.StatusOK, Response{dcap.String(), user.MostRecentAllocation})
}

func serveCheckVerifierRemainingBytes(c *gin.Context) {
	targetAddr := c.Param("target_addr")

	dcap, err := lotusCheckVerifierRemainingBytes(targetAddr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dcap)
}
